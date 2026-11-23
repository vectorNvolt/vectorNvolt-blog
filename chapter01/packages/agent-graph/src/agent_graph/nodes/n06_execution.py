"""S6 execution (SFR-AGG-012, SFR-AGG-027, SFR-AGG-037): a thin wrapper around the S6 engine.

Builds the engine context from the run context and calls ``engine.run_engine``. The engine owns
E1-E7 and the dynamic machine; this state only maps its outcome to the master:

* done                -> S8 (COMPLETED)
* escalation          -> S7 with the engine's RunError (the run is not ended)
* interruption        -> the exception propagates; ``main`` records SUSPENDED (no S7, SFR-AGG-036)

``env.start_mode`` selects how E1 starts: ``start`` (new run), ``resume`` (``--resume``: continue
where the checkpoint stopped) or ``restart`` (after a resolution: RESTART(node) with the summary).
"""

import importlib

from ..errors import ExitState, Interrupted, RunError, TerminalClosed
from ..services.checkpoint import open_checkpointer
from . import Env

SRC = "S6 execution"


def _engine_api():
    engine = importlib.import_module("agent_graph.engine")
    ctx_cls = getattr(engine, "EngineContext", None)
    if ctx_cls is None:
        ctx_cls = importlib.import_module("agent_graph.engine.graph").EngineContext
    return ctx_cls, engine.run_engine


def run(env: Env, state: dict) -> dict:
    rec, paths = env.record, env.paths
    try:
        engine_context, run_engine = _engine_api()
    except (ImportError, AttributeError) as exc:
        err = RunError(ExitState.BUILD_FAILED, "S6", f"the execution engine is not available: {exc}")
        return {"phase": "S7", "error": err.to_json()}

    mode = env.start_mode
    restart_node, summary = None, ""
    if rec.reentry is not None:
        mode = "restart"
        restart_node = rec.reentry.restart_node or (rec.open_error.node if rec.open_error else None)
        summary = rec.reentry.summary
        if restart_node is None:
            mode = "resume"
    env.set_phase("S6")
    checkpointer = open_checkpointer(paths.checkpoint_db)
    try:
        ctx = engine_context(
            paths=paths, process=env.proc, spec=env.spec, index=env.index, binding=env.binding,
            record=rec, gateway=env.gateway, claude=env.claude, facts=env.facts(),
            history=env.history(), checkpointer=checkpointer, mode=mode,
            restart_node=restart_node, restart_summary=summary)
        outcome = run_engine(ctx)
    except (Interrupted, TerminalClosed):
        raise
    except Exception as exc:                        # an engine defect must not end the run (SSR-AGG-016)
        err = RunError(ExitState.NODE_FAILED, "S6", f"unexpected engine error: {exc!r}",
                       node=restart_node)
        return {"phase": "S7", "error": err.to_json()}
    finally:
        checkpointer.close()

    env.start_mode = "resume"
    esc = outcome.escalation
    if not outcome.done and esc is not None and esc.exit_state == ExitState.SUSPENDED:
        # the developer quit in E3: the run stays resumable (SFR-AGG-036), no S7
        rec.status = "SUSPENDED"
        env.save_record()
        return {"phase": "S8", "error": None, "exit_state": ExitState.SUSPENDED.value}
    rec.reentry = None
    if outcome.done:
        rec.open_error = None
        env.save_record()
        return {"phase": "S8", "error": None, "exit_state": ExitState.COMPLETED.value,
                "completed_output": outcome.output_ref}
    err = outcome.escalation or RunError(ExitState.NODE_FAILED, "S6", "the run was escalated")
    return {"phase": "S7", "error": err.to_json()}
