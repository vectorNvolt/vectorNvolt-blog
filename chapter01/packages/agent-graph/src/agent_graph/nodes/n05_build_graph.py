"""S5 build_graph (SFR-AGG-010, SFR-AGG-011).

Compiles the dynamic state machine from the process (one node per step, one edge per
``transitions.edges`` entry), writes ``graph.json``, shows the outline through the gateway and
waits for the developer's confirmation to start. A compile error is BUILD_FAILED: the error and
the partial graph are written to the run folder and the state goes to S7.

After a resolution (``record.reentry`` set) the graph is compared with the previous
``graph.json``: S6 restarts at the earliest node whose step, edges or input reads changed, else
at the node that failed. The start confirmation is not asked again then.
"""

import json
import traceback
from dataclasses import replace

from ..errors import ExitState, RunError
from ..process import changed_sources, earliest_changed_node, snapshot
from ..services.render import format_outline
from ..services.runfs import canonical_json, write_atomic
from ..state import GraphSpec
from . import Env

SRC = "S5 build_graph"


def _outline(env: Env, spec: GraphSpec) -> str:
    proc = env.proc
    b = proc.runtime.budgets
    human = [s.name for s in proc.steps if s.type == "human"]
    lines = [format_outline(spec, proc),
             f"Human steps: {', '.join(human) if human else 'none'}",
             f"Budgets: max_steps {b.max_steps}, max_llm_calls {b.max_llm_calls}, "
             f"timeout {b.timeout_s}s; step retry {proc.runtime.retry}, "
             f"step timeout {proc.runtime.timeout_s}s"]
    return "\n".join(lines)


def run(env: Env, state: dict) -> dict:
    env.set_phase("S5")
    rec, paths, gw = env.record, env.paths, env.gateway
    old = None
    if paths.graph_json.is_file():
        try:
            old = GraphSpec.from_json(json.loads(paths.graph_json.read_text(encoding="utf-8")))
        except (OSError, ValueError, KeyError, TypeError):
            old = None
    spec = None
    try:
        from ..dynamic.builder import build_graph_spec, compile_dynamic
        from langgraph.checkpoint.memory import InMemorySaver
        spec = build_graph_spec(env.proc)
        compile_dynamic(env.proc, InMemorySaver())
    except Exception as exc:                                    # any compile problem is BUILD_FAILED
        write_atomic(paths.graph_error_json, canonical_json({
            "type": type(exc).__name__, "message": str(exc),
            "trace": traceback.format_exc().splitlines()[-12:]}), guard=env.guard)
        detail = {"graph_error": paths.graph_error_json.name}
        if spec is not None:
            write_atomic(paths.graph_json, canonical_json(spec.to_json()), guard=env.guard)
            detail["partial_graph"] = paths.graph_json.name
        err = RunError(ExitState.BUILD_FAILED, "S5", f"the dynamic state machine cannot be built: {exc}",
                       detail=detail)
        return {"phase": "S7", "error": err.to_json()}
    write_atomic(paths.graph_json, canonical_json(spec.to_json()), guard=env.guard)
    env.spec = spec

    re = rec.reentry
    if re is not None and (old is None or not paths.history_jsonl.is_file()):
        rec.reentry = re = None                 # the engine never ran: this is the first start
    if re is not None:
        changed = set()
        if env.snapshot_before is not None:
            after = snapshot(env.ws, paths, rec.process_file, env.binding)
            changed = changed_sources(env.snapshot_before, after)
        node = earliest_changed_node(old, spec, changed) if old is not None else None
        node = node or re.restart_node
        rec.reentry = replace(re, state="S6", restart_node=node if node in spec.order else None)
        env.start_mode = "restart"
    else:
        env.start_mode = "start"
    env.save_record()

    with gw.conversation(SRC):
        gw.say(SRC, _outline(env, spec))
        if re is None:
            while not gw.confirm(SRC, "Start the run?"):
                answer = gw.ask(SRC, "Not started. Press Enter to see the outline again, "
                                     "or /quit to leave the run as it is.", allow_quit=True)
                if answer.quit:
                    return {"phase": "S8", "quit": True, "exit_state": ExitState.INTENT_REJECTED.value,
                            "error": None}
                gw.say(SRC, _outline(env, spec))
    return {"phase": "S6", "error": None}
