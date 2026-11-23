"""S2 clarify (SFR-AGG-003, SFR-AGG-004, SFR-AGG-029).

Lists the processes (files that share an id are listed as not selectable) and lets a Claude
instance match the developer's intent against id, title and description, asking back through
the gateway until exactly one process matches and the developer confirms it. Then it sets
PROCESS_ID, creates the runID and the run folder. Leaves only on agreement (to S3) or on the
developer's explicit quit or when nothing matches (INTENT_REJECTED, no S7, to S8). When Claude is
unavailable the choice is a numbered menu (text-only fallback).
"""

import shutil

from ..errors import ExitState, RunError
from ..process.catalog import CatalogEntry, duplicate_findings, find_entry, list_processes
from ..prompts.clarify import (clarify_followup, clarify_first_prompt, clarify_rejected,
                               clarify_unknown, format_catalog)
from . import Env
from ._dialog import Dialog, DialogUnavailable

SRC = "S2 clarify"
MAX_TURNS = 20


def run(env: Env, state: dict) -> dict:
    gw = env.gateway
    entries = list_processes(env.ws)
    selectable = [e for e in entries if e.selectable]
    with gw.conversation(SRC):
        gw.say(SRC, "Processes in .process/:\n" + format_catalog(entries))
        if not selectable:
            err = RunError(ExitState.PROCESS_INVALID, "S2",
                           "no selectable process: files share an id or cannot be read",
                           findings=tuple(duplicate_findings(entries)))
            return {"phase": "S7", "error": err.to_json()}
        chosen = None
        preselected = state.get("process_id") if state.get("reentry") else None
        if preselected and find_entry(entries, preselected) is not None:
            chosen = find_entry(entries, preselected)
            if not gw.confirm(SRC, f"Run process {chosen.id} ({chosen.title})?"):
                chosen = None
        if chosen is None:
            chosen = _clarify(env, entries, selectable)
    if chosen is None:
        gw.say(SRC, "No process chosen.")
        return {"phase": "S8", "quit": True, "exit_state": ExitState.INTENT_REJECTED.value,
                "error": None, "reentry": None}
    old_static = env.ws.static_dir / "clarify" / "transcript.jsonl"
    paths = env.create_run(chosen.id, chosen.file)
    if old_static.is_file():                         # SSR-AGG-014: dialogs/S2-1/
        dest = paths.dialogs_dir / "S2-1" / "transcript.jsonl"
        env.guard.check(dest)
        dest.parent.mkdir(parents=True, exist_ok=True)
        shutil.move(str(old_static), str(dest))
    gw.say(SRC, f"Run {paths.run_id} created for process {chosen.id}.")
    return {"phase": "S3", "error": None, "reentry": None, "quit": False, "exit_state": None,
            "run_id": paths.run_id, "process_id": chosen.id, "process_file": chosen.file}


def _clarify(env: Env, entries: list[CatalogEntry], selectable: list[CatalogEntry]) -> "CatalogEntry | None":
    gw = env.gateway
    reply = gw.ask(SRC, "Which process do you want to run? Describe what you want to do.",
                   allow_quit=True)
    if reply.quit:
        return None
    work = env.ws.static_dir / "clarify"
    (work / "transcript.jsonl").unlink(missing_ok=True)      # a rejected earlier dialog
    dialog = Dialog(env, work, work / "transcript.jsonl", SRC)
    prompt = clarify_first_prompt(entries, reply.text)
    for _ in range(MAX_TURNS):
        try:
            r = dialog.turn(prompt)
        except DialogUnavailable as exc:
            gw.notice(SRC, "LLM_UNAVAILABLE", f"Claude is unavailable ({exc}); using a menu instead.")
            return _menu(env, selectable)
        gw.say(SRC, r.say, from_llm=True)
        if r.action in ("agree", "propose", "done") and r.process_id:
            entry = find_entry(entries, r.process_id)
            if entry is None:
                prompt = clarify_unknown(r.process_id)
                continue
            if gw.confirm(SRC, f"Run process {entry.id} ({entry.title})?"):
                return entry
            answer = gw.ask(SRC, "What should be different?", allow_quit=True)
            if answer.quit:
                return None
            prompt = clarify_rejected(entry.id, answer.text)
            continue
        if r.action == "done":                       # nothing matches
            return None
        answer = gw.ask(SRC, "Your answer:", allow_quit=True)
        if answer.quit:
            return None
        prompt = clarify_followup(answer.text)
    return None


def _menu(env: Env, selectable: list[CatalogEntry]) -> "CatalogEntry | None":
    options = [f"{e.id}: {e.title}" for e in selectable] + ["Quit"]
    idx = env.gateway.choose(SRC, "Choose the process to run:", options)
    return selectable[idx] if idx < len(selectable) else None
