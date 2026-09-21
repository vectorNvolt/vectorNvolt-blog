"""Node 1 — ask the user which agent to work with."""
from agent_sandbox.services.gateway import ask_user
from agent_sandbox.state import AgentState


def select_agent(state: AgentState) -> AgentState:
    name = ""
    while not name:
        name = ask_user("Which agent do you want to work with? (name)")
    return {"agent_name": name.lower().replace(" ", "-")}
