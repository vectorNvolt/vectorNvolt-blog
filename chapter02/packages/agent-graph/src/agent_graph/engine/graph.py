"""The S6 execution engine as a LangGraph subgraph StateGraph(EngineState) with nodes E1..E7.

Covers: SFR-AGG-012..016, 021..031, 037 (see the e*.py modules). The engine graph has no
checkpointer: its durable data are the counters in run.json, the dynamic graph's checkpoint and
history.jsonl. EngineState holds only small JSON (mode, pending status, next command, escalation).

Routing: E1 -> E2; after E2 by the pending status (WAITING -> E3, NODE_FAILED -> E4,
COMPLETED -> E5, SUSPENDED -> E7); E3 and E4 -> E2 (next command) or E7 (escalation);
E5 -> E6 or E7; E6 and E7 -> end.
"""

from langgraph.graph import END, START, StateGraph

from ..config import RECURSION_LIMIT
from ..errors import Interrupted, RunError, TerminalClosed
from ..state import EngineState
from .budgets import (EngineContext, EngineOutcome, EngineRuntime, RunBudgets, apply_overrides,
                      load_overrides)
from .e1_load import e1_load
from .e2_supervise import e2_supervise
from .e3_hil import e3_hil
from .e4_failure_policy import e4_failure_policy
from .e5_output_check import e5_output_check
from .e6_cleanup import e6_cleanup
from .e7_suspend import e7_suspend

_BY_PENDING = {"WAITING": "e3_hil", "NODE_FAILED": "e4_failure_policy",
               "COMPLETED": "e5_output_check", "SUSPENDED": "e7_suspend"}


def route_pending(state: dict) -> str:
    if state.get("escalation"):
        return "e7_suspend"
    pend = state.get("pending")
    return _BY_PENDING[pend["name"]] if pend else "e2_supervise"


def route_e5(state: dict) -> str:
    return "e7_suspend" if state.get("escalation") else "e6_cleanup"


def build_engine_graph(rt: EngineRuntime):
    def n1(state: EngineState) -> dict:
        return e1_load(rt, state)

    def n2(state: EngineState) -> dict:
        return e2_supervise(rt, state)

    def n3(state: EngineState) -> dict:
        return e3_hil(rt, state)

    def n4(state: EngineState) -> dict:
        return e4_failure_policy(rt, state)

    def n5(state: EngineState) -> dict:
        return e5_output_check(rt, state)

    def n6(state: EngineState) -> dict:
        return e6_cleanup(rt, state)

    def n7(state: EngineState) -> dict:
        return e7_suspend(rt, state)

    g = StateGraph(EngineState)
    for name, fn in (("e1_load", n1), ("e2_supervise", n2), ("e3_hil", n3), ("e4_failure_policy", n4),
                     ("e5_output_check", n5), ("e6_cleanup", n6), ("e7_suspend", n7)):
        g.add_node(name, fn)
    g.add_edge(START, "e1_load")
    full = {n: n for n in ("e2_supervise", "e3_hil", "e4_failure_policy", "e5_output_check", "e7_suspend")}
    for src in ("e1_load", "e2_supervise", "e3_hil", "e4_failure_policy"):
        g.add_conditional_edges(src, route_pending, full)
    g.add_conditional_edges("e5_output_check", route_e5, {"e6_cleanup": "e6_cleanup", "e7_suspend": "e7_suspend"})
    g.add_edge("e6_cleanup", END)
    g.add_edge("e7_suspend", END)
    return g.compile()


def _abort(rt: EngineRuntime) -> None:
    """SIGINT / closed terminal: stop the running attempt and leave the run SUSPENDED."""
    rt.budgets.disarm_attempt()
    if rt.machine is not None:
        rt.machine.abort("interrupted")
    rt.budgets.pause_clock()
    rt.record.status = "SUSPENDED"
    rt.record.phase = "S6"
    rt.persist()


def run_engine(ctx: EngineContext) -> EngineOutcome:
    """Run S6 to the end: done (output checked, work products cleaned) or an escalation.
    ``Interrupted`` and ``TerminalClosed`` leave the run SUSPENDED and propagate to the caller."""
    proc = apply_overrides(ctx.process, load_overrides(ctx.paths.overrides_json))
    budgets = RunBudgets(ctx.record.counters, proc.runtime.budgets)
    rt = EngineRuntime(ctx=ctx, proc=proc, guard=ctx.facts.guard, budgets=budgets)
    graph = build_engine_graph(rt)
    try:
        final = graph.invoke({"mode": ctx.mode, "restart_node": ctx.restart_node,
                              "restart_summary": ctx.restart_summary}, {"recursion_limit": RECURSION_LIMIT})
    except (Interrupted, TerminalClosed):
        _abort(rt)
        raise
    finally:
        budgets.close()
        if rt.journal is not None:
            rt.journal.close()
    if final.get("escalation"):
        return EngineOutcome(False, None, RunError.from_json(final["escalation"]))
    return EngineOutcome(bool(final.get("done")), final.get("output_ref"), None)
