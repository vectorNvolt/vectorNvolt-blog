"""Applies the developer-approved changes of one resolution and records them (SFR-AGG-033, 041).

The only code that writes ``.process/``, ``.input/`` and ``identity/`` (through
``WriteGuard.resolution(approved=True)``) or ``overrides.json``. Every change is recorded in
``resolution/<n>/``: ``changes.jsonl`` (one line per change), ``before/`` and ``after/`` copies of
edited files, and ``hashes.json`` (written by ``finish``).

``overrides.json`` format (read by the engine)::

    {"retry": int | {"<node>": int}, "timeout": int | {"<node>": int},   (seconds)
     "budgets": {"max_steps": int, "max_llm_calls": int, "timeout": int},
     "step_budget": {"<node>": int}}                                       (context tokens)

(the format of ``engine.budgets.load_overrides``; a key is either global or per node)
"""

import dataclasses
import json
import os
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

from ..errors import WriteBoundaryError
from ..process.loader import parse_duration, parse_tokens
from .runfs import (WorkspacePaths, WriteGuard, append_jsonl, canonical_json, sha256_bytes,
                    write_atomic)

ALLOWED_ROOTS = (".process", ".input", "identity")
GLOBAL_KEYS = ("retry", "timeout", "budgets.max_steps", "budgets.max_llm_calls", "budgets.timeout")
NODE_KEYS = ("retry", "timeout", "budget")


def _sha(p: Path) -> "str | None":
    return sha256_bytes(p.read_bytes()) if p.is_file() else None


class ResolutionStore:
    def __init__(self, ws: WorkspacePaths, guard: WriteGuard, folder: Path, overrides_path: Path):
        self.ws = ws
        self.guard = guard
        self.folder = Path(folder)
        self.overrides_path = Path(overrides_path)
        self._changes: list[dict] = []

    # ---------------------------------------------------------------- helpers
    def changes(self) -> list[dict]:
        return list(self._changes)

    def _record(self, rec: dict) -> dict:
        rec = {"seq": len(self._changes) + 1,
               "ts": datetime.now(timezone.utc).isoformat(timespec="seconds"), **rec}
        append_jsonl(self.folder / "changes.jsonl", rec, guard=self.guard)
        self._changes.append(rec)
        return rec

    def target(self, rel: str) -> Path:
        """Validate ``rel`` (workspace-relative, under an allowed root) and return its path."""
        if not isinstance(rel, str) or not rel or os.path.isabs(rel) or ".." in Path(rel).parts:
            raise WriteBoundaryError(f"not a workspace-relative path: {rel!r}")
        parts = Path(rel).parts
        if parts[0] not in ALLOWED_ROOTS:
            raise WriteBoundaryError(
                f"{rel}: only {', '.join(ALLOWED_ROOTS)} and the run overrides may be changed")
        return self.ws.root / rel

    # ---------------------------------------------------------------- file edits
    def apply_edit(self, rel: str, content: "str | None", reason: str, *, approved: bool) -> dict:
        """Create or overwrite ``rel`` with ``content``; ``content=None`` deletes the file."""
        target = self.target(rel)
        before = _sha(target)
        if before is not None:
            write_atomic(self.folder / "before" / rel, target.read_bytes(), guard=self.guard)
        with self.guard.resolution(approved):
            if content is None:
                real = self.guard.check(target)
                if real.is_file():
                    real.unlink()
            else:
                write_atomic(target, content, guard=self.guard)
        after = _sha(target)
        if after is not None:
            write_atomic(self.folder / "after" / rel, target.read_bytes(), guard=self.guard)
        return self._record({"kind": "edit_file", "path": rel, "reason": reason,
                             "before_sha256": before, "after_sha256": after})

    # ---------------------------------------------------------------- overrides
    def apply_override(self, key: str, value: Any, node: "str | None", reason: str, *,
                       approved: bool) -> dict:
        if not approved:
            raise WriteBoundaryError("an override needs the developer's approval")
        keys = NODE_KEYS if node else GLOBAL_KEYS
        if key not in keys:
            raise ValueError(f"override key {key!r} is not one of {', '.join(keys)}"
                             + (" (per node)" if node else ""))
        try:
            if key.endswith("timeout"):
                num = parse_duration(value if not isinstance(value, str) else value.strip())
            elif key == "budget":
                num = parse_tokens(value if not isinstance(value, str) else value.strip())
            else:
                num = int(value)
        except (ValueError, KeyError, IndexError, TypeError) as exc:
            raise ValueError(f"override {key}: invalid value {value!r}") from exc
        if num is None or isinstance(num, bool) or num < (0 if key == "retry" else 1):
            raise ValueError(f"override {key}: invalid value {value!r}")
        current = {}
        if self.overrides_path.is_file():
            current = json.loads(self.overrides_path.read_text(encoding="utf-8"))
        before = sha256_bytes(canonical_json(current).encode("utf-8")) if current else None
        if node:
            field = "step_budget" if key == "budget" else key
            slot = current.get(field)
            if isinstance(slot, int):
                raise ValueError(f"override {key}: a global value is set; per-node values cannot be mixed with it")
            current.setdefault(field, {})[node] = num
        elif key.startswith("budgets."):
            current.setdefault("budgets", {})[key.split(".", 1)[1]] = num
        else:
            if isinstance(current.get(key), dict):
                raise ValueError(f"override {key}: per-node values are set; a global value cannot be mixed with them")
            current[key] = num
        text = canonical_json(current)
        write_atomic(self.overrides_path, text, guard=self.guard)
        return self._record({"kind": "override", "key": key, "node": node, "value": num,
                             "reason": reason, "before_sha256": before,
                             "after_sha256": sha256_bytes(text.encode("utf-8"))})

    # ---------------------------------------------------------------- other records
    def record_choice(self, process_id: str, reason: str) -> dict:
        return self._record({"kind": "choose_process", "process_id": process_id, "reason": reason})

    def finish(self, before: Any, after: Any, reentry: "dict | None") -> None:
        """Write hashes.json: the snapshots at entry and exit of S7 and the re-entry decision."""
        def as_dict(s: Any) -> Any:
            return dataclasses.asdict(s) if dataclasses.is_dataclass(s) else s
        write_atomic(self.folder / "hashes.json", canonical_json({
            "before": as_dict(before), "after": as_dict(after), "reentry": reentry,
            "changes": [{k: c.get(k) for k in ("seq", "kind", "path", "key", "before_sha256", "after_sha256")}
                        for c in self._changes]}), guard=self.guard)
