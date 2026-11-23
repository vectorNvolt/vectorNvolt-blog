"""context.json: the node's context record (SFR-AGG-017, SFR-AGG-020, SSR-AGG-011).

Written once by compose (layers, snippet ids, staged and excluded skills, items with token
counts, omissions with reasons, hashes), then extended per turn by ``record_turn`` and per
compaction by ``record_compaction``. Serialized with sorted keys.
"""

import json
from pathlib import Path
from typing import Sequence

from ..ports import Usage
from ..services.runfs import WriteGuard, canonical_json, write_atomic

MANIFEST_SCHEMA = "agent-graph/context/v1"
EMPTY_MCP_JSON = '{"mcpServers": {}}\n'          # same bytes for every node and for the summarizer


def read_manifest(path: Path) -> dict:
    return json.loads(Path(path).read_text(encoding="utf-8"))


def write_manifest(path: Path, data: dict, guard: WriteGuard) -> None:
    write_atomic(Path(path), canonical_json(data), guard=guard)


def preserved_records(path: Path, node: str) -> dict:
    """turns and compactions of an existing record of the same node (compose re-run after a
    resume must not lose them); empty otherwise."""
    p = Path(path)
    if not p.is_file():
        return {"turns": [], "compactions": []}
    try:
        old = read_manifest(p)
    except (OSError, ValueError):
        return {"turns": [], "compactions": []}
    if old.get("node") != node:
        return {"turns": [], "compactions": []}
    return {"turns": list(old.get("turns") or []), "compactions": list(old.get("compactions") or [])}


def record_turn(manifest: Path, turn: int, usage: Sequence[Usage], guard: WriteGuard) -> None:
    """Record the usage of turn ``turn`` (one Usage per assistant event). Idempotent per turn."""
    data = read_manifest(manifest)
    usages = [u.to_json() for u in usage]
    totals = {k: sum(u[k] for u in usages) for k in
              ("input_tokens", "cache_creation_input_tokens", "cache_read_input_tokens",
               "ephemeral_1h_input_tokens", "ephemeral_5m_input_tokens")}
    turns = [t for t in data.get("turns", []) if t.get("turn") != turn]
    turns.append({"turn": turn, "usage": usages, "totals": totals})
    data["turns"] = sorted(turns, key=lambda t: t["turn"])
    write_manifest(manifest, data, guard)


def record_compaction(manifest: Path, event: dict, guard: WriteGuard) -> None:
    data = read_manifest(manifest)
    data.setdefault("compactions", []).append(event)
    write_manifest(manifest, data, guard)
