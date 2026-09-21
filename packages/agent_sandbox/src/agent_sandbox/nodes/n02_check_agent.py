"""Node 2 — check whether a container for this agent already exists."""
from agent_sandbox.services import container as ct
from agent_sandbox.state import AgentState


def check_agent(state: AgentState) -> AgentState:
    c = ct.find(state["agent_name"])
    if c is None:
        return {"agent_exists": False}
    ct.ensure_running(c)
    return {"agent_exists": True, "container_id": c.id}


def route_after_check(state: AgentState) -> str:
    # existing agent is assumed already authenticated -> still verify in node 4
    return "auth_gateway" if state["agent_exists"] else "create_container"
