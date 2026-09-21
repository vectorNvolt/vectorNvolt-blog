"""Message gateway between the user and the Claude CLI running inside the sandbox.

The gateway is the only place that touches the user channel. Inside a LangGraph
node it uses `interrupt()`, so the graph pauses and the driver (app/main.py) resumes
it with `Command(resume=<user text>)`. Swap `ask_user` for a chat/webhook transport
without touching the nodes.
"""
from __future__ import annotations

import json
import re
import time

from langgraph.types import interrupt

from agent_sandbox import config
from agent_sandbox.services import container as ct
from agent_sandbox.services.render import TurnRenderer
from agent_sandbox.services.sanitize import sanitize


def ask_user(prompt: str) -> str:
    """Relay `prompt` to the user and block until the user answers."""
    answer = interrupt({"prompt": prompt})                              # interrupt(arg): sends the prompt to the user and pauses the graph until a response is received.
    return (answer or "").strip()


def notify_user(message: str) -> None:
    print(f"\n[agent] {message}\n")


# --- authentication relay -------------------------------------------------

_ANSI = re.compile(r"\x1b\[[0-9;?]*[A-Za-z]|\x1b\]8;;.*?(\x07|\x1b\\)|\r")


def _clean(s: str) -> str:
    """Login output and CLI stderr: drop the CLI's own colours and links for
    readability (the regex), then mark whatever else is in there (the parser)."""
    return sanitize(_ANSI.sub("", s)).strip()


def is_authenticated(c) -> bool:
    code, _ = ct.exec(c, config.AUTH_STATUS_CMD)
    return code == 0


def start_login(c) -> str:
    """Start `claude auth login` as a live exec session, read until it prompts for the
    code, and return the (URL + instructions) text to relay to the user.
    The session stays open; `finish_login` writes the code into it."""
    s = ct.open_session(c, config.AUTH_LOGIN_CMD)                           # open_session(): creates a new exec session for the container and returns it.

    try:
        out = s.read_until(config.AUTH_CODE_PROMPT, config.AUTH_TIMEOUT)    # out is
    except ct.LineTooLong as e:
        ct.close_session(c)
        return f"login output over the line cap ({e})"
    if config.AUTH_CODE_PROMPT not in out:
        ct.close_session(c)
        return f"login did not reach the code prompt:\n{_clean(out)}"
    return _clean(out)


def finish_login(c, user_reply: str) -> tuple[bool, str]:               # returns a tuple where the first element indicates success and the second element contains a message
    """Feed the user's OAuth code into the pending login session. Returns (ok, message)."""
    s = ct.get_session(c)                                               # get_session(): retrieves the existing exec session for the container, or None if no session exists.
                                                                        # session in this context is the active exec session for the container, used to interact with the ongoing login process.
                                                                        # session is created when `start_login` is called and remains open until `finish_login` is invoked.
    if s is None:
        return False, "no pending login session"
    s.write(user_reply.strip() + "\r")                                  # write the user's OAuth code followed by a carriage return to the login session
    try:
        out = s.read_until(config.AUTH_FAIL_MARKER, config.AUTH_TIMEOUT)    # success -> CLI exits (EOF); failure -> marker
    except ct.LineTooLong as e:
        ct.close_session(c)
        return False, f"login output over the line cap ({e})"
    ok = s.exit_code() == 0 and is_authenticated(c)                     # determine if the login was successful: exit code 0 and authenticated
    ct.close_session(c)                                                 # wrong code -> fresh login next loop
    return ok, _clean(out)


# --- task relay -------------------------------------------------------------

def send_prompt(c, text: str) -> str:
    code, out = ct.exec(c, config.PROMPT_CMD, stdin=text)       # code is the exit code of the command executed inside the container,
                                                                # out is the command's output, e.g.: the response from the agent.
    if code != 0:
        return f"[claude exited {code}]\n{out}"
    return out.strip()


# --- persistent chat session --------------------------------------------------
# One `claude -p --input-format stream-json --output-format stream-json` process per
# container, kept alive across turns so the CLI holds the conversation itself.
# stdin: one JSON user message per line; stdout: NDJSON events, `result` ends a turn.

def start_chat(c) -> None:
    ct.open_session(c, config.CHAT_CMD, tty=False)


# --- event schema ---------------------------------------------------------------
# The shape half of trusting the envelope: an event is dispatched only if its type is
# known and every field the renderer reads has the type the renderer expects. Anything
# else is dropped before `feed()` sees it — an unknown type, a `message` that is a
# string, a content block without a `type`, a `duration_ms` that is a list. What the
# fields *say* is still the CLI's claim; the renderer presents them as such.
#
# A spec is {field: (type | nested spec, required)}. A list spec is [item spec].
_STR_OR_NONE = (str, type(None))
_BLOCK = {"type": (str, True), "text": (str, False), "name": (str, False),
          "content": ((str, list), False), "is_error": (bool, False)}
_MESSAGE = {"content": ((str, [_BLOCK]), False)}
_EVENT_SCHEMA = {
    "system": {},
    "stream_event": {"event": ({"type": (str, True),
                                "content_block": ({"type": (str, True), "name": (str, False)}, False),
                                "delta": ({"type": (str, True), "text": (str, False)}, False)}, True)},
    "assistant": {"message": (_MESSAGE, True)},
    "user": {"message": (_MESSAGE, True)},
    "result": {"result": (_STR_OR_NONE, False), "is_error": (bool, False),
               "duration_ms": ((int, float), False), "num_turns": (int, False)},
}


def _conforms(value, spec) -> bool:
    if isinstance(spec, dict):                       # object: check each declared field
        if not isinstance(value, dict):
            return False
        for field, (fspec, required) in spec.items():
            if field not in value:
                if required:
                    return False
                continue
            if not _conforms(value[field], fspec):
                return False
        return True
    if isinstance(spec, list):                       # homogeneous list
        return isinstance(value, list) and all(_conforms(v, spec[0]) for v in value)
    if isinstance(spec, tuple):                      # any of: types and/or nested specs
        return any(_conforms(value, alt) for alt in spec)
    return isinstance(value, spec) and not (spec is int and isinstance(value, bool))


def valid_event(ev) -> bool:
    return isinstance(ev, dict) and ev.get("type") in _EVENT_SCHEMA and _conforms(ev, _EVENT_SCHEMA[ev["type"]])


def _parse_event(line: str) -> dict | None:
    """One NDJSON line → a conforming event, or None. `json.loads` on a deeply nested
    line raises RecursionError, not JSONDecodeError; both mean 'drop the line'."""
    try:
        ev = json.loads(line)
    except (json.JSONDecodeError, RecursionError, ValueError):
        return None
    return ev if valid_event(ev) else None


def send_chat_turn(c, text: str) -> str:
    """Send one user turn, render the streamed events, return the final answer text.
    Any failure of the channel itself — EOF, an over-long line, a timeout — closes the
    session so the next turn starts a fresh process instead of reading stale events."""
    s = ct.get_session(c)
    if s is None:
        return "[no chat session]"
    s.write(json.dumps({"type": "user", "message": {"role": "user", "content": text},
                        "parent_tool_use_id": None, "session_id": "default"}) + "\n")
    r = TurnRenderer()
    deadline = time.monotonic() + config.CHAT_TURN_TIMEOUT
    try:
        while (left := deadline - time.monotonic()) > 0:
            try:
                line = s.readline(timeout=left)
            except ct.LineTooLong as e:
                ct.close_session(c)
                return f"[claude session reset: {e}]"
            if line == "":
                ct.close_session(c)
                return f"[claude exited {s.exit_code()}]\n{_clean(s.stderr)}"
            if not line or not line.strip():
                continue
            ev = _parse_event(line)
            if ev is None:
                r.dropped += 1
                continue
            r.feed(ev)
            if ev["type"] == "result":
                answer = sanitize(ev.get("result") or "")
                return f"[claude error]\n{answer}" if ev.get("is_error") else answer
        # Whatever the process still emits belongs to this turn; a later turn must not
        # read it as its own answer. Close, so node 6 opens a new session next pass.
        ct.close_session(c)
        return "[claude turn timed out — session reset]"
    finally:
        r.finish()


def close_chat(c) -> None:
    s = ct.get_session(c)
    if s is None:
        return
    s.close_stdin()
    deadline = time.monotonic() + config.CHAT_CLOSE_TIMEOUT
    try:
        while (left := deadline - time.monotonic()) > 0 and s.readline(timeout=left) != "":
            pass
    except ct.LineTooLong:
        pass
    ct.close_session(c)
