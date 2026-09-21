"""Node 5 — ask the user what they want to do."""
from agent_sandbox.services.gateway import ask_user
from agent_sandbox.state import AgentState


def ask_task(state: AgentState) -> AgentState:
    task = ""
    while not task:
        task = ask_user(f"Agent '{state['agent_name']}' is ready. What do you want to do?")
    return {"task": task, "done": False, "turn": "agent", "transcript": [{"role": "user", "content": task}]}
