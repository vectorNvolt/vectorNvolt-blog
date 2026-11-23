"""S7 resolution (SFR-AGG-033, 034, 035, 029, SSR-AGG-016).

Every exception flow of S1-S6 and every escalation of the engine ends here instead of ending
the run. A Claude instance (read-only tools) explains the error through the gateway and proposes
fixes; the runtime applies a fix only after the developer approved that change, through
``ResolutionStore``, which records it in ``resolution/<n>/``. At exit the hashes taken at entry
and exit decide where the run re-enters (``decide_reentry``). The run ends only on the
developer's explicit quit (to S8, checkpoint kept, ``--resume`` re-enters S7).

Before a run folder exists (errors of S1 and S2) S7 is a text-only menu. When Claude is
unavailable or returns no valid reply, or the dialog reaches ``MAX_TURNS`` without the developer's
decision (the limit never counts as resolved), S7 switches to a text-only menu (check again, set
an override, quit) that waits for the developer's explicit choice.
"""

from dataclasses import replace

from ..errors import ExitState, RunError, WriteBoundaryError
from ..process import decide_reentry, find_entry, list_processes, snapshot
from ..prompts.resolution import resolution_first_prompt, resolution_followup
from ..services.render import format_findings
from ..services.resolution_store import ResolutionStore
from . import Env
from ._dialog import Dialog, DialogUnavailable

SRC = "S7 resolution"
MAX_TURNS = 40
PREVIEW_LINES = 40
_NEEDS = {"S4": ("proc",), "S5": ("proc", "binding"), "S6": ("spec", "proc", "index", "binding")}


def _explain(err: RunError) -> str:
    lines = [f"The run stopped: {err.exit_state.value} (state {err.state}"
             + (f", node {err.node}" if err.node else "") + ")", err.message]
    if err.findings:
        lines.append(format_findings(err.findings))
    return "\n".join(lines)


def _quit(err: RunError) -> dict:
    return {"phase": "S8", "quit": True, "exit_state": err.exit_state.value, "error": err.to_json()}


def run(env: Env, state: dict) -> dict:
    err = RunError.from_json(state["error"])
    rec = env.record
    if rec is not None:
        rec.phase, rec.status, rec.open_error = "S7", "ESCALATED", err
        env.save_record()
    with env.gateway.conversation(SRC):
        env.gateway.say(SRC, _explain(err))
        if env.paths is None:
            return _prerun(env, err)
        return _resolve(env, err)


# ------------------------------------------------------------------ before a run folder exists
def _prerun(env: Env, err: RunError) -> dict:
    gw = env.gateway
    while True:
        idx = gw.choose(SRC, "How do you want to continue?",
                        ["Check again (after you fixed the cause)", "Quit"])
        if idx == 0:
            return {"phase": err.state if err.state in ("S1", "S2") else "S1", "error": None}
        if gw.confirm(SRC, "Quit?"):
            return _quit(err)


# ------------------------------------------------------------------ with a run folder
def _resolve(env: Env, err: RunError) -> dict:
    gw, rec, paths, ws = env.gateway, env.record, env.paths, env.ws
    rec.resolution_count += 1
    rdir = paths.resolution_dir(rec.resolution_count)
    store = ResolutionStore(ws, env.guard, rdir, paths.overrides_json)
    before = snapshot(ws, paths, rec.process_file, env.binding)
    env.snapshot_before = before
    chosen = {"process": None}
    last_say = ""
    quit_ = False
    resolved = False
    text_only = False
    dialog = None
    prompt = resolution_first_prompt(err, rec.process_file)
    for _ in range(MAX_TURNS):
        if dialog is None:
            dialog = Dialog(env, rdir / "claude", rdir / "transcript.jsonl", SRC)
        try:
            r = dialog.turn(prompt)
        except DialogUnavailable as exc:
            gw.notice(SRC, "LLM_UNAVAILABLE", f"Claude is unavailable ({exc}); using a menu instead.")
            text_only = True
            break
        gw.say(SRC, r.say, from_llm=True)
        last_say = r.say
        results = [_proposal(env, store, p, chosen) for p in r.proposals]
        if r.action == "done":
            if gw.confirm(SRC, "Re-enter the run now?"):
                resolved = True
                break
        elif results:
            prompt = resolution_followup(results, "")
            continue
        answer = gw.ask(SRC, "Your reply (/resolved when the cause is fixed, /quit to leave the run "
                             "as it is):", allow_quit=True, commands=("/resolved",))
        if answer.quit:
            quit_ = True
            break
        if answer.command == "/resolved":
            resolved = True
            break
        prompt = resolution_followup(results, answer.text)
    else:
        # SFR-AGG-033/035: the turn limit is not a resolution; only the developer decides.
        gw.notice(SRC, "RESOLUTION_LIMIT",
                  f"The resolution dialog reached its limit of {MAX_TURNS} turns without a decision; "
                  "using a menu instead.")
        text_only = True
    while text_only and not (resolved or quit_):
        outcome = _menu(env, store, err)
        quit_ = outcome == "quit"
        resolved = outcome == "retry"

    after = snapshot(ws, paths, rec.process_file, env.binding)
    if quit_:
        store.finish(before, after, None)
        env.save_record()
        return _quit(err)

    new_proc = chosen["process"]
    re = decide_reentry(err.state, before, after,
                        process_choice_changed=bool(new_proc) and new_proc != rec.process_id)
    if re.state in _NEEDS and any(getattr(env, a) is None for a in _NEEDS[re.state]):
        re = replace(re, state="S3")                  # the run context was not loaded: rebuild it
    summary = _summary(store.changes(), last_say)
    re = replace(re, summary=summary,
                 restart_node=err.node if re.state not in ("S1", "S2") else None)
    store.finish(before, after, re.to_json())
    if summary and not re.new_run:
        env.facts().append("resolution", err.node, summary)
    if re.new_run:                                    # SFR-AGG-034: S2 with a new runID, old run kept
        env.save_record()
        env.reset_run()
        return {"phase": "S2", "error": None, "reentry": re.to_json(), "process_id": new_proc}
    rec.reentry, rec.phase, rec.status = re, re.state, "ACTIVE"
    env.save_record()
    return {"phase": re.state, "error": None, "reentry": re.to_json()}


def _summary(changes: list[dict], last_say: str) -> str:
    if changes:
        return "; ".join(f"{c['kind']} {c.get('path') or c.get('key') or c.get('process_id')}: "
                         f"{c.get('reason', '')}".strip() for c in changes)[:500]
    return (last_say or "Resolved without recorded changes.")[:500]


def _preview(content: "str | None") -> str:
    if content is None:
        return ""
    lines = content.splitlines()
    shown = "\n".join(lines[:PREVIEW_LINES])
    more = f"\n... ({len(lines) - PREVIEW_LINES} more line(s))" if len(lines) > PREVIEW_LINES else ""
    return "\n---\n" + shown + more + "\n---"


def _proposal(env: Env, store: ResolutionStore, p: dict, chosen: dict) -> str:
    """Show one proposal, ask the developer, apply it when approved. Returns the outcome line."""
    gw = env.gateway
    kind, reason = p.get("kind"), p.get("reason", "")
    try:
        if kind == "edit_file":
            path, content = p.get("path"), p.get("content")
            store.target(path)                                  # refuse bad paths before asking
            gw.say(SRC, f"Proposed change: {'delete' if content is None else 'write'} {path}\n"
                        f"Reason: {reason}{_preview(content)}")
            if not gw.confirm(SRC, "Apply this change?"):
                return f"edit {path}: rejected by the developer"
            store.apply_edit(path, content, reason, approved=True)
            return f"edit {path}: applied"
        if kind == "override":
            key, node, value = p.get("key"), p.get("node"), p.get("value")
            gw.say(SRC, f"Proposed override: {key}{f' of {node}' if node else ''} = {value}\nReason: {reason}")
            if not gw.confirm(SRC, "Apply this override?"):
                return f"override {key}: rejected by the developer"
            store.apply_override(key, value, node, reason, approved=True)
            return f"override {key}: applied"
        if kind == "choose_process":
            pid = p.get("value")
            if not isinstance(pid, str) or find_entry(list_processes(env.ws), pid) is None:
                return f"choose_process {pid!r}: not a selectable process id"
            gw.say(SRC, f"Proposed: run process {pid} instead (a new run is started).\nReason: {reason}")
            if not gw.confirm(SRC, "Switch to this process?"):
                return f"choose_process {pid}: rejected by the developer"
            store.record_choice(pid, reason)
            chosen["process"] = pid
            return f"choose_process {pid}: accepted"
        return f"unknown proposal kind {kind!r}"
    except (WriteBoundaryError, ValueError, OSError, TypeError) as exc:
        gw.say(SRC, f"The change was refused: {exc}")
        return f"{kind}: refused ({exc})"


def _menu(env: Env, store: ResolutionStore, err: RunError) -> str:
    """Text-only resolution. Returns 'retry' (re-enter), 'quit' or 'stay' (ask again)."""
    gw = env.gateway
    idx = gw.choose(SRC, "How do you want to continue?",
                    ["Re-check now (the cause is fixed outside this run)",
                     "Set a run override (retry, timeout, budgets.max_steps, ...)",
                     "Quit and leave the run as it is"])
    if idx == 0:
        return "retry"
    if idx == 2:
        return "quit" if gw.confirm(SRC, "Quit?") else "stay"
    text = gw.ask(SRC, "Enter: <key> <value> [<node>]   (e.g. 'budgets.max_steps 80', "
                       "'retry 3', 'timeout 20m write-summary')").text.split()
    if len(text) not in (2, 3):
        gw.say(SRC, "Expected: <key> <value> [<node>].")
        return "stay"
    try:
        store.apply_override(text[0], text[1], text[2] if len(text) == 3 else None,
                             "set by the developer (text-only resolution)", approved=True)
        gw.say(SRC, f"Override {text[0]} = {text[1]} recorded.")
    except (ValueError, WriteBoundaryError) as exc:
        gw.say(SRC, f"The override was refused: {exc}")
    return "stay"
