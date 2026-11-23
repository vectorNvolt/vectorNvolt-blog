"""E7 suspend: the shared escalation path.

Covers: SFR-AGG-027 (SUSPEND to the dynamic machine, which reports SUSPENDED; the checkpoint and
run.json are written; S6 is left with ESCALATED(RunError) naming node, failure kind and the reasons
of each attempt), SFR-AGG-016/028/029/030 (budget, context, model and output escalations all end here),
SFR-AGG-036 (a developer quit or an interruption leaves the run SUSPENDED and resumable).
The next attempt only happens after resolution (S7) and a RESTART.
"""

from ..contract import Notice, suspend
from ..errors import ExitState, RunError
from .budgets import EngineRuntime


def e7_suspend(rt: EngineRuntime, state: dict) -> dict:
    gw = rt.ctx.gateway
    raw = state.get("escalation")
    if raw:
        err = RunError.from_json(raw)
    else:                                       # the machine itself reported SUSPENDED
        err = RunError(ExitState.SUSPENDED, "S6", "the dynamic machine suspended")
    for item in rt.machine.send(suspend(err.message)):
        if isinstance(item, Notice):
            gw.notice(item.source, item.code, item.text)
    rec = rt.record
    rec.phase = "S6"
    rec.status = "SUSPENDED" if err.exit_state == ExitState.SUSPENDED else "ESCALATED"
    rec.open_error = err
    rt.persist()
    return {"escalation": err.to_json(), "pending": None, "next_cmd": None}
