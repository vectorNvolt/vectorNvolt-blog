"""S1 start (SFR-AGG-002, SFR-AGG-037, SFR-AGG-040, SSR-AGG-002).

Opens the run: takes the active-run lock, checks the start preconditions and the process
listing. A failure goes to S7 with exit state NOT_STARTED; before a run folder exists that S7 is
text-only. With ``resume_run_id`` it loads the run and re-enters its saved state: S6 (or S7 when
the run was suspended in resolution); a run suspended in S2-S5 re-validates from S3.
"""

from ..errors import ExitState, Finding, RunError
from ..process.catalog import list_processes
from ..services.preflight import check_preconditions
from . import Env

SRC = "S1 start"


def _fail(message: str, findings=()) -> dict:
    err = RunError(ExitState.NOT_STARTED, "S1", message, findings=tuple(findings))
    return {"phase": "S7", "error": err.to_json()}


def run(env: Env, state: dict) -> dict:
    resume = state.get("resume_run_id")
    if not env.lock.held:
        holder = env.lock.acquire(resume)
        if holder is not None:
            return _fail(f"another agent_graph run is active in this agent (run {holder})")
    findings = check_preconditions(env.ws, env.claude_dir)
    if findings:
        return _fail("start preconditions are not met", findings)

    if resume and env.record is None:
        return _resume(env, resume)

    if not list_processes(env.ws):
        return _fail("no process file in /workspace/.process/",
                     [Finding("no_process", "add a process file (apiVersion: agent-graph/v1) to .process/",
                              file=".process")])
    return {"phase": "S2", "error": None}


def _resume(env: Env, run_id: str) -> dict:
    problems = env.load_run(run_id)
    rec = env.record
    env.gateway.say(SRC, f"Resuming run {run_id} (saved state {rec.phase}, status {rec.status}).")
    if rec.phase == "S7":
        err = rec.open_error or RunError(ExitState.NODE_FAILED, "S6",
                                         "the run was suspended in resolution")
        return {"phase": "S7", "error": err.to_json(), "run_id": run_id, "process_id": rec.process_id,
                "process_file": rec.process_file}
    common = {"error": None, "run_id": run_id, "process_id": rec.process_id,
              "process_file": rec.process_file}
    if rec.phase == "S6":
        if problems or env.spec is None or env.index is None or env.binding is None:
            err = RunError(ExitState.PROCESS_INVALID, "S3",
                           "the saved run state cannot be loaded; the process is validated again",
                           findings=tuple(Finding("resume_state", p) for p in problems))
            return {"phase": "S7", "error": err.to_json(), **common}
        env.start_mode = "restart" if rec.reentry is not None else "resume"
        return {"phase": "S6", **common}
    # suspended before the graph existed (S2-S5): validate again from S3 (idempotent)
    return {"phase": "S3", **common}
