"""Run facts (L3 of the context): developer answers, review decisions, resolution summaries.

Covers: SFR-AGG-022/023/024 (answers and review feedback become facts), SFR-AGG-034
(resolution summary), SFR-AGG-017 (L3 is these facts in the order they occurred).
Stored in run_facts.jsonl; ``seq`` starts at 1 and increases by 1.
"""

import threading
from pathlib import Path

from ..state import RunFact
from .runfs import WriteGuard, append_jsonl, read_jsonl

KINDS = ("answer", "hil_ok", "hil_nok", "resolution")


class RunFactsStore:
    def __init__(self, path: Path, guard: WriteGuard):
        self.path = Path(path)
        self.guard = guard
        self._lock = threading.Lock()

    def append(self, kind: str, node: "str | None", text: str) -> RunFact:
        if kind not in KINDS:
            raise ValueError(f"unknown run fact kind: {kind!r}")
        with self._lock:
            recs = read_jsonl(self.path)
            seq = max((r["seq"] for r in recs), default=0) + 1
            append_jsonl(self.path, {"seq": seq, "kind": kind, "node": node, "text": text},
                         guard=self.guard)
        return RunFact(seq=seq, kind=kind, node=node, text=text)  # type: ignore[arg-type]

    def all(self) -> list[RunFact]:
        with self._lock:
            recs = read_jsonl(self.path)
        return [RunFact(seq=r["seq"], kind=r["kind"], node=r.get("node"), text=r["text"]) for r in recs]
