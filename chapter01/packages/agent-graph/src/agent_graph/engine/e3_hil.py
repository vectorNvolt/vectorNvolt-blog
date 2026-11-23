"""E3 hil: the developer's side of a waiting node (a question or a human review step).

Covers: SFR-AGG-022 (the node's question is asked through the gateway tagged with the node;
ANSWER goes back to the same session and the answer becomes a run fact), SFR-AGG-023 (review brief
and items presented through the gateway, ``/show <n>`` shows an item, approval is HIL_OK and a run
fact), SFR-AGG-024 (a rejection is HIL_NOK(feedback) and a run fact), SSR-AGG-001 (all terminal I/O
through the gateway), decision 12 (no Claude in the review discussion).

A quit of the developer here suspends the run (escalation with exit state SUSPENDED); the run
keeps its checkpoint and is resumable. A human step waits at most its own timeout (SFR-AGG-016):
every gateway call of the review gets the time left (``timeout_s``), and ``ReplyTimeout`` - or a
decision that came in after the limit, the fallback check - escalates as NODE_FAILED(timeout).
"""

import time

from ..contract import Status, answer, hil_nok, hil_ok
from ..errors import ExitState, ReplyTimeout, RunError
from ..services.runfs import InputReadGuard
from .budgets import EngineRuntime

APPROVE_WORDS = frozenset({"ok", "okay", "approve", "approved", "y", "yes", "lgtm"})
_SHOW_MAX = 20_000


def _suspended(node: "str | None", message: str) -> dict:
    err = RunError(ExitState.SUSPENDED, "S6", message, node=node)
    return {"escalation": err.to_json(), "next_cmd": None, "pending": None}


def _show(rt: EngineRuntime, source: str, reads: list, arg: str) -> None:
    gw = rt.ctx.gateway
    try:
        n = int(arg.strip())
        item = reads[n - 1]
        if n < 1:
            raise IndexError
    except (ValueError, IndexError):
        gw.say(source, f"/show needs a number between 1 and {len(reads)}")
        return
    text = item.get("inline")
    if item.get("path"):
        try:
            text = InputReadGuard(rt.ctx.paths.ws, rt.ctx.binding).open_text(rt.ctx.paths.ws.root / item["path"])
        except (OSError, UnicodeDecodeError, ValueError) as exc:
            if text is None:
                gw.say(source, f"cannot show {item['path']}: {exc}")
                return
    if text is None:
        gw.say(source, f"{item['ref']} is not available")
        return
    gw.say(source, text[:_SHOW_MAX])


def _input(rt: EngineRuntime, st: Status) -> dict:
    gw, node = rt.ctx.gateway, st.node
    source = f"node {node}"
    question = str(st.payload.get("question", ""))
    with gw.conversation(source):
        gw.say(source, question, from_llm=True)
        rep = gw.ask(source, "Your answer?", allow_quit=True)
    if rep.quit:
        return _suspended(node, f"the developer quit at the question of node {node}")
    rt.ctx.facts.append("answer", node, f"{question}\n-> {rep.text}")
    return {"next_cmd": answer(node, rep.text).to_json(), "pending": None}


def _timed_out(node: str, limit: float) -> dict:
    err = RunError(ExitState.NODE_FAILED, "S6", f"human step {node} timed out after {limit}s", node=node,
                   detail={"kind": "timeout", "reason": "the review took longer than the step timeout",
                           "attempts": [f"timeout: no decision within {limit}s"]})
    return {"escalation": err.to_json(), "next_cmd": None, "pending": None}


def _hil(rt: EngineRuntime, st: Status) -> dict:
    gw, node = rt.ctx.gateway, st.node
    source = f"node {node}"
    reads = list(st.payload.get("reads") or [])
    started = rt.human_started.get(node)
    limit = rt.proc.timeout_of(rt.proc.step(node))
    if limit and started is None:
        started = rt.human_started[node] = time.monotonic()    # e.g. after a resume: count from now

    def left() -> "float | None":
        return None if not limit else max(0.0, limit - (time.monotonic() - started))

    decision: "tuple[str, str] | None" = None
    try:
        with gw.conversation(source):
            gw.say(source, f"Review requested.\n{st.payload.get('brief', '')}")
            for i, r in enumerate(reads, 1):
                where = r["path"] if r.get("path") else "not available"
                gw.say(source, f"  [{i}] {r['ref']} ({where})")
            while decision is None:
                rep = gw.ask(source, "Approve with 'ok', or type your feedback to reject. /show <n> shows an item.",
                             allow_quit=True, commands=("/show",), timeout_s=left())
                if rep.quit:
                    return _suspended(node, f"the developer quit at the review of node {node}")
                if rep.command == "/show":
                    _show(rt, source, reads, rep.text)
                    continue
                if rep.command:
                    gw.say(source, f"unknown command {rep.command}")
                    continue
                text = rep.text.strip()
                if not text:
                    continue
                if text.lower() in APPROVE_WORDS:
                    decision = ("ok", "approved")
                elif gw.confirm(source, f"Send the step back with this feedback?\n{text}", timeout_s=left()):
                    decision = ("nok", text)
    except ReplyTimeout:
        return _timed_out(node, limit)
    if limit and time.monotonic() - started > limit:          # fallback: a reply that came in too late
        return _timed_out(node, limit)
    kind, text = decision
    if kind == "ok":
        rt.ctx.facts.append("hil_ok", node, text)
        return {"next_cmd": hil_ok(node).to_json(), "pending": None}
    rt.ctx.facts.append("hil_nok", node, text)
    return {"next_cmd": hil_nok(node, text).to_json(), "pending": None}


def e3_hil(rt: EngineRuntime, state: dict) -> dict:
    st = Status.from_json(state["pending"])
    mode = st.payload.get("mode")
    if mode == "INPUT":
        return _input(rt, st)
    if mode == "HIL":
        return _hil(rt, st)
    raise ValueError(f"WAITING with unknown mode {mode!r}")
