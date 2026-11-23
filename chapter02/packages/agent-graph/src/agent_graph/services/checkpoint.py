"""SQLite checkpoint store for the dynamic graph, plus the node journal.

Covers: SSR-AGG-006 (durable checkpoint in .agent/<runID>/checkpoint.sqlite), SSR-AGG-009
(session ids live in the checkpoint store, via the journal), SFR-AGG-037 (resume).

``SqliteCheckpointSaver`` implements the synchronous surface of
langgraph.checkpoint.base.BaseCheckpointSaver (langgraph-checkpoint 4.3.1): get_tuple, list, put,
put_writes, delete_thread. The async methods, delete_for_runs, copy_thread and prune keep the base
class default (NotImplementedError); the graph does not call them. Standard-library sqlite3,
WAL mode, synchronous=FULL, JsonPlusSerializer. ``get_delta_channel_history`` uses the base
default (walks get_tuple), which relies on pending_writes being returned in writes_sort_key order.

The journal table ``node_journal(thread, node, visit, key, value)`` stores JSON values per
(node, visit, key); the session id of a node visit is the reserved key ``__session__``.
"""

from __future__ import annotations

import json
import sqlite3
import threading
from collections.abc import Iterator, Sequence
from pathlib import Path
from typing import Any

from langgraph.checkpoint.base import (
    WRITES_IDX_MAP,
    BaseCheckpointSaver,
    CheckpointTuple,
    get_checkpoint_id,
    get_checkpoint_metadata,
    writes_sort_key,
)
from langgraph.checkpoint.serde.jsonplus import JsonPlusSerializer

SESSION_KEY = "__session__"

_SCHEMA = """
CREATE TABLE IF NOT EXISTS checkpoints (
    thread_id TEXT NOT NULL, checkpoint_ns TEXT NOT NULL, checkpoint_id TEXT NOT NULL,
    parent_checkpoint_id TEXT, type TEXT NOT NULL, checkpoint BLOB NOT NULL,
    metadata_type TEXT NOT NULL, metadata BLOB NOT NULL,
    PRIMARY KEY (thread_id, checkpoint_ns, checkpoint_id));
CREATE TABLE IF NOT EXISTS blobs (
    thread_id TEXT NOT NULL, checkpoint_ns TEXT NOT NULL, channel TEXT NOT NULL,
    version TEXT NOT NULL, type TEXT NOT NULL, blob BLOB NOT NULL,
    PRIMARY KEY (thread_id, checkpoint_ns, channel, version));
CREATE TABLE IF NOT EXISTS writes (
    thread_id TEXT NOT NULL, checkpoint_ns TEXT NOT NULL, checkpoint_id TEXT NOT NULL,
    task_id TEXT NOT NULL, idx INTEGER NOT NULL, channel TEXT NOT NULL, type TEXT NOT NULL,
    blob BLOB NOT NULL, task_path TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (thread_id, checkpoint_ns, checkpoint_id, task_id, idx));
CREATE TABLE IF NOT EXISTS node_journal (
    thread TEXT NOT NULL, node TEXT NOT NULL, visit INTEGER NOT NULL, key TEXT NOT NULL,
    value TEXT NOT NULL, PRIMARY KEY (thread, node, visit, key));
"""


def _connect(db: Path) -> sqlite3.Connection:
    Path(db).parent.mkdir(parents=True, exist_ok=True)
    conn = sqlite3.connect(str(db), check_same_thread=False, isolation_level=None, timeout=30.0)
    conn.execute("PRAGMA journal_mode=WAL")
    conn.execute("PRAGMA synchronous=FULL")
    conn.execute("PRAGMA busy_timeout=30000")
    conn.executescript(_SCHEMA)
    return conn


class SqliteCheckpointSaver(BaseCheckpointSaver[str]):
    def __init__(self, db: Path | str, *, serde: Any = None):
        super().__init__(serde=serde or JsonPlusSerializer())
        self.db = Path(db)
        self._lock = threading.RLock()
        self._conn: sqlite3.Connection | None = _connect(self.db)

    # ------------------------------------------------------------------ lifecycle
    @property
    def conn(self) -> sqlite3.Connection:
        if self._conn is None:
            raise RuntimeError("checkpoint store is closed")
        return self._conn

    def close(self) -> None:
        with self._lock:
            if self._conn is not None:
                self._conn.close()
                self._conn = None

    def __enter__(self) -> "SqliteCheckpointSaver":
        return self

    def __exit__(self, *exc: object) -> None:
        self.close()

    # ------------------------------------------------------------------ helpers
    def _load_blobs(self, thread_id: str, ns: str, versions: dict) -> dict[str, Any]:
        out: dict[str, Any] = {}
        for channel, ver in versions.items():
            row = self.conn.execute(
                "SELECT type, blob FROM blobs WHERE thread_id=? AND checkpoint_ns=? "
                "AND channel=? AND version=?", (thread_id, ns, channel, str(ver))).fetchone()
            if row is None or row[0] == "empty":
                continue
            out[channel] = self.serde.loads_typed((row[0], bytes(row[1])))
        return out

    def _pending_writes(self, thread_id: str, ns: str, checkpoint_id: str) -> list[tuple[str, str, Any]]:
        rows = self.conn.execute(
            "SELECT task_id, idx, channel, type, blob, task_path FROM writes "
            "WHERE thread_id=? AND checkpoint_ns=? AND checkpoint_id=?",
            (thread_id, ns, checkpoint_id)).fetchall()
        rows.sort(key=lambda r: writes_sort_key(r[5], r[0], r[1]))
        return [(r[0], r[2], self.serde.loads_typed((r[3], bytes(r[4])))) for r in rows]

    def _tuple(self, thread_id: str, ns: str, row: tuple, config: Any = None) -> CheckpointTuple:
        checkpoint_id, parent_id, ctype, cblob, mtype, mblob = row
        checkpoint = self.serde.loads_typed((ctype, bytes(cblob)))
        metadata = self.serde.loads_typed((mtype, bytes(mblob)))
        if config is None:
            config = {"configurable": {"thread_id": thread_id, "checkpoint_ns": ns,
                                       "checkpoint_id": checkpoint_id}}
        parent = ({"configurable": {"thread_id": thread_id, "checkpoint_ns": ns,
                                    "checkpoint_id": parent_id}} if parent_id else None)
        return CheckpointTuple(
            config=config,
            checkpoint={**checkpoint,
                        "channel_values": self._load_blobs(thread_id, ns, checkpoint["channel_versions"])},
            metadata=metadata,
            parent_config=parent,
            pending_writes=self._pending_writes(thread_id, ns, checkpoint_id))

    _COLS = "checkpoint_id, parent_checkpoint_id, type, checkpoint, metadata_type, metadata"

    # ------------------------------------------------------------------ BaseCheckpointSaver
    def get_tuple(self, config: Any) -> CheckpointTuple | None:
        thread_id = config["configurable"]["thread_id"]
        ns = config["configurable"].get("checkpoint_ns", "")
        checkpoint_id = get_checkpoint_id(config)
        with self._lock:
            if checkpoint_id:
                row = self.conn.execute(
                    f"SELECT {self._COLS} FROM checkpoints WHERE thread_id=? AND checkpoint_ns=? "
                    "AND checkpoint_id=?", (thread_id, ns, checkpoint_id)).fetchone()
                return self._tuple(thread_id, ns, row, config) if row else None
            row = self.conn.execute(
                f"SELECT {self._COLS} FROM checkpoints WHERE thread_id=? AND checkpoint_ns=? "
                "ORDER BY checkpoint_id DESC LIMIT 1", (thread_id, ns)).fetchone()
            return self._tuple(thread_id, ns, row) if row else None

    def list(self, config: Any, *, filter: dict[str, Any] | None = None, before: Any = None,
             limit: int | None = None) -> Iterator[CheckpointTuple]:
        sql = f"SELECT thread_id, checkpoint_ns, {self._COLS} FROM checkpoints"
        where: list[str] = []
        params: list[Any] = []
        if config:
            where.append("thread_id=?")
            params.append(config["configurable"]["thread_id"])
            ns = config["configurable"].get("checkpoint_ns")
            if ns is not None:
                where.append("checkpoint_ns=?")
                params.append(ns)
            cid = get_checkpoint_id(config)
            if cid:
                where.append("checkpoint_id=?")
                params.append(cid)
        if before and (bid := get_checkpoint_id(before)):
            where.append("checkpoint_id<?")
            params.append(bid)
        if where:
            sql += " WHERE " + " AND ".join(where)
        sql += " ORDER BY thread_id, checkpoint_ns, checkpoint_id DESC"
        results: list[CheckpointTuple] = []
        with self._lock:
            for r in self.conn.execute(sql, params).fetchall():
                tup = self._tuple(r[0], r[1], r[2:])
                if filter and not all(tup.metadata.get(k) == v for k, v in filter.items()):
                    continue
                if limit is not None and len(results) >= limit:
                    break
                results.append(tup)
        yield from results

    def put(self, config: Any, checkpoint: Any, metadata: Any, new_versions: Any) -> Any:
        c = dict(checkpoint)
        thread_id = config["configurable"]["thread_id"]
        ns = config["configurable"].get("checkpoint_ns", "")
        values: dict[str, Any] = c.pop("channel_values")
        ctype, cblob = self.serde.dumps_typed(c)
        mtype, mblob = self.serde.dumps_typed(get_checkpoint_metadata(config, metadata))
        with self._lock:
            self.conn.execute("BEGIN IMMEDIATE")
            try:
                for channel, ver in new_versions.items():
                    btype, bblob = (self.serde.dumps_typed(values[channel]) if channel in values
                                    else ("empty", b""))
                    self.conn.execute(
                        "INSERT OR REPLACE INTO blobs (thread_id, checkpoint_ns, channel, version, type, blob) "
                        "VALUES (?,?,?,?,?,?)", (thread_id, ns, channel, str(ver), btype, bblob))
                self.conn.execute(
                    "INSERT OR REPLACE INTO checkpoints (thread_id, checkpoint_ns, checkpoint_id, "
                    "parent_checkpoint_id, type, checkpoint, metadata_type, metadata) VALUES (?,?,?,?,?,?,?,?)",
                    (thread_id, ns, checkpoint["id"], config["configurable"].get("checkpoint_id"),
                     ctype, cblob, mtype, mblob))
                self.conn.execute("COMMIT")
            except BaseException:
                self.conn.execute("ROLLBACK")
                raise
        return {"configurable": {"thread_id": thread_id, "checkpoint_ns": ns,
                                 "checkpoint_id": checkpoint["id"]}}

    def put_writes(self, config: Any, writes: Sequence[tuple[str, Any]], task_id: str,
                   task_path: str = "") -> None:
        thread_id = config["configurable"]["thread_id"]
        ns = config["configurable"].get("checkpoint_ns", "")
        checkpoint_id = config["configurable"]["checkpoint_id"]
        with self._lock:
            self.conn.execute("BEGIN IMMEDIATE")
            try:
                for i, (channel, value) in enumerate(writes):
                    idx = WRITES_IDX_MAP.get(channel, i)
                    vtype, vblob = self.serde.dumps_typed(value)
                    verb = "INSERT OR IGNORE" if idx >= 0 else "INSERT OR REPLACE"
                    self.conn.execute(
                        f"{verb} INTO writes (thread_id, checkpoint_ns, checkpoint_id, task_id, idx, "
                        "channel, type, blob, task_path) VALUES (?,?,?,?,?,?,?,?,?)",
                        (thread_id, ns, checkpoint_id, task_id, idx, channel, vtype, vblob, task_path))
                self.conn.execute("COMMIT")
            except BaseException:
                self.conn.execute("ROLLBACK")
                raise

    def delete_thread(self, thread_id: str) -> None:
        with self._lock:
            self.conn.execute("BEGIN IMMEDIATE")
            try:
                for table in ("checkpoints", "blobs", "writes"):
                    self.conn.execute(f"DELETE FROM {table} WHERE thread_id=?", (thread_id,))
                self.conn.execute("COMMIT")
            except BaseException:
                self.conn.execute("ROLLBACK")
                raise


class SqliteNodeJournal:
    """NodeJournalPort over the ``node_journal`` table of checkpoint.sqlite (own connection)."""

    def __init__(self, db: Path | str, thread: str):
        self.thread = thread
        self._lock = threading.RLock()
        self._conn: sqlite3.Connection | None = _connect(Path(db))

    def close(self) -> None:
        with self._lock:
            if self._conn is not None:
                self._conn.close()
                self._conn = None

    def get(self, node: str, visit: int, key: str) -> dict | None:
        with self._lock:
            row = self._conn.execute(
                "SELECT value FROM node_journal WHERE thread=? AND node=? AND visit=? AND key=?",
                (self.thread, node, visit, key)).fetchone()
        return json.loads(row[0]) if row else None

    def put(self, node: str, visit: int, key: str, value: dict) -> None:
        with self._lock:
            self._conn.execute(
                "INSERT OR REPLACE INTO node_journal (thread, node, visit, key, value) VALUES (?,?,?,?,?)",
                (self.thread, node, visit, key, json.dumps(value, sort_keys=True, ensure_ascii=False)))

    def session(self, node: str, visit: int) -> str | None:
        rec = self.get(node, visit, SESSION_KEY)
        return rec.get("session_id") if rec else None

    def set_session(self, node: str, visit: int, session_id: str) -> None:
        self.put(node, visit, SESSION_KEY, {"session_id": session_id})


def open_checkpointer(db: Path) -> SqliteCheckpointSaver:
    """Open (create) the checkpoint store; also creates the node_journal table."""
    return SqliteCheckpointSaver(db)


def open_journal(db: Path, thread: str) -> SqliteNodeJournal:
    return SqliteNodeJournal(db, thread)
