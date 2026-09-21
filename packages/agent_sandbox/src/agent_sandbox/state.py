"""Shared graph state."""
from typing import Annotated, Literal, TypedDict
from operator import add


class AgentState(TypedDict, total=False):
    # node 1
    agent_name: str
    # node 2
    agent_exists: bool
    container_id: str
    # node 4 (loop) — "check": probe/start login, "relay": wait for user, feed reply back
    auth_phase: Literal["check", "relay"]
    auth_prompt: str          # CLI output relayed to the user (login URL / instructions)
    authenticated: bool
    auth_attempts: int
    # node 5
    task: str
    # node 6 (loop) — "agent": send last user msg to CLI, "user": collect next user msg
    turn: Literal["agent", "user"]
    transcript: Annotated[list[dict], add]   # [{"role": "user"|"agent", "content": str}]
    done: bool
