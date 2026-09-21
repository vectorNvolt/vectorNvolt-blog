"""IP-1 ingress: the operator's line is flushed before and checked after (T1e reply half)."""
import builtins

from agent_sandbox import main


def test_reply_with_terminal_answer_prefix_is_rejected_then_reasked(monkeypatch, capsys):
    lines = iter(["\x1b[24;80Rsummarise README.md", "summarise README.md"])
    monkeypatch.setattr(builtins, "input", lambda prompt: next(lines))
    flushed = []
    monkeypatch.setattr(main, "_flush_stdin", lambda: flushed.append(True))

    assert main.read_reply("Your reply (or /quit):") == "summarise README.md"
    assert len(flushed) == 2                                  # flushed before every read
    out = capsys.readouterr().out
    assert "dropped" in out and "␛[24;80R" in out and "\x1b[" not in out


def test_flush_is_a_no_op_off_a_tty():
    main._flush_stdin()                                       # pytest's stdin is not a tty; must not raise
