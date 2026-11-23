"""S4 bind_input (SFR-AGG-009, SSR-AGG-008).

Binds every source with a ``path`` glob to the files of ``.input/`` (several candidate sets: the
developer chooses, the runtime never picks), asks the developer for every ``note`` source, checks
presence, type and schema of the bound files and of the templates and rulesets, and writes
``binding.json``. From here on only bound files are read. A failed check is INPUT_INVALID (S7).
On re-entry after a resolution, choices and notes that are still valid are reused.
"""

from ..errors import ExitState, RunError
from ..process import check_binding, find_candidates, make_binding
from ..prompts.note_question import note_question
from ..services.runfs import canonical_json, write_atomic
from . import Env

SRC = "S4 bind_input"


def run(env: Env, state: dict) -> dict:
    env.set_phase("S4")
    gw = env.gateway
    proc, ws = env.proc, env.ws
    previous = env.binding
    candidates = find_candidates(proc, ws)
    chosen: dict[str, tuple[str, ...]] = {}
    notes: dict[str, str] = {}
    with gw.conversation(SRC):
        for s in proc.sources:
            prev = previous.sources.get(s.name) if previous is not None else None
            if s.path is not None:
                sets = candidates.get(s.name, [])
                prev_files = tuple(f.path for f in prev.files) if prev is not None else ()
                if len(sets) == 1:
                    chosen[s.name] = sets[0]
                elif len(sets) > 1:
                    if prev_files in sets:
                        chosen[s.name] = prev_files
                    else:
                        labels = [f"{', '.join(x.rsplit('/', 1)[-1] for x in cand)} "
                                  f"(in {cand[0].rsplit('/', 1)[0]})" for cand in sets]
                        idx = gw.choose(SRC, f"Source '{s.name}': {len(sets)} candidate sets match "
                                             f"'{s.path}'. Which one is bound?", labels)
                        chosen[s.name] = sets[idx]
            if s.note is not None:
                if prev is not None and prev.note_text and prev.note_text.strip():
                    notes[s.name] = prev.note_text
                else:
                    notes[s.name] = gw.ask(SRC, note_question(s)).text
        binding = make_binding(proc, ws, chosen, notes)
        findings = check_binding(proc, binding, ws)
    if findings:
        err = RunError(ExitState.INPUT_INVALID, "S4",
                       f"the input does not match the process ({len(findings)} finding(s))",
                       findings=tuple(findings))
        return {"phase": "S7", "error": err.to_json()}
    write_atomic(env.paths.binding_json, canonical_json(binding.to_json()), guard=env.guard)
    env.binding = binding
    bound = sum(len(b.files) for b in binding.sources.values())
    gw.say(SRC, f"Input bound: {bound} file(s) from {len(binding.sources)} source(s); "
                f"{len(binding.templates)} template(s), {len(binding.rulesets)} ruleset(s).")
    return {"phase": "S5", "error": None}
