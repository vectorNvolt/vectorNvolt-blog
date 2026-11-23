"""Snippet layers L0..L3 (SFR-AGG-017, SFR-AGG-039, SSR-AGG-011).

L0  all snippets with layer agent, sorted by id          -> system.md
L1  runtime.snippets in declaration order                -> system.md
L2  step snippets (declared order), then auto snippets whose tags match the node goal
    (sorted by id), then the generated step contract     -> CLAUDE.md
L3  run facts in the order they occurred                 -> CLAUDE.md

Pure functions: the same inputs give the same selection and the same bytes.
"""

from dataclasses import dataclass
from typing import Sequence

from ..state import PoolIndex, ProcessDef, RunFact, SnippetEntry, StepDef
from .normalize import join_blocks, normalize_block
from .tokens import estimate_tokens, goal_tokens, word_tokens

FACTS_HEADING = "# Run facts"
FACTS_POINTER_PATH = "../run_facts.jsonl"      # relative to active/ (active/ is a child of the run folder)


def _get(index: PoolIndex, snippet_id: str) -> SnippetEntry:
    try:
        return index.snippets[snippet_id]
    except KeyError:
        raise ValueError(f"snippet not in the pool index: {snippet_id}") from None


def layer0_ids(index: PoolIndex) -> tuple:
    return tuple(sorted(i for i, s in index.snippets.items() if s.layer == "agent"))


def layer1_ids(process: ProcessDef, l0: Sequence[str] = ()) -> tuple:
    """runtime.snippets in declaration order; ids already in L0 or repeated are dropped."""
    seen = set(l0)
    out = []
    for i in process.runtime.snippets:
        if i not in seen:
            seen.add(i)
            out.append(i)
    return tuple(out)


def system_snippet_ids(process: ProcessDef, index: PoolIndex) -> tuple:
    """(L0 ids, L1 ids)."""
    l0 = layer0_ids(index)
    return l0, layer1_ids(process, l0)


def compose_system_md(process: ProcessDef, index: PoolIndex) -> str:
    """system.md: L0 then L1 bodies. Depends only on the process and the pool, never on the step,
    so it is byte-identical for every cognitive node of a run."""
    l0, l1 = system_snippet_ids(process, index)
    return join_blocks(_get(index, i).body for i in (*l0, *l1))


@dataclass(frozen=True)
class L2Selection:
    declared: tuple
    auto: tuple


def tag_matches(tag: str, goal: frozenset) -> bool:
    toks = word_tokens(tag)
    return bool(toks) and toks <= goal


def select_layer2(process: ProcessDef, step: StepDef, index: PoolIndex) -> L2Selection:
    declared = []
    for i in step.snippets:
        _get(index, i)
        if i not in declared:
            declared.append(i)
    goal = goal_tokens(step.name, step.instruction)
    auto = sorted(i for i, s in index.snippets.items()
                  if s.layer == "step" and s.auto and i not in declared
                  and any(tag_matches(t, goal) for t in s.tags))
    return L2Selection(declared=tuple(declared), auto=tuple(auto))


def snippet_bodies(index: PoolIndex, ids: Sequence[str]) -> list:
    return [_get(index, i).body for i in ids]


def snippet_records(index: PoolIndex, ids: Sequence[str]) -> list:
    return [{"id": i, "path": _get(index, i).path, "sha256": _get(index, i).sha256} for i in ids]


# ------------------------------------------------------------------ L3 run facts

def fact_line(f: RunFact) -> str:
    head = f"- #{f.seq} {f.kind}" + (f" ({f.node})" if f.node else "")
    body = normalize_block(f.text).split("\n")
    first = f"{head}: {body[0]}" if body[0] else head
    rest = [("  " + ln) if ln else "" for ln in body[1:]]
    return "\n".join([first, *rest])


def render_facts(facts: Sequence[RunFact], cap_tokens: int) -> tuple:
    """L3 block. Returns (text, kept_seqs, omitted_seqs). Over the cap, the newest facts are
    kept and the older ones are replaced by one pointer line (plan decision 8). "" without facts."""
    if not facts:
        return "", (), ()
    lines = [fact_line(f) for f in facts]
    full = join_blocks([FACTS_HEADING + "\n\n" + "\n".join(lines)])
    if estimate_tokens(full) <= cap_tokens:
        return full, tuple(f.seq for f in facts), ()
    pointer_tpl = "- (facts #{first}-#{last} omitted, see `" + FACTS_POINTER_PATH + "`)"
    kept: list = []
    used = estimate_tokens(FACTS_HEADING + "\n\n" + pointer_tpl.format(first=0, last=0) + "\n")
    for f, ln in zip(reversed(facts), reversed(lines)):
        cost = estimate_tokens(ln + "\n")
        if used + cost > cap_tokens:
            break
        kept.append((f, ln))
        used += cost
    kept.reverse()
    n_omitted = len(facts) - len(kept)
    omitted = facts[:n_omitted]
    if not omitted:
        return full, tuple(f.seq for f in facts), ()
    pointer = pointer_tpl.format(first=omitted[0].seq, last=omitted[-1].seq)
    text = join_blocks([FACTS_HEADING + "\n\n" + "\n".join([pointer, *[ln for _f, ln in kept]])])
    return text, tuple(f.seq for f, _ in kept), tuple(f.seq for f in omitted)
