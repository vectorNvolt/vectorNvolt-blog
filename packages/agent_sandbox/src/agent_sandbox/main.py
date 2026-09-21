"""Terminal driver: runs the graph, answers every interrupt() from stdin."""
import uuid

from langgraph.types import Command

from agent_sandbox.graph import build_graph


def main() -> None:
    graph = build_graph()
    cfg = {"configurable": {"thread_id": str(uuid.uuid4())}, "recursion_limit": 1000}

    result = graph.invoke({}, cfg)
    while "__interrupt__" in result:
        prompt = result["__interrupt__"][0].value["prompt"]
        try:
            reply = input(f"{prompt}\n> ")
        except (EOFError, KeyboardInterrupt):
            reply = "/quit"
        result = graph.invoke(Command(resume=reply), cfg)

    print("\n[graph] finished")


if __name__ == "__main__":
    main()
