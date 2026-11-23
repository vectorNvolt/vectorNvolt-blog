"""Node journal and the replay-safe node protocol.

Covers: SFR-AGG-019 (session id kept with the checkpoint data), SFR-AGG-037 and SSR-AGG-009
(no completed side effect is repeated when a node is re-run), SSR-AGG-004 (pauses are
interrupts, answers come back as commands).

LangGraph re-runs a node from its beginning whenever it resumes after ``interrupt()``. A node
therefore does every side effect (gate call, language-model turn, procedure run, staging)
through ``NodeRun.once``, which stores the result in the journal; a replay reads the stored
result. The journal is the table ``dyn_journal`` in ``checkpoint.sqlite`` (stdlib sqlite3,
WAL). It is keyed by (node, visit, key), not by thread, so a new dynamic thread that
continues an interrupted node (RESTART with continue_session) finds the earlier results.

Pauses (``NodeRun.pause``) are journaled too, tagged with the writing thread: an answer that
an *older* thread received is returned from the journal; an answer written by the *current*
thread goes through ``interrupt()`` again so that LangGraph's resume list stays aligned.
"""

import json
import sqlite3
import threading
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Callable

from langgraph.config import get_config
from langgraph.errors import GraphBubbleUp
from langgraph.types import interrupt

from ..contract import Cmd, Notice, Status
from ..ports import DynamicContext, ExecBackend

INTERRUPTED_CAUSE = "cancelled:interrupted"      # DynamicMachine.abort (SIGINT, closed terminal)

_SCHEMA = """
CREATE TABLE IF NOT EXISTS dyn_journal (
    node TEXT NOT NULL, visit INTEGER NOT NULL, key TEXT NOT NULL, value TEXT NOT NULL,
    PRIMARY KEY (node, visit, key));
CREATE TABLE IF NOT EXISTS dyn_session (
    node TEXT NOT NULL, visit INTEGER NOT NULL, session_id TEXT NOT NULL,
    PRIMARY KEY (node, visit));
"""


class NodeJournal:
    """Implements ``ports.NodeJournalPort`` on sqlite3. Thread-safe."""

    def __init__(self, db: Path):
        self.db = Path(db)
        self.db.parent.mkdir(parents=True, exist_ok=True)
        self._lock = threading.RLock()
        self._conn = sqlite3.connect(str(self.db), check_same_thread=False, timeout=15)
        with self._lock:
            self._conn.execute("PRAGMA journal_mode=WAL")
            self._conn.execute("PRAGMA synchronous=FULL")
            self._conn.executescript(_SCHEMA)
            self._conn.commit()

    def get(self, node: str, visit: int, key: str) -> "dict | None":
        with self._lock:
            row = self._conn.execute(
                "SELECT value FROM dyn_journal WHERE node=? AND visit=? AND key=?",
                (node, visit, key)).fetchone()
        return None if row is None else json.loads(row[0])

    def put(self, node: str, visit: int, key: str, value: dict) -> None:
        with self._lock:
            self._conn.execute(
                "INSERT OR REPLACE INTO dyn_journal(node, visit, key, value) VALUES (?,?,?,?)",
                (node, visit, key, json.dumps(value, sort_keys=True, ensure_ascii=False)))
            self._conn.commit()

    def session(self, node: str, visit: int) -> "str | None":
        with self._lock:
            row = self._conn.execute(
                "SELECT session_id FROM dyn_session WHERE node=? AND visit=?", (node, visit)).fetchone()
        return None if row is None else row[0]

    def set_session(self, node: str, visit: int, session_id: str) -> None:
        with self._lock:
            self._conn.execute(
                "INSERT OR REPLACE INTO dyn_session(node, visit, session_id) VALUES (?,?,?)",
                (node, visit, session_id))
            self._conn.commit()

    def close(self) -> None:
        with self._lock:
            self._conn.close()


@dataclass
class NodeContext(DynamicContext):
    """``DynamicContext`` plus optional hooks. Every extra has a default, so a plain
    ``DynamicContext`` works too (read through ``getattr``)."""
    emit: "Callable[[Status | Notice], None] | None" = None      # set by DynamicMachine
    compose: "Callable | None" = None            # compose_context; default: agent_graph.compose
    record_turn: "Callable | None" = None        # compose.record_turn
    record_compaction: "Callable | None" = None  # compose.record_compaction
    container: "ExecBackend | None" = None       # backend for isolate: true
    gateway: "object | None" = None              # GatewayPort: cognitive turns render live through gateway.turn


class StepFailure(Exception):
    """A failed attempt: reported as NODE_FAILED, never raised to the engine."""

    def __init__(self, kind: str, reason: str, *, cause: "str | None" = None,
                 retryable: bool = True, extra: "dict | None" = None):
        super().__init__(reason)
        self.kind = kind
        self.reason = reason
        self.cause = cause
        self.retryable = retryable
        self.extra = dict(extra or {})

    def payload(self) -> dict:
        return {"kind": self.kind, "reason": self.reason, "cause": self.cause,
                "retryable": self.retryable, **self.extra}

    @staticmethod
    def from_payload(d: dict) -> "StepFailure":
        extra = {k: v for k, v in d.items() if k not in ("kind", "reason", "cause", "retryable")}
        return StepFailure(d["kind"], d["reason"], cause=d.get("cause"),
                           retryable=d.get("retryable", True), extra=extra)


class NodeRun:
    """One execution of one node body (a replay creates a new NodeRun)."""

    def __init__(self, ctx: DynamicContext, node: str, visit: int, thread: "str | None" = None):
        self.ctx = ctx
        self.node = node
        self.visit = visit
        self.journal = ctx.journal
        if thread is None:
            thread = get_config().get("configurable", {}).get("thread_id", "")
        self.thread = str(thread)
        self._pause_n = 0
        self._emit = getattr(ctx, "emit", None)

    # ---- side effects
    def once(self, key: str, fn: Callable[[], dict]) -> dict:
        """Run ``fn`` once per (node, visit, key). A StepFailure (or any other exception,
        mapped to a non-retryable tool failure) is journaled and re-raised on replay."""
        rec = self.journal.get(self.node, self.visit, key)
        if rec is not None:
            if "__fail__" in rec:
                raise StepFailure.from_payload(rec["__fail__"])
            return rec
        try:
            out = fn()
        except StepFailure as f:
            if f.cause != INTERRUPTED_CAUSE:     # an interrupted attempt must run again on resume
                self.journal.put(self.node, self.visit, key, {"__fail__": f.payload()})
            raise
        except GraphBubbleUp:
            raise
        except Exception as exc:  # noqa: BLE001 - nothing may escape a node as a crash
            f = StepFailure("tool", f"{type(exc).__name__}: {exc}", retryable=False)
            self.journal.put(self.node, self.visit, key, {"__fail__": f.payload()})
            raise f from exc
        self.journal.put(self.node, self.visit, key, out)
        return out

    # ---- events
    def emit(self, item: "Status | Notice") -> None:
        if self._emit is not None:
            self._emit(item)

    def started(self, context_ref: str) -> None:
        self.emit(Status("NODE_STARTED", self.node, {"context_ref": context_ref, "visit": self.visit}))

    def begin(self, context_ref: str) -> None:
        """Step 1-2 of the attempt structure: ask the gate (once), then emit NODE_STARTED.
        A refusal is a non-retryable NODE_FAILED with cause ``cancelled:<budget cause>``."""
        rec = self.once("gate", lambda: {"refusal": self.ctx.gate.before_node_start(self.node)})
        if rec.get("refusal"):
            raise StepFailure("tool", f"run budget reached: {rec['refusal']}",
                              cause=f"cancelled:{rec['refusal']}", retryable=False)
        self.started(context_ref)

    def notice(self, code: str, text: str) -> None:
        self.emit(Notice(f"node {self.node}", code, text))

    def notice_once(self, key: str, code: str, text: str) -> None:
        self.once(key, lambda: (self.notice(code, text), {"shown": True})[1])

    # ---- pauses
    def pause(self, status: Status) -> Cmd:
        """Surface ``status`` through interrupt() and return the command that resumes the node."""
        key = f"p{self._pause_n}"
        self._pause_n += 1
        rec = self.journal.get(self.node, self.visit, key)
        if rec is not None and rec.get("w") != self.thread:
            return Cmd.from_json(rec["cmd"])
        cmd = Cmd.from_json(interrupt(status.to_json()))
        if cmd.name != "SUSPEND":
            self.journal.put(self.node, self.visit, key, {"cmd": cmd.to_json(), "w": self.thread})
        return cmd

    def done(self, *, outcome: "str | None", refs: dict, summary: str, hil: "str | None" = None,
             handoff: "str | None" = None, extra: "dict | None" = None) -> dict:
        """Emit NODE_DONE and return the state update of the node (references and a short summary)."""
        summary = summary[:500]
        payload = {"outcome": outcome, "products": list(refs.values()), "summary": summary,
                   **(extra or {})}
        self.emit(Status("NODE_DONE", self.node, payload))
        update: dict = {"last_node": self.node, "last_outcome": outcome, "last_hil": hil,
                        "visits": {self.node: self.visit}, "products": dict(refs),
                        "summaries": {self.node: summary}}
        if handoff:
            update["handoffs"] = {self.node: handoff}
        return update

    def wait_for(self, status: Status, *expected: str) -> Cmd:
        """Pause at ``status`` until a command in ``expected`` arrives. Any other command (SUSPEND)
        leaves the node paused at SUSPENDED; the engine never resumes it."""
        cmd = self.pause(status)
        while cmd.name not in expected:
            cmd = self.pause(Status("SUSPENDED", self.node, {}))
        return cmd

    def wait_after_failure(self, failure: StepFailure) -> Cmd:
        """Report NODE_FAILED and wait for the engine's RETRY."""
        return self.wait_for(Status("NODE_FAILED", self.node, failure.payload()), "RETRY")


def cancel_failure(cause: str) -> StepFailure:
    """A cancelled attempt: ``timeout`` is retryable (SFR-AGG-026), a budget or interrupt cause is not."""
    return StepFailure("timeout" if cause == "timeout" else "tool", f"attempt cancelled: {cause}",
                       cause=f"cancelled:{cause}", retryable=(cause == "timeout"))


def run_attempts(run: NodeRun, body: Callable[[int, "str | None"], dict],
                 on_fail: "Callable[[int, StepFailure], None] | None" = None) -> dict:
    """Call ``body(attempt, retry_reason)`` until it returns; every StepFailure goes to the
    engine as NODE_FAILED and the engine answers RETRY (loop) or SUSPEND (pause)."""
    attempt, reason = 1, None
    while True:
        try:
            return body(attempt, reason)
        except StepFailure as f:
            if on_fail is not None:
                on_fail(attempt, f)
            cmd = run.wait_after_failure(f)
            reason = str(cmd.payload.get("reason") or f.reason)
            attempt += 1
