"""Node 4 (loop) — message gateway that lets the user authenticate the CLI in the sandbox.

Two phases alternate on the self-loop:
  check : `claude auth status` ok? -> leave. Else run `claude auth login`, keep its
          output (URL / instructions) in state.
  relay : interrupt FIRST (relay stored output to the user, wait for code/token),
          then feed the reply to the CLI.
Keeping the interrupt first in its pass matters: LangGraph re-runs a node from the
top on resume, so any side effect placed before `interrupt()` would execute twice.
"""
from agent_sandbox import config
from agent_sandbox.services import container as ct
from agent_sandbox.services import gateway as gw
from agent_sandbox.state import AgentState


def auth_gateway(state: AgentState) -> AgentState:
    c = ct.client().containers.get(state["container_id"])
    attempts = state.get("auth_attempts", 0)

    if state.get("auth_phase", "check") == "check":
        if gw.is_authenticated(c):
            return {"authenticated": True, "auth_phase": "check"}
        return {"authenticated": False, "auth_prompt": gw.start_login(c), "auth_phase": "relay"}

    # relay
    reply = gw.ask_user(f"{state['auth_prompt']}\n\nEnter the authorization code/token (or 'skip'):")
    if reply.lower() == "skip":
        return {"authenticated": False, "auth_attempts": config.MAX_AUTH_ATTEMPTS, "auth_phase": "check"}
    ok, msg = gw.finish_login(c, reply)
    if msg:
        gw.notify_user(msg)
    return {"authenticated": ok, "auth_attempts": attempts + 1, "auth_phase": "check"}


def route_after_auth(state: AgentState) -> str:
    if state.get("authenticated"):
        return "ask_task"
    if state.get("auth_attempts", 0) >= config.MAX_AUTH_ATTEMPTS:
        return "end"
    return "auth_gateway"
