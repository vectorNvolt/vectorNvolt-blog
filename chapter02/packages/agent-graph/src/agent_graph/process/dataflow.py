"""Product availability: every product a step reads is produced on every path into it (SFR-AGG-007).

Forward "must" analysis over the edges. A human step yields ``<step>_feedback`` only on its
``hil_nok`` edge, so reading it is legal only as an optional read (``name?``). Reads of unknown
products are rejected even when optional. Only steps reachable from the entry are analysed.
"""

from ..errors import Finding
from ..state import ProcessDef
from . import refs
from .transitions import bfs_order


def known_products(proc: ProcessDef) -> frozenset[str]:
    names = {p.name for s in proc.steps for p in s.produces}
    names.update(refs.feedback_name(s.name) for s in proc.steps if s.type == "human")
    return frozenset(names)


def available_before(proc: ProcessDef) -> dict[str, frozenset[str]]:
    """step -> products guaranteed present when the step starts (reachable steps only)."""
    order = bfs_order(proc)
    reach = set(order)
    steps = {s.name: s for s in proc.steps}
    universe = known_products(proc)
    inn: dict[str, frozenset[str]] = {n: universe for n in order}
    if proc.entry in inn:
        inn[proc.entry] = frozenset()

    def out_of(e) -> frozenset[str]:
        src = steps[e.src]
        got = set(inn[e.src]) | {p.name for p in src.produces}
        if e.when == "hil_nok" and src.type == "human":
            got.add(refs.feedback_name(src.name))
        return frozenset(got)

    changed = True
    while changed:
        changed = False
        for n in order:
            if n == proc.entry:
                continue
            incoming = [e for e in proc.edges if e.dst == n and e.src in reach]
            new = frozenset.intersection(*(out_of(e) for e in incoming)) if incoming else frozenset()
            if new != inn[n]:
                inn[n] = new
                changed = True
    return inn


def check_reads(proc: ProcessDef) -> list[Finding]:
    out: list[Finding] = []
    known = known_products(proc)
    avail = available_before(proc)
    for s in proc.steps:
        for r in s.reads:
            if r.kind != "product":
                continue
            if r.name not in known:
                out.append(Finding(code="unresolved_ref",
                                   message=f"step {s.name!r} reads unknown product {r.name!r}",
                                   file=proc.source_file, line=s.line, step=s.name, item=r.name))
            elif s.name in avail and not r.optional and r.name not in avail[s.name]:
                out.append(Finding(code="read_not_on_all_paths",
                                   message=f"step {s.name!r} reads product {r.name!r} that is not produced "
                                           f"on every path into it (use {r.name}? for an optional read)",
                                   file=proc.source_file, line=s.line, step=s.name, item=r.name))
    return out
