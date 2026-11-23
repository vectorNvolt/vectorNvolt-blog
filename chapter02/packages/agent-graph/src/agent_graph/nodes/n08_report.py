"""S8 report (SFR-AGG-032, SFR-AGG-035, SFR-AGG-021).

Writes the run summary to the terminal and, when a run folder exists, to
``.output/<runID>/report.md`` (open error, resolution transcript, drift notices). Sets the exit
state and exit code. A run that ended by completion or by an intent rejection is ENDED; a run the
developer left in S7 keeps its checkpoint and status (``--resume`` re-enters S7).
"""

from ..errors import ExitState, RunError
from ..services.report import build_report, console_summary
from ..services.runfs import write_atomic
from . import Env

SRC = "S8 report"


def run(env: Env, state: dict) -> dict:
    exit_state = ExitState(state.get("exit_state") or ExitState.NODE_FAILED.value)
    error = RunError.from_json(state["error"]) if state.get("error") else None
    rec, paths = env.record, env.paths
    report_path = None
    if rec is not None and paths is not None:
        rec.exit_state, rec.exit_code = exit_state.value, exit_state.exit_code
        if exit_state in (ExitState.COMPLETED, ExitState.INTENT_REJECTED):
            rec.status, rec.phase = "ENDED", "S8"
        env.save_record()
        report_path = paths.report_md
        write_atomic(report_path, build_report(paths, rec, exit_state, error), guard=env.guard)
    env.gateway.say(SRC, console_summary(rec, exit_state, error, report_path))
    return {"phase": "S8", "exit_state": exit_state.value}
