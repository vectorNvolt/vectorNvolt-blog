"""Command/status history of the dynamic machine (history.jsonl).

Covers: SSR-AGG-006 (durable history), SFR-AGG-012/013 (every command and status is
recorded; the command is recorded before the status it causes).

Record: {"seq","ts","dir":"cmd|status|notice","name","node","payload"}. A notice is
recorded with name=code and payload {"source","text"}.
"""

import threading
from datetime import datetime, timezone
from pathlib import Path

from ..contract import Cmd, Notice, Status
from .runfs import WriteGuard, append_jsonl, read_jsonl


class History:
    def __init__(self, path: Path, guard: WriteGuard):
        self.path = Path(path)
        self.guard = guard
        self._lock = threading.Lock()
        self._seq: int | None = None

    def _append(self, direction: str, name: str, node: "str | None", payload: dict) -> None:
        with self._lock:                    # seq assignment and fsync'd append are one step
            if self._seq is None:
                recs = read_jsonl(self.path)
                self._seq = max((r.get("seq", 0) for r in recs), default=0)
            seq = self._seq + 1
            append_jsonl(self.path, {
                "seq": seq,
                "ts": datetime.now(timezone.utc).isoformat(timespec="milliseconds"),
                "dir": direction, "name": name, "node": node, "payload": payload,
            }, guard=self.guard)
            self._seq = seq

    def command(self, cmd: Cmd) -> None:
        self._append("cmd", cmd.name, cmd.node, dict(cmd.payload))

    def status(self, st: Status) -> None:
        self._append("status", st.name, st.node, dict(st.payload))

    def notice(self, n: Notice) -> None:
        self._append("notice", n.code, None, {"source": n.source, "text": n.text})

    def records(self) -> list[dict]:
        with self._lock:
            return read_jsonl(self.path)
