"""Rich rendering of a streamed turn (TurnRenderer) and the gateway's rich output."""

import io

import pytest
from rich.console import Console

from agent_graph.services import render
from agent_graph.services.gateway import Gateway
from agent_graph.services.render import THEME, TurnRenderer

EVIL = "https://evil.example/x"


def make_console(force_terminal=False, width=100):
    buf = io.StringIO()
    return Console(file=buf, theme=THEME, force_terminal=force_terminal, width=width, highlight=False), buf


@pytest.fixture
def screen():
    console, buf = make_console()
    buf.console = console
    return buf


def _turn(screen, *events, **kw):
    r = TurnRenderer(screen.console, **kw)
    for ev in events:
        r.feed(ev)
    r.finish()
    return r


def _user(content, is_error=False):
    return {"type": "user", "message": {"role": "user", "content": [
        {"type": "tool_result", "tool_use_id": "t1", "content": content, "is_error": is_error}]}}


def _assistant(*blocks):
    return {"type": "assistant", "message": {"role": "assistant", "content": list(blocks)}}


def _result(**kw):
    return {"type": "result", **kw}


def _delta(text):
    return {"type": "stream_event", "event": {"type": "content_block_delta",
                                              "delta": {"type": "text_delta", "text": text}}}


def _start_text():
    return {"type": "stream_event", "event": {"type": "content_block_start", "content_block": {"type": "text"}}}


def _stop():
    return {"type": "stream_event", "event": {"type": "content_block_stop"}}


def test_tool_result_escape_sequences_neutralised(screen):
    _turn(screen, _user("A\x1b]52;c;SGVsbG8=\x1b\\B\x1b[6nC\x1b[2JD"))
    out = screen.getvalue()
    for seq in ("\x1b]52", "\x1b[6n", "\x1b[2J"):
        assert seq not in out
    assert "␛]52;c;SGVsbG8=␛\\" in out and "␛[6n" in out and "␛[2J" in out


def test_model_text_escape_sequences_neutralised(screen):
    _turn(screen, _assistant({"type": "text", "text": "clear \x1b[2J screen \x9b2J too"}))
    out = screen.getvalue()
    assert "\x1b[2J" not in out and "\x9b" not in out
    assert "␛[2J" in out


def test_tool_name_is_not_markup(screen):
    _turn(screen, _assistant({"type": "tool_use", "id": "t1", "name": f"[link={EVIL}]Read[/link]", "input": {}}))
    out = screen.getvalue()
    assert "\x1b]8;;" not in out
    assert f"[link={EVIL}]Read[/link]" in out


def test_tool_use_panel_shows_name_and_input(screen):
    _turn(screen, _assistant({"type": "tool_use", "id": "t1", "name": "Read", "input": {"file_path": "/x/y"}}))
    out = screen.getvalue()
    assert "tool · Read" in out and "/x/y" in out


def test_markdown_links_show_their_target(screen):
    _turn(screen, _assistant({"type": "text", "text": f"see [docs]({EVIL}) here"}))
    out = screen.getvalue()
    assert "\x1b]8;;" not in out
    assert EVIL in out


def test_clipping_keeps_head_and_tail(screen):
    _turn(screen, _user("HEAD " + "x" * 5000 + " TAIL"))
    out = screen.getvalue()
    assert "HEAD" in out and "TAIL" in out and "chars omitted" in out


def test_bidi_override_is_visible(screen):
    _turn(screen, _user("safe ‮ evas"))
    assert "‮" not in screen.getvalue() and "⟨U+202E⟩" in screen.getvalue()


def test_envelope_fields_presented_as_reported(screen):
    _turn(screen, _user("boom", is_error=True), _result(duration_ms=4200, num_turns=2, is_error=False))
    out = screen.getvalue()
    assert "error (reported)" in out
    assert "4.2s · 2 turn(s) — as reported by the cli" in out


def test_footer_survives_wrong_field_types(screen):
    _turn(screen, _result(duration_ms="4200", num_turns=[2]))
    assert "?s · ? turn(s)" in screen.getvalue()


def test_streamed_text_block_is_bounded(screen, monkeypatch):
    monkeypatch.setattr(render, "TEXT_BLOCK_MAX", 100)
    r = TurnRenderer(screen.console)
    r.feed(_start_text())
    for _ in range(50):
        r.feed(_delta("0123456789"))
        assert len(r._text) <= 100
    r.feed(_stop())
    r.finish()
    assert "400 more chars not shown" in screen.getvalue()


def test_final_text_block_is_bounded(screen, monkeypatch):
    monkeypatch.setattr(render, "TEXT_BLOCK_MAX", 300)
    _turn(screen, _assistant({"type": "text", "text": "START " + "y " * 5000 + " END"}))
    out = screen.getvalue()
    assert "START" in out and "END" in out and "chars omitted" in out


def test_non_dict_content_blocks_do_not_crash(screen):
    _turn(screen, _assistant("just a string", 42, {"type": "text", "text": "ok"}), _user("r"))
    assert "ok" in screen.getvalue()


def test_dropped_events_are_announced(screen):
    r = TurnRenderer(screen.console)
    r.dropped = 3
    r.finish()
    assert "3 malformed event(s) dropped" in screen.getvalue()


def test_streamed_text_is_printed_once_as_markdown_not_twice(screen):
    _turn(screen, _start_text(), _delta("hello **wor"), _delta("ld**"), _stop(),
          _assistant({"type": "text", "text": "hello **world**"}), _result(duration_ms=1000, num_turns=1))
    assert screen.getvalue().count("hello world") == 1
    assert "\x1b[" not in screen.getvalue()                  # not a TTY: no ANSI, no Live


def test_show_text_false_hides_assistant_text_but_keeps_panels(screen):
    _turn(screen, _start_text(), _delta("SECRETJSON"), _stop(),
          _assistant({"type": "text", "text": "SECRETJSON"},
                     {"type": "tool_use", "name": "Grep", "input": {"pattern": "p"}}),
          _result(duration_ms=2000, num_turns=1), show_text=False)
    out = screen.getvalue()
    assert "SECRETJSON" not in out and "tool · Grep" in out and "2.0s" in out


def test_tail_is_limited_to_terminal_height():
    console, buf = make_console(force_terminal=True)
    r = TurnRenderer(console)
    r.feed(_start_text())
    r.feed(_delta("\n".join(f"line{i}" for i in range(500))))
    keep = max(console.size.height - 2, 3)
    assert r._tail().plain.count("\n") + 1 <= keep
    r.feed(_stop())
    r.finish()


def test_tty_output_is_styled_and_spinner_is_cleaned_up(color_env):
    console, buf = make_console(force_terminal=True)
    r = TurnRenderer(console)
    r.feed(_assistant({"type": "tool_use", "name": "Read", "input": {}}))
    r.feed(_user("done"))
    r.feed(_result(duration_ms=1000, num_turns=1))
    assert "\x1b[" in buf.getvalue()                          # themed
    assert r._status is None and r._live is None


# ---------------------------------------------------------------- gateway output through rich
def test_gateway_plain_output_off_tty_has_no_ansi_and_keeps_tags():
    out = io.StringIO()
    gw = Gateway(io.StringIO(""), out, clock=lambda: 1.0)
    gw.say("S2 clarify", "[bold]not markup[/bold] :smile:")
    gw.notice("engine", "WARNING", "careful")
    gw.say("S2 clarify", "**bold** text", from_llm=True)
    assert out.getvalue() == ("[S2 clarify] [bold]not markup[/bold] :smile:\n"
                              "[engine] WARNING: careful\n"
                              "[S2 clarify] **bold** text\n")


def test_gateway_long_line_is_not_wrapped_off_tty():
    out = io.StringIO()
    gw = Gateway(io.StringIO(""), out, clock=lambda: 1.0)
    gw.say("s", "word " * 100)
    assert out.getvalue().count("\n") == 1


@pytest.fixture
def color_env(monkeypatch):
    monkeypatch.setenv("TERM", "xterm-256color")
    for var in ("NO_COLOR", "TTY_COMPATIBLE"):
        monkeypatch.delenv(var, raising=False)


def test_gateway_tty_say_llm_renders_markdown_and_tag(color_env):
    out = io.StringIO()
    gw = Gateway(io.StringIO(""), out, clock=lambda: 1.0, force_terminal=True)
    gw.say("S2 clarify", f"**bold** [docs]({EVIL}) \x1b[2J", from_llm=True)
    shown = out.getvalue()
    assert "[S2 clarify]" in shown and "\x1b[" in shown        # styled tag line and markdown
    assert EVIL in shown and "\x1b[2J" not in shown and "\x1b]8;" not in shown
    assert "**" not in shown


def test_gateway_notice_styles_by_code(color_env):
    out = io.StringIO()
    gw = Gateway(io.StringIO(""), out, clock=lambda: 1.0, force_terminal=True)
    gw.notice("e", "LLM_UNAVAILABLE", "x")
    gw.notice("e", "PROGRESS", "y")
    lines = out.getvalue().splitlines()
    assert "LLM_UNAVAILABLE: x" in lines[0] and "PROGRESS: y" in lines[1]
    assert "\x1b[" in lines[0] and "\x1b[" in lines[1]
    assert lines[0].split("LLM_UNAVAILABLE")[0] != lines[1].split("PROGRESS")[0]     # different styles
    assert render.notice_style("WARNING") == "notice.warn"
    assert render.notice_style("PROGRESS") == "notice.progress"
    assert render.notice_style("RESOLUTION_LIMIT") == "notice.error"


def test_gateway_prompt_is_tagged_and_input_still_read():
    out = io.StringIO()
    gw = Gateway(io.StringIO("yes\n"), out, clock=lambda: 1.0)
    assert gw.confirm("S5", "Start?") is True
    assert "[S5] Start? (y/n)\n[S5] > " in out.getvalue()


def test_gateway_turn_renders_events_and_footer(tmp_path):
    out = io.StringIO()
    gw = Gateway(io.StringIO(""), out, clock=lambda: 1.0)
    gw.attach_run(tmp_path / "gateway.jsonl")
    with gw.turn("S7 resolution") as t:
        t.feed(_assistant({"type": "text", "text": "answer"}))
        t.feed(_result(duration_ms=1500, num_turns=1))
    assert "answer" in out.getvalue() and "1.5s" in out.getvalue()
    assert not (tmp_path / "gateway.jsonl").exists()               # turn() itself transcribes nothing


def test_gateway_turn_holds_the_conversation():
    import threading
    gw = Gateway(io.StringIO(""), io.StringIO(), clock=lambda: 1.0)
    got = threading.Event()

    def other():
        with gw.conversation("b"):
            got.set()

    with gw.turn("a"):
        t = threading.Thread(target=other)
        t.start()
        assert not got.wait(0.2)
    assert got.wait(2.0)
    t.join()
