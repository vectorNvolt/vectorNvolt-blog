"""Gateway (SFR-AGG-038, SSR-AGG-001) and render helpers."""

import io
import json
import os
import queue
import threading
import time

import pytest

from agent_graph.errors import ReplyTimeout, TerminalClosed
from agent_graph.ports import Reply
from agent_graph.services.gateway import Gateway
from agent_graph.services.render import strip_controls


def make(inp: str = ""):
    out = io.StringIO()
    return Gateway(io.StringIO(inp), out, clock=lambda: 1.0), out


def records(path):
    return [json.loads(line) for line in path.read_text(encoding="utf-8").splitlines()]


def test_say_tags_every_line_with_source():
    gw, out = make()
    gw.say("S2 clarify", "hello")
    gw.say("node normalize", "line one\nline two")
    assert out.getvalue() == ("[S2 clarify] hello\n"
                              "[node normalize] line one\n[node normalize] line two\n")


def test_llm_text_control_sequences_are_made_visible():
    gw, out = make()
    gw.say("S2 clarify", "\x1b[31mred\x1b[0m \x1b]0;title\x07ok\x08", from_llm=True)
    shown = out.getvalue()
    assert shown.startswith("[S2 clarify] ")
    assert "\x1b" not in shown and "\x07" not in shown and "\x08" not in shown
    assert "␛" in shown and "␇" in shown        # neutralised, still visible


def test_notice_is_stripped_and_does_not_wait_for_a_conversation():
    gw, out = make()
    done = threading.Event()

    def other():
        gw.notice("engine", "WARNING", "careful\x1b[2J")
        done.set()

    with gw.conversation("S7 resolution"):
        t = threading.Thread(target=other)
        t.start()
        assert done.wait(2.0)
    t.join()
    assert out.getvalue() == "[engine] WARNING: careful␛[2J\n"


def test_conversations_queue_instead_of_preempting():
    gw, out = make()
    started = threading.Event()

    def other():
        started.set()
        gw.say("b", "second")

    with gw.conversation("a"):
        gw.say("a", "first")
        t = threading.Thread(target=other)
        t.start()
        assert started.wait(2.0)
        time.sleep(0.2)
        assert "second" not in out.getvalue()
    t.join(2.0)
    assert out.getvalue() == "[a] first\n[b] second\n"


def test_transcript_is_buffered_until_attach_run_then_appended(tmp_path):
    gw, _ = make()
    path = tmp_path / "run" / "gateway.jsonl"
    gw.say("S1", "before")
    gw.say("S2 clarify", "from claude", from_llm=True)
    assert not path.exists()
    gw.attach_run(path)
    gw.notice("engine", "PROGRESS", "after")
    recs = records(path)
    assert [(r["kind"], r["source"], r["text"]) for r in recs] == [
        ("say", "S1", "before"), ("llm", "S2 clarify", "from claude"),
        ("notice", "engine", "PROGRESS: after")]
    assert all(r["ts"] == 1.0 for r in recs)


def test_ask_returns_text_and_records_prompt_and_reply(tmp_path):
    gw, out = make("my answer\n")
    path = tmp_path / "gateway.jsonl"
    gw.attach_run(path)
    reply = gw.ask("S2 clarify", "Which process?")
    assert reply == Reply(text="my answer")
    assert "[S2 clarify] Which process?" in out.getvalue()
    assert [(r["kind"], r["text"]) for r in records(path)] == [
        ("ask", "Which process?"), ("reply", "my answer")]


def test_eof_raises_terminal_closed():
    gw, _ = make("")
    with pytest.raises(TerminalClosed):
        gw.ask("S2 clarify", "anyone?")
    gw2, _ = make("")
    with pytest.raises(TerminalClosed):
        gw2.confirm("S5", "start?")


def test_quit_needs_confirmation():
    gw, out = make("/quit\ny\n")
    assert gw.ask("S2", "q?", allow_quit=True) == Reply(text="", quit=True, command="/quit")
    assert "Quit" in out.getvalue()


def test_declined_quit_asks_again():
    gw, _ = make("/quit\nn\nreal answer\n")
    assert gw.ask("S2", "q?", allow_quit=True) == Reply(text="real answer")


def test_quit_where_not_allowed_is_refused_and_asked_again():
    gw, out = make("/quit\nhi\n")
    assert gw.ask("node x", "q?") == Reply(text="hi")
    assert "not available" in out.getvalue()


def test_listed_commands_only():
    gw, _ = make("/show 2\n/workspace/file.txt\n/resolved\n")
    cmds = ("/show", "/resolved")
    assert gw.ask("E3", "?", commands=cmds) == Reply(text="2", command="/show")
    assert gw.ask("E3", "?", commands=cmds) == Reply(text="/workspace/file.txt")
    assert gw.ask("S7", "?", commands=cmds) == Reply(text="", command="/resolved")


def test_choose_is_one_based_on_screen_zero_based_in_result():
    gw, out = make("9\nx\n2\n")
    assert gw.choose("S4", "Which input set?", ["set a", "set b"]) == 1
    assert "1) set a" in out.getvalue() and "2) set b" in out.getvalue()


def test_confirm_reasks_until_explicit():
    gw, _ = make("\nmaybe\nYes\n")
    assert gw.confirm("S5", "Start?") is True
    gw2, _ = make("n\n")
    assert gw2.confirm("S5", "Start?") is False


def test_strip_controls_cases():
    assert strip_controls("a\tb\nc") == "a\tb\nc"
    assert strip_controls("x\x1b[1;32mgreen\x1b[0m") == "xgreen"
    assert strip_controls("x\x1b]8;;http://e\x1b\\link\x1b]8;;\x1b\\") == "xlink"
    assert strip_controls("a\rb\r\nc\x00\x7f") == "ab\nc"
    assert strip_controls("café") == "café"


# ---------------------------------------------------------------- bounded waits (timeout_s)
@pytest.fixture
def pipe_gateway(tmp_path):
    """A Gateway on a real pipe (the select path). Yields (gw, write, close_writer)."""
    r, w = os.pipe()
    stdin = os.fdopen(r, "r")
    gw = Gateway(stdin, io.StringIO(), clock=lambda: 1.0)
    gw.attach_run(tmp_path / "gateway.jsonl")
    state = {"w": w}

    def write(text):
        os.write(state["w"], text.encode("utf-8"))

    def close_writer():
        if state["w"] is not None:
            os.close(state["w"])
            state["w"] = None
    yield gw, write, close_writer, tmp_path / "gateway.jsonl"
    close_writer()
    stdin.close()


def test_pipe_without_input_times_out(pipe_gateway):
    gw, _write, _close, log = pipe_gateway
    t0 = time.monotonic()
    with pytest.raises(ReplyTimeout) as exc:
        gw.ask("node review", "Approve?", timeout_s=0.2)
    assert 0.15 <= time.monotonic() - t0 < 5
    assert exc.value.source == "node review" and exc.value.timeout_s == 0.2
    assert [r["kind"] for r in records(log)] == ["ask", "timeout"]


def test_pipe_lines_are_split_and_eof_still_closes(pipe_gateway):
    gw, write, close, _log = pipe_gateway
    write("first\nsecond\n")
    assert gw.ask("S", "q", timeout_s=2).text == "first"
    assert gw.ask("S", "q").text == "second"                                 # buffered, no timeout
    close()
    with pytest.raises(TerminalClosed):
        gw.ask("S", "q", timeout_s=2)


def test_deadline_covers_reasks(pipe_gateway):
    gw, write, _close, _log = pipe_gateway
    write("9\nx\n")                                                           # two invalid answers
    with pytest.raises(ReplyTimeout):
        gw.choose("S", "pick", ["a", "b"], timeout_s=0.3)
    write("maybe\n")
    with pytest.raises(ReplyTimeout):
        gw.confirm("S", "sure?", timeout_s=0.3)


def test_late_reply_after_a_timeout_does_not_answer_the_next_prompt(pipe_gateway):
    gw, write, _close, log = pipe_gateway
    with pytest.raises(ReplyTimeout):
        gw.confirm("node review", "Send back?", timeout_s=0.1)
    write("y\n")                                                              # typed after the timeout
    threading.Timer(0.2, write, args=("n\n",)).start()                        # the answer to the new prompt
    assert gw.confirm("S7 resolution", "Quit?", timeout_s=5) is False
    recs = records(log)
    assert ("stale", "y") in [(r["kind"], r["text"]) for r in recs]
    assert [r["text"] for r in recs if r["kind"] == "reply"] == ["n"]


class BlockingStream:
    """A stdin without a file descriptor whose readline blocks until a line is fed."""

    def __init__(self):
        self.lines: "queue.Queue[str]" = queue.Queue()
        self.reads = 0

    def readline(self):
        line = self.lines.get()
        self.reads += 1
        return line

    def fileno(self):
        raise io.UnsupportedOperation("no fd")


def test_thread_fallback_times_out_and_leaves_no_stray_reader():
    stream = BlockingStream()
    gw = Gateway(stream, io.StringIO(), clock=lambda: 1.0)
    with pytest.raises(ReplyTimeout):
        gw.ask("node review", "Approve?", timeout_s=0.1)
    stream.lines.put("late\n")                                               # the outstanding read gets it
    deadline = time.monotonic() + 5
    while stream.reads < 1 and time.monotonic() < deadline:
        time.sleep(0.01)
    time.sleep(0.05)
    threading.Timer(0.2, stream.lines.put, args=("fresh\n",)).start()
    assert gw.ask("S7 resolution", "Now?", timeout_s=5).text == "fresh"      # "late" was dropped as stale
    stream.lines.put("later\n")
    time.sleep(0.1)
    assert stream.reads == 2 and stream.lines.qsize() == 1                   # no read-ahead of "later"
    reader = gw._input._thread
    assert gw.ask("S", "q").text == "later"                                  # blocking read, same thread
    assert gw._input._thread is reader and reader.is_alive()
    assert len([t for t in threading.enumerate() if t.name == "gateway-stdin" and t is reader]) == 1


def test_thread_fallback_without_timeout_reads_directly():
    gw, _out = make("hello\n")
    assert gw.ask("S", "q").text == "hello"
    assert gw._input._thread is None                                         # no thread started at all
