"""Message gateway between the user and the Claude CLI running inside the sandbox.

The gateway is the only place that touches the user channel. Inside a LangGraph
node it uses `interrupt()`, so the graph pauses and the driver (app/main.py) resumes
it with `Command(resume=<user text>)`. Swap `ask_user` for a chat/webhook transport
without touching the nodes.
"""
from __future__ import annotations

import re

from langgraph.types import interrupt

from agent_sandbox import config
from agent_sandbox.services import container as ct


def ask_user(prompt: str) -> str:
    """Relay `prompt` to the user and block until the user answers."""
    answer = interrupt({"prompt": prompt})                              # interrupt(arg): sends the prompt to the user and pauses the graph until a response is received.
    return (answer or "").strip()


def notify_user(message: str) -> None:
    print(f"\n[agent] {message}\n")


# --- authentication relay -------------------------------------------------

_ANSI = re.compile(r"\x1b\[[0-9;?]*[A-Za-z]|\x1b\]8;;.*?(\x07|\x1b\\)|\r")


def _clean(s: str) -> str:
    return _ANSI.sub("", s).strip()


def is_authenticated(c) -> bool:
    code, _ = ct.exec(c, config.AUTH_STATUS_CMD)
    return code == 0


def start_login(c) -> str:
    """Start `claude auth login` as a live exec session, read until it prompts for the
    code, and return the (URL + instructions) text to relay to the user.
    The session stays open; `finish_login` writes the code into it."""
    s = ct.open_session(c, config.AUTH_LOGIN_CMD)                           # open_session(): creates a new exec session for the container and returns it.

    out = s.read_until(config.AUTH_CODE_PROMPT, config.AUTH_TIMEOUT)        # out is
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
    out = s.read_until(config.AUTH_FAIL_MARKER, config.AUTH_TIMEOUT)    # success -> CLI exits (EOF); failure -> marker
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
