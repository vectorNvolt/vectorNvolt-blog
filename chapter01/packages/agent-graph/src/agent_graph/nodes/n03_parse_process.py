"""S3 parse_process (SFR-AGG-005, 006, 007, 008, 004).

Loads the selected process, validates structure, transitions, data flow and the skill and snippet
declarations against the pools, indexes the pools with their SHA-256 and writes ``index.json``.
Any finding leaves the run in PROCESS_INVALID (to S7); nothing runs before this passes.
"""

from ..compose import compose_system_md
from ..errors import ExitState, RunError
from ..process import (build_index, duplicate_findings, list_processes, load_process,
                       validate_against_pools, validate_process, write_index)
from . import Env

SRC = "S3 parse_process"


def run(env: Env, state: dict) -> dict:
    env.set_phase("S3")
    rec = env.record
    proc, findings = load_process(env.ws.root / rec.process_file, env.ws)
    index = None
    if proc is not None:
        findings = list(validate_process(proc, env.ws))
        index, index_findings = build_index(env.ws)
        findings += index_findings
        findings += validate_against_pools(proc, index, system_md=compose_system_md)
        findings += [f for f in duplicate_findings(list_processes(env.ws)) if f.item == proc.id]
    if findings:
        err = RunError(ExitState.PROCESS_INVALID, "S3",
                       f"process {rec.process_file} is not valid ({len(findings)} finding(s))",
                       findings=tuple(findings))
        return {"phase": "S7", "error": err.to_json()}
    write_index(index, env.paths, env.guard)
    env.proc, env.index = proc, index
    if proc.id != rec.process_id:
        rec.process_id = proc.id
    rec.process_file = proc.source_file
    env.save_record()
    env.gateway.say(SRC, f"Process {proc.id} is valid: {len(proc.steps)} step(s), "
                         f"{len(index.skills)} skill(s) and {len(index.snippets)} snippet(s) indexed.")
    return {"phase": "S4", "error": None, "process_id": proc.id, "process_file": proc.source_file}
