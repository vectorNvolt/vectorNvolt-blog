"""Node 3 — create a gVisor (runsc) container running the Claude CLI."""
from agent_sandbox.services import container as ct
from agent_sandbox.services.gateway import notify_user
from agent_sandbox.state import AgentState


def create_container(state: AgentState) -> AgentState:
    c = ct.create(state["agent_name"])
    notify_user(f"created sandbox {c.name} ({c.short_id}) with runtime=runsc")
    return {"container_id": c.id, "agent_exists": True, "authenticated": False, "auth_attempts": 0, "auth_phase": "check"}
