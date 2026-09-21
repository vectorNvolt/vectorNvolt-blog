"""Node 6 (loop) — exchange loop between the sandboxed agent and the user.

Two turns alternate on the self-loop:
  agent : send the latest user message to `claude -p`, show the answer.
  user  : interrupt FIRST, collect the next user message; `/quit` ends the loop.
(Interrupt-first per pass avoids re-invoking the CLI on resume — see node 4.)
"""
from agent_sandbox import config
from agent_sandbox.services import container as ct
from agent_sandbox.services import gateway as gw
from agent_sandbox.state import AgentState


def exchange(state: AgentState) -> AgentState:
    if state.get("turn", "agent") == "agent":
        c = ct.client().containers.get(state["container_id"])
        last_user = next(m["content"] for m in reversed(state["transcript"]) if m["role"] == "user")    # reversed():
        answer = gw.send_prompt(c, last_user)
        gw.notify_user(answer)
        return {"turn": "user", "transcript": [{"role": "agent", "content": answer}]}

    nxt = gw.ask_user("Your reply (or /quit):")
    if nxt.lower() in config.EXIT_WORDS:
        return {"done": True}
    return {"turn": "agent", "transcript": [{"role": "user", "content": nxt}]}


def route_after_exchange(state: AgentState) -> str:
    return "end" if state.get("done") else "exchange"
