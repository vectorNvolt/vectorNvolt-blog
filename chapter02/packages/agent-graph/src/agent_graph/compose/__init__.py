"""compose_context: the working set of a cognitive node (SFR-AGG-017/018/039, SSR-AGG-010/011/017).

Pure function of its inputs: writes only into ``active_dir`` (``system.md``, ``CLAUDE.md``,
``context.md``, ``context.json``, ``mcp.json``, ``.claude/skills/``). Nothing is written when
ContextOverBudget is raised before the write phase (no session may start for such a node).
"""

from dataclasses import dataclass
from pathlib import Path
from typing import Mapping, Sequence

from .. import config
from ..errors import ContextOverBudget
from ..ports import Launch, SummarizerPort
from ..services.runfs import (InputReadGuard, WorkspacePaths, WriteGuard, sha256_bytes,
                              write_atomic)
from ..state import Binding, PoolIndex, ProcessDef, ProductRef, RunFact, StepDef
from .context_pack import build_context_pack, ctx_path
from .manifest import (EMPTY_MCP_JSON, MANIFEST_SCHEMA, preserved_records, record_compaction,
                       record_turn, write_manifest)
from .normalize import join_blocks
from .skills import select_skills, stage_skills
from .snippets import (compose_system_md, render_facts, select_layer2, snippet_bodies,
                       snippet_records, system_snippet_ids)
from .step_contract import build_step_contract
from .tokens import estimate_tokens

__all__ = ["ComposeInputs", "ComposeResult", "compose_context", "compose_system_md",
           "estimate_tokens", "record_turn", "record_compaction"]


@dataclass(frozen=True)
class ComposeInputs:
    process: ProcessDef
    step: StepDef
    index: PoolIndex
    binding: Binding
    products: Mapping[str, ProductRef]
    upstream_handoffs: Sequence[tuple]      # (node name, path of its handoff.json)
    facts: Sequence[RunFact]
    ws: WorkspacePaths
    active_dir: Path
    launch: Launch


@dataclass(frozen=True)
class ComposeResult:
    system_md_sha256: str
    claude_md_sha256: str
    launch_sha256: str
    staged_skills: tuple
    excluded_skills: tuple
    context_tokens: int
    manifest: Path


def _sha(text: str) -> str:
    return sha256_bytes(text.encode("utf-8"))


def compose_context(inp: ComposeInputs, summarizer: SummarizerPort, guard: WriteGuard) -> ComposeResult:
    """Compose the working set of ``inp.step`` into ``inp.active_dir``.

    Raises ContextOverBudget when the mandatory context (goal, output schema, declared reads) or
    layer L2 does not fit, before anything is written."""
    process, step, index, ws, active = inp.process, inp.step, inp.index, inp.ws, inp.active_dir

    # --- system.md (L0 + L1): independent of the step
    l0, l1 = system_snippet_ids(process, index)
    system_md = compose_system_md(process, index)

    # --- CLAUDE.md: L2 (declared, auto, contract) + L3 (facts)
    sel = select_layer2(process, step, index)
    contract = build_step_contract(process, step, lambda p: ctx_path(ws, active, p))
    cap2 = config.LAYER_CAPS["L2"]
    auto = sel.auto
    l2 = join_blocks([*snippet_bodies(index, sel.declared), *snippet_bodies(index, auto), contract])
    dropped: tuple = ()
    if estimate_tokens(l2) > cap2 and auto:
        dropped, auto = auto, ()
        l2 = join_blocks([*snippet_bodies(index, sel.declared), contract])
    if estimate_tokens(l2) > cap2:
        raise ContextOverBudget(step.name, cap2, estimate_tokens(l2))
    l3, kept_facts, omitted_facts = render_facts(list(inp.facts), config.LAYER_CAPS["L3"])
    claude_md = join_blocks([l2, l3])

    # --- skills
    skills = select_skills(process, step, index)

    # --- context.md
    pack = build_context_pack(
        process=process, step=step, binding=inp.binding, products=inp.products,
        upstream_handoffs=inp.upstream_handoffs, facts=inp.facts, ws=ws, active_dir=active,
        summarizer=summarizer, reader=InputReadGuard(ws, inp.binding))

    launch_sha = inp.launch.launch_hash()
    system_sha, claude_sha = _sha(system_md), _sha(claude_md)
    manifest_path = active / "context.json"
    manifest = {
        "schema": MANIFEST_SCHEMA,
        "node": step.name,
        "step_id": step.id,
        "layers": {
            "L0": {"snippets": snippet_records(index, l0), "tokens": estimate_tokens(join_blocks(snippet_bodies(index, l0)))},
            "L1": {"snippets": snippet_records(index, l1), "tokens": estimate_tokens(join_blocks(snippet_bodies(index, l1)))},
            "L2": {"snippets": snippet_records(index, sel.declared), "auto": snippet_records(index, auto),
                   "dropped_auto": list(dropped), "step_contract": True, "tokens": estimate_tokens(l2)},
            "L3": {"facts": list(kept_facts), "omitted_facts": list(omitted_facts),
                   "tokens": estimate_tokens(l3)},
        },
        "skills": {"declared": list(skills.declared), "staged": list(skills.staged),
                   "excluded": list(skills.excluded),
                   "ranked": [{"name": n, "score": s} for n, s in skills.ranked],
                   "not_staged_cap": list(skills.not_staged_cap),
                   "max_skills": process.max_skills_of(step)},
        "products_consumed": list(pack.products),
        "items": list(pack.items),
        "omitted": list(pack.omitted),
        "budget_tokens": process.budget_of(step),
        "context_tokens": pack.tokens,
        "hashes": {"system_md_sha256": system_sha, "claude_md_sha256": claude_sha,
                   "context_md_sha256": _sha(pack.text), "launch_sha256": launch_sha},
        **preserved_records(manifest_path, step.name),
    }

    # --- write phase (only into active/)
    write_atomic(active / "system.md", system_md, guard=guard)
    write_atomic(active / "CLAUDE.md", claude_md, guard=guard)
    write_atomic(active / "context.md", pack.text, guard=guard)
    write_atomic(active / inp.launch.mcp_config, EMPTY_MCP_JSON, guard=guard)
    stage_skills(skills.staged, index, ws, active, guard)
    write_manifest(manifest_path, manifest, guard)
    return ComposeResult(system_md_sha256=system_sha, claude_md_sha256=claude_sha,
                         launch_sha256=launch_sha, staged_skills=skills.staged,
                         excluded_skills=skills.excluded, context_tokens=pack.tokens,
                         manifest=manifest_path)
