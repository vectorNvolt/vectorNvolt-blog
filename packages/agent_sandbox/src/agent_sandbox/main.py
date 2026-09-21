"""Terminal driver: runs the graph, answers every interrupt() from stdin."""
import sys
import uuid

from langgraph.types import Command

from agent_sandbox.graph import build_graph
from agent_sandbox.services.sanitize import has_control, sanitize


def _flush_stdin() -> None:
    """Discard whatever is queued on the tty before we read the operator's line.

    During a turn the renderer may have printed a query sequence the agent smuggled
    into its output (`ESC[6n`, `ESC[c`, an OSC 52 read); the terminal emulator answers
    it *on stdin*, and without this the answer is the invisible prefix of the next
    reply. The cost is type-ahead: anything typed while the agent was working is
    dropped too."""
    try:
        import termios
        if sys.stdin.isatty():
            termios.tcflush(sys.stdin.fileno(), termios.TCIFLUSH)
    except (ImportError, OSError, ValueError):
        pass


def read_reply(prompt: str) -> str:
    """One operator line: flushed before, checked after. A line with ESC, C0/C1 or
    format characters is not an operator's — it is a terminal's reply to a query,
    or a paste of one — so it is shown neutralised and asked again."""
    while True:
        _flush_stdin()
        reply = input(f"{prompt}\n> ")
        if not has_control(reply):
            return reply
        print(f"[harness] reply held control characters and was dropped: {sanitize(reply)}")


def main() -> None:
    graph = build_graph()
    cfg = {"configurable": {"thread_id": str(uuid.uuid4())}, "recursion_limit": 1000}

    result = graph.invoke({}, cfg)
    while "__interrupt__" in result:
        prompt = result["__interrupt__"][0].value["prompt"]
        try:
            reply = read_reply(prompt)
        except (EOFError, KeyboardInterrupt):
            reply = "/quit"
        result = graph.invoke(Command(resume=reply), cfg)

    print("\n[graph] finished")


if __name__ == "__main__":
    main()
