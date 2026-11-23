"""Re-entry after a resolution (SFR-AGG-034): hash snapshots and the decision table.

Snapshots are taken when S7 is entered and when it is left. Decision (``decide_reentry``):

* the developer chose another process        -> S2 with a new run (the old run is kept)
* the failed state is S1 (precondition fixed) -> S1
* otherwise the earliest of: the failed state; S3 when the ``.process`` listing, the selected
  process file, the process id, a step script under ``.process/scripts/`` or a pool file
  changed; S4 when bound input files, the input globs/templates/rulesets or the binding changed.
  Nothing changed, or only ``overrides.json`` -> the failed state itself.

S6 restarts at a node: after S5 rebuilt the graph the caller computes
``earliest_changed_node(old_graph, new_graph, changed_sources(before, after))`` and sends
RESTART(node). ``decide_reentry`` itself never sets ``restart_node``.
"""

from collections.abc import Mapping
from dataclasses import dataclass, field
from pathlib import Path

from ..errors import StaticStateName
from ..services.runfs import RunPaths, WorkspacePaths, canonical_json, sha256_bytes, sha256_file
from ..state import Binding, GraphSpec, ReEntry
from . import refs
from .loader import load_process


@dataclass(frozen=True)
class HashSnapshot:
    process_listing: str
    process_file: str
    pools: str
    inputs: Mapping[str, str] = field(default_factory=dict)   # "source:<N>", "glob:<N>", "template:<N>", "ruleset:<N>"
    binding: str = ""
    overrides: str = ""
    process_id: str = ""
    scripts: str = ""                                          # files under .process/scripts/


def _h(lines: "list[str]") -> str:
    return sha256_bytes("\n".join(lines).encode("utf-8"))


def _file_hash(p: Path) -> str:
    return sha256_file(p) if p.is_file() else "missing"


def snapshot(ws: WorkspacePaths, paths: "RunPaths | None", proc_file: "str | None",
             binding: "Binding | None") -> HashSnapshot:
    listing = _h(sorted(p.name for p in ws.process_dir.iterdir()
                        if p.is_file() and p.suffix in (".yml", ".yaml"))) if ws.process_dir.is_dir() else _h([])

    pfile = ""
    pid = ""
    proc = None
    if proc_file:
        path = Path(proc_file)
        path = path if path.is_absolute() else ws.root / path
        pfile = _file_hash(path)
        if path.is_file():
            proc, _ = load_process(path, ws)
            if proc is not None:
                pid = proc.id

    pool_lines = []
    for base in (ws.snippet_dir, ws.skills_dir):
        if base.is_dir():
            for f in sorted(x for x in base.rglob("*") if x.is_file()):
                pool_lines.append(f"{ws.rel(f)}\0{sha256_file(f)}")

    scripts_dir = ws.process_dir / "scripts"
    script_lines = [f"{ws.rel(f)}\0{sha256_file(f)}" for f in sorted(scripts_dir.rglob("*")) if f.is_file()] \
        if scripts_dir.is_dir() else []

    inputs: dict[str, str] = {}
    if proc is not None:
        from .binding import find_candidates
        for name, sets in find_candidates(proc, ws).items():
            inputs[f"glob:{name}"] = _h([",".join(s) for s in sets])
    if binding is not None:
        for name, bs in binding.sources.items():
            inputs[f"source:{name}"] = _h([f"{f.path}:{_file_hash(ws.root / f.path)}" for f in bs.files]
                                          + [f"note:{bs.note_text or ''}"])
        for name, b in binding.templates.items():
            inputs[f"template:{name}"] = _file_hash(ws.root / b.path)
        for name, b in binding.rulesets.items():
            inputs[f"ruleset:{name}"] = _file_hash(ws.root / b.path)
    elif proc is not None:
        for t in proc.templates:
            inputs[f"template:{t.name}"] = _file_hash(ws.root / t.path)
        for r in proc.rulesets:
            inputs[f"ruleset:{r.name}"] = _file_hash(ws.root / r.path)

    overrides = ""
    if paths is not None and paths.overrides_json.is_file():
        overrides = sha256_file(paths.overrides_json)

    return HashSnapshot(
        process_listing=listing, process_file=pfile, pools=_h(pool_lines), inputs=dict(inputs),
        binding=sha256_bytes(canonical_json(binding.to_json()).encode("utf-8")) if binding is not None else "",
        overrides=overrides, process_id=pid, scripts=_h(script_lines))


_ORDER = {"S1": 1, "S2": 2, "S3": 3, "S4": 4, "S5": 5, "S6": 6}


def process_changed(before: HashSnapshot, after: HashSnapshot) -> bool:
    return (before.process_listing != after.process_listing or before.process_file != after.process_file
            or before.process_id != after.process_id or before.pools != after.pools
            or before.scripts != after.scripts)


def inputs_changed(before: HashSnapshot, after: HashSnapshot) -> bool:
    return before.inputs != after.inputs or before.binding != after.binding


def changed_sources(before: HashSnapshot, after: HashSnapshot) -> set[str]:
    """Names of sources, templates and rulesets whose snapshot keys differ."""
    names = set()
    for key in set(before.inputs) | set(after.inputs):
        if before.inputs.get(key) != after.inputs.get(key):
            names.add(key.split(":", 1)[1])
    return names


def decide_reentry(failed: StaticStateName, before: HashSnapshot, after: HashSnapshot, *,
                   process_choice_changed: bool) -> ReEntry:
    if failed not in _ORDER:
        raise ValueError(f"re-entry is defined for S1..S6, not {failed!r}")
    if process_choice_changed:
        return ReEntry(state="S2", new_run=True)
    if failed == "S1":
        return ReEntry(state="S1")
    target = failed
    if process_changed(before, after) and _ORDER["S3"] < _ORDER[target]:
        target = "S3"
    if inputs_changed(before, after) and _ORDER["S4"] < _ORDER[target]:
        target = "S4"
    return ReEntry(state=target)


def _edge_key(spec: GraphSpec, node: str) -> frozenset:
    return frozenset((e.dst, e.when, e.outcome) for e in spec.edges if e.src == node)


def earliest_changed_node(old: GraphSpec, new: GraphSpec, changed_sources: "set[str]") -> "str | None":
    """First node (in ``new.order``) that is new, or whose step hash, outgoing edges or input reads
    changed. None when nothing in the graph changed."""
    old_nodes = {n.name: n for n in old.nodes}
    new_nodes = {n.name: n for n in new.nodes}
    if old.entry != new.entry and new.order:
        return new.order[0]
    for name in new.order:
        n = new_nodes[name]
        o = old_nodes.get(name)
        if o is None or o.step_hash != n.step_hash or _edge_key(old, name) != _edge_key(new, name):
            return name
        if changed_sources:
            for raw in n.reads:
                r = refs.parse_read(raw)
                if r is not None and r.kind.startswith("input.") and r.name in changed_sources:
                    return name
    return None
