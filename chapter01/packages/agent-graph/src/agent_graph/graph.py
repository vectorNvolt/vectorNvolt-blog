"""The static master state machine S1-S8 as a LangGraph graph (SSR-AGG-003, SFR-AGG-027).

The same graph for every process: only the dynamic machine compiled in S5 differs. Every state
sets ``phase`` to the state that runs next, so the routing here is a table of the allowed
transitions:

    S1 -> S2 (new run) | S3 (resume, validate again) | S6 (resume) | S7 (error)
    S2 -> S3 | S7 | S8 (intent rejected)
    S3 -> S4 | S7;  S4 -> S5 | S7;  S5 -> S6 | S7 | S8 (start declined)
    S6 -> S8 (completed) | S7 (escalation)
    S7 -> S1..S6 (re-entry after a resolution) | S8 (the developer quit)
    S8 -> END

Interruptions (Ctrl-C, SIGTERM, closed terminal) are exceptions that leave the graph; ``main``
records SUSPENDED (no S7, SFR-AGG-036).
"""

from collections.abc import Callable

from langgraph.graph import END, START, StateGraph

from .config import RECURSION_LIMIT
from .nodes import Env
from .nodes import (n01_start, n02_clarify, n03_parse_process, n04_bind_input, n05_build_graph,
                    n06_execution, n07_resolution, n08_report)
from .state import StaticState

STATES = {
    "S1": n01_start.run, "S2": n02_clarify.run, "S3": n03_parse_process.run,
    "S4": n04_bind_input.run, "S5": n05_build_graph.run, "S6": n06_execution.run,
    "S7": n07_resolution.run, "S8": n08_report.run,
}

TRANSITIONS = {
    "S1": ("S2", "S3", "S6", "S7"),
    "S2": ("S3", "S7", "S8"),
    "S3": ("S4", "S7"),
    "S4": ("S5", "S7"),
    "S5": ("S6", "S7", "S8"),
    "S6": ("S7", "S8"),
    "S7": ("S1", "S2", "S3", "S4", "S5", "S6", "S8"),
}


def _node(env: Env, fn: Callable[[Env, dict], dict]) -> Callable[[StaticState], dict]:
    def node(state: StaticState) -> dict:
        return fn(env, state)
    return node


def _route(src: str) -> Callable[[StaticState], str]:
    allowed = TRANSITIONS[src]

    def route(state: StaticState) -> str:
        nxt = state.get("phase")
        if nxt not in allowed:
            raise RuntimeError(f"{src}: transition to {nxt!r} is not allowed")
        return nxt
    return route


def build_master(env: Env):
    """Compile the static master for this environment (no checkpointer: run.json is the record)."""
    builder = StateGraph(StaticState)
    for name, fn in STATES.items():
        builder.add_node(name, _node(env, fn))
    builder.add_edge(START, "S1")
    for src, targets in TRANSITIONS.items():
        builder.add_conditional_edges(src, _route(src), {t: t for t in targets})
    builder.add_edge("S8", END)
    return builder.compile()


def run_master(env: Env, resume_run_id: "str | None") -> dict:
    """Run S1..S8 and return the final state (``exit_state`` is set by S8)."""
    graph = build_master(env)
    return graph.invoke({"resume_run_id": resume_run_id, "phase": "S1"},
                        config={"recursion_limit": RECURSION_LIMIT})
