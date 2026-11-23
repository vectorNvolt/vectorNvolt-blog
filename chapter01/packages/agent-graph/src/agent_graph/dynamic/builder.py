"""Builds graph.json (GraphSpec) and compiles the process into the dynamic LangGraph machine.

Covers: SFR-AGG-010 (one node per step, only the edges the process defines, typed reference
state, graph recorded), SSR-AGG-005 (DynState holds references and short summaries), SSR-AGG-004
(every transition is a process edge; the reserved START router is marked ``engine_entry``).
A compile failure (for example a step name that LangGraph rejects) raises ValueError; S5 reports
BUILD_FAILED.
"""

from collections import deque

from langgraph.graph import END, START, StateGraph
from langgraph.runtime import Runtime

from ..ports import DynamicContext
from ..services.runfs import canonical_json, sha256_bytes
from ..state import DynState, GraphNode, GraphSpec, ProcessDef, StepDef
from .router import make_entry_router, make_router
from .step_cognitive import run_cognitive
from .step_human import run_human
from .step_procedure import run_procedure

_RUNNERS = {"procedure": run_procedure, "cognitive": run_cognitive, "human": run_human}


def _bfs_order(proc: ProcessDef) -> "tuple[str, ...]":
    seen = [proc.entry]
    queue = deque([proc.entry])
    while queue:
        cur = queue.popleft()
        for e in proc.edges:
            if e.src == cur and e.dst != "END" and e.dst not in seen:
                seen.append(e.dst)
                queue.append(e.dst)
    seen.extend(s.name for s in proc.steps if s.name not in seen)      # unreachable (invalid) steps last
    return tuple(seen)


def build_graph_spec(proc: ProcessDef) -> GraphSpec:
    pj = proc.to_json()
    nodes = tuple(GraphNode(
        name=s.name, type=s.type, step_hash=sha256_bytes(canonical_json(pj["steps"][i]).encode("utf-8")),
        reads=tuple(r.raw for r in s.reads), produces=tuple(p.name for p in s.produces))
        for i, s in enumerate(proc.steps))
    return GraphSpec(process_id=proc.id, process_sha256=proc.source_sha256, entry=proc.entry, nodes=nodes,
                     edges=tuple(proc.edges), order=_bfs_order(proc), process=pj)


def _make_node(step: StepDef):
    runner = _RUNNERS[step.type]

    def node(state: DynState, runtime: Runtime[DynamicContext]) -> dict:
        return runner(step, state, runtime.context)
    node.__name__ = f"node_{step.name}"
    return node


def compile_dynamic(proc: ProcessDef, checkpointer):
    """One node and one conditional router per step, built only from ``proc.edges``."""
    g = StateGraph(DynState, context_schema=DynamicContext)
    names = [s.name for s in proc.steps]
    for step in proc.steps:
        g.add_node(step.name, _make_node(step))
    g.add_conditional_edges(START, make_entry_router(proc), {n: n for n in names})
    for step in proc.steps:
        dsts = {e.dst for e in proc.edges if e.src == step.name}
        g.add_conditional_edges(step.name, make_router(proc, step),
                                {d: (END if d == "END" else d) for d in dsts})
    return g.compile(checkpointer=checkpointer)
