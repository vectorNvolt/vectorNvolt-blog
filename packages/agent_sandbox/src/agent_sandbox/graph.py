"""Graph wiring.

select_agent -> check_agent -+-> create_container -> auth_gateway <-+
                             |                          |            | (not authenticated)
                             +--------------------------+------------+
                                                        v (authenticated)
                                                    ask_task -> exchange <-+
                                                                   |       | (not done)
                                                                   +-------+
                                                                   v (done)
                                                                  END
"""
from langgraph.checkpoint.memory import MemorySaver
from langgraph.graph import END, START, StateGraph

from agent_sandbox.nodes.n01_select_agent import select_agent
from agent_sandbox.nodes.n02_check_agent import check_agent, route_after_check
from agent_sandbox.nodes.n03_create_container import create_container
from agent_sandbox.nodes.n04_auth_gateway import auth_gateway, route_after_auth
from agent_sandbox.nodes.n05_ask_task import ask_task
from agent_sandbox.nodes.n06_exchange import exchange, route_after_exchange
from agent_sandbox.state import AgentState


def build_graph(checkpointer=None):
    g = StateGraph(AgentState)

    g.add_node("select_agent", select_agent)
    g.add_node("check_agent", check_agent)
    g.add_node("create_container", create_container)
    g.add_node("auth_gateway", auth_gateway)
    g.add_node("ask_task", ask_task)
    g.add_node("exchange", exchange)

    g.add_edge(START, "select_agent")
    g.add_edge("select_agent", "check_agent")
    g.add_conditional_edges("check_agent", route_after_check,
                            {"create_container": "create_container", "auth_gateway": "auth_gateway"})
    g.add_edge("create_container", "auth_gateway")
    g.add_conditional_edges("auth_gateway", route_after_auth,
                            {"auth_gateway": "auth_gateway", "ask_task": "ask_task", "end": END})
    g.add_edge("ask_task", "exchange")
    g.add_conditional_edges("exchange", route_after_exchange,
                            {"exchange": "exchange", "end": END})

    # interrupt() requires a checkpointer
    return g.compile(checkpointer=checkpointer or MemorySaver())
