"""Transition checks: reachability, edge completeness, dead ends (SFR-AGG-005).

Rules (plan decision 10): a step with ``outcomes`` needs exactly one ``outcome == v`` edge per
outcome and no ``done`` edge; a plain step needs exactly one ``done`` edge; a human step needs
exactly one ``hil_ok`` and one ``hil_nok`` edge. Cycles are accepted through defined edges
(they are bounded at run time by ``runtime.budgets.max_steps``).
"""

from collections import deque

from ..errors import Finding
from ..state import Edge, ProcessDef, StepDef

END = "END"


def edges_from(proc: ProcessDef, name: str) -> list[Edge]:
    return [e for e in proc.edges if e.src == name]


def bfs_order(proc: ProcessDef) -> tuple[str, ...]:
    """Steps reachable from the entry, breadth first, edges in declared order. Defines "earliest"."""
    names = {s.name for s in proc.steps}
    if proc.entry not in names:
        return ()
    seen = [proc.entry]
    seen_set = {proc.entry}
    q = deque([proc.entry])
    while q:
        cur = q.popleft()
        for e in edges_from(proc, cur):
            if e.dst in names and e.dst not in seen_set:
                seen_set.add(e.dst)
                seen.append(e.dst)
                q.append(e.dst)
    return tuple(seen)


def reachable(proc: ProcessDef) -> list[str]:
    return list(bfs_order(proc))


def _fd(proc: ProcessDef, code: str, msg: str, *, line=None, step=None, item=None) -> Finding:
    return Finding(code=code, message=msg, file=proc.source_file, line=line, step=step, item=item)


def _check_step_edges(proc: ProcessDef, step: StepDef, out: list[Finding]) -> None:
    edges = edges_from(proc, step.name)
    line = step.line

    def count(pred) -> list[Edge]:
        return [e for e in edges if pred(e)]

    if step.type == "human":
        if step.outcomes:
            out.append(_fd(proc, "bad_when", f"human step {step.name!r} cannot declare outcomes",
                           line=line, step=step.name))
        for kind in ("hil_ok", "hil_nok"):
            es = count(lambda e, k=kind: e.when == k)
            if not es:
                out.append(_fd(proc, "missing_edge",
                               f"human step {step.name!r} has no {kind} edge", line=line, step=step.name))
            elif len(es) > 1:
                out.append(_fd(proc, "dup_edge", f"human step {step.name!r} has {len(es)} {kind} edges",
                               line=es[1].line, step=step.name))
        for e in edges:
            if e.when in ("done", "outcome"):
                out.append(_fd(proc, "bad_when",
                               f"human step {step.name!r} may only use hil_ok and hil_nok edges",
                               line=e.line, step=step.name))
        return

    for e in edges:
        if e.when in ("hil_ok", "hil_nok"):
            out.append(_fd(proc, "bad_when", f"{e.when} is only allowed on human steps",
                           line=e.line, step=step.name))
    if step.outcomes:
        for o in step.outcomes:
            es = count(lambda e, o=o: e.when == "outcome" and e.outcome == o)
            if not es:
                out.append(_fd(proc, "missing_edge", f"step {step.name!r} has no edge for outcome {o!r}",
                               line=line, step=step.name, item=o))
            elif len(es) > 1:
                out.append(_fd(proc, "dup_edge", f"step {step.name!r} has {len(es)} edges for outcome {o!r}",
                               line=es[1].line, step=step.name, item=o))
        for e in edges:
            if e.when == "outcome" and e.outcome not in step.outcomes:
                out.append(_fd(proc, "unknown_outcome",
                               f"edge uses outcome {e.outcome!r} that step {step.name!r} does not declare",
                               line=e.line, step=step.name, item=e.outcome))
            elif e.when == "done":
                out.append(_fd(proc, "bad_when",
                               f"step {step.name!r} declares outcomes; use 'outcome == <value>' edges",
                               line=e.line, step=step.name))
    else:
        for e in edges:
            if e.when == "outcome":
                out.append(_fd(proc, "unknown_outcome",
                               f"edge uses outcome {e.outcome!r} but step {step.name!r} declares no outcomes",
                               line=e.line, step=step.name, item=e.outcome))
        es = count(lambda e: e.when == "done")
        if not es:
            out.append(_fd(proc, "missing_edge", f"step {step.name!r} has no 'done' edge",
                           line=line, step=step.name))
        elif len(es) > 1:
            out.append(_fd(proc, "dup_edge", f"step {step.name!r} has {len(es)} 'done' edges",
                           line=es[1].line, step=step.name))


def check_transitions(proc: ProcessDef) -> list[Finding]:
    out: list[Finding] = []
    names = {s.name for s in proc.steps}
    if proc.entry not in names:
        out.append(_fd(proc, "unresolved_ref", f"transitions.entry {proc.entry!r} is not a step",
                       item=proc.entry))
    for e in proc.edges:
        if e.src not in names:
            out.append(_fd(proc, "unresolved_ref", f"edge from unknown step {e.src!r}",
                           line=e.line, item=e.src))
        if e.dst != END and e.dst not in names:
            out.append(_fd(proc, "unresolved_ref", f"edge to unknown step {e.dst!r}",
                           line=e.line, step=e.src if e.src in names else None, item=e.dst))
    for s in proc.steps:
        _check_step_edges(proc, s, out)

    reach = set(bfs_order(proc))
    for s in proc.steps:
        if s.name not in reach and proc.entry in names:
            out.append(_fd(proc, "unreachable", f"step {s.name!r} is not reachable from the entry",
                           line=s.line, step=s.name))
    # dead ends: reachable steps from which END cannot be reached
    can_end = {e.src for e in proc.edges if e.dst == END}
    changed = True
    while changed:
        changed = False
        for e in proc.edges:
            if e.src not in can_end and e.dst in can_end:
                can_end.add(e.src)
                changed = True
    for s in proc.steps:
        if s.name in reach and s.name not in can_end:
            out.append(_fd(proc, "dead_end", f"step {s.name!r} cannot reach END", line=s.line, step=s.name))
    return out
