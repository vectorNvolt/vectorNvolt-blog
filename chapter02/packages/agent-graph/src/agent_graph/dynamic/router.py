"""Edge selection of the dynamic state machine.

Covers: SFR-AGG-013 (after a node, only the one edge whose condition holds is followed; the
machine never selects a node itself), SFR-AGG-024/025 (hil_nok and outcome edges may lead back),
decision 10 (transition strictness). The only non-process element is the reserved START router
(restart_at, else the process entry), recorded in graph.json as ``engine_entry``.
"""

from typing import Callable, Sequence

from ..state import Edge, ProcessDef, StepDef


class RoutingError(Exception):
    """No edge, or more than one, matches (a validated process never causes this)."""


def select_edge(edges: Sequence[Edge], step: StepDef, state: dict) -> str:
    """Destination ("END" allowed) of the single edge that holds after ``step`` ran."""
    mine = [e for e in edges if e.src == step.name]
    if step.type == "human":
        hil = state.get("last_hil")
        hits = [e for e in mine if e.when == hil]
    elif step.outcomes:
        out = state.get("last_outcome")
        hits = [e for e in mine if e.when == "outcome" and e.outcome == out]
    else:
        hits = [e for e in mine if e.when == "done"]
    if len(hits) != 1:
        raise RoutingError(f"step {step.name!r}: {len(hits)} edges match (outcome={state.get('last_outcome')!r}, "
                           f"hil={state.get('last_hil')!r})")
    return hits[0].dst


def make_router(proc: ProcessDef, step: StepDef) -> Callable[[dict], str]:
    def router(state: dict) -> str:
        return select_edge(proc.edges, step, state)
    router.__name__ = f"route_{step.name}"
    return router


def make_entry_router(proc: ProcessDef) -> Callable[[dict], str]:
    def entry(state: dict) -> str:
        return state.get("restart_at") or proc.entry
    entry.__name__ = "route_entry"
    return entry


def next_after(proc: ProcessDef, values: dict) -> "str | None":
    """Where the machine goes after the last completed node of ``values`` (None: nothing ran yet;
    "END": the last node led to the end)."""
    last = values.get("last_node")
    if not last:
        return None
    return select_edge(proc.edges, proc.step(last), values)
