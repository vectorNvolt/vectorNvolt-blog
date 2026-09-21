"""IP-1 egress: what leaves the renderer for the terminal."""
from agent_sandbox import config
from agent_sandbox.services.render import TurnRenderer

EVIL = "https://evil.example/x"


def _turn(*events):
    r = TurnRenderer()
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


# --- T9d ------------------------------------------------------------------

def test_tool_result_escape_sequences_neutralised(screen):
    _turn(_user("A\x1b]52;c;SGVsbG8=\x1b\\B\x1b[6nC\x1b[2JD"))
    out = screen.getvalue()
    for seq in ("\x1b]52", "\x1b[6n", "\x1b[2J"):
        assert seq not in out
    assert "␛]52;c;SGVsbG8=␛\\" in out and "␛[6n" in out and "␛[2J" in out


def test_model_text_escape_sequences_neutralised(screen):
    _turn(_assistant({"type": "text", "text": "clear \x1b[2J screen \x9b2J too"}))
    out = screen.getvalue()
    assert "\x1b[2J" not in out and "\x9b" not in out
    assert "␛[2J" in out


def test_tool_name_is_not_markup(screen):
    _turn(_assistant({"type": "tool_use", "id": "t1", "name": f"[link={EVIL}]Read[/link]", "input": {}}))
    out = screen.getvalue()
    assert "\x1b]8;;" not in out
    assert f"[link={EVIL}]Read[/link]" in out          # shown literally in the title


# --- T9e ------------------------------------------------------------------

def test_markdown_links_show_their_target(screen):
    _turn(_assistant({"type": "text", "text": f"see [docs]({EVIL}) here"}))
    out = screen.getvalue()
    assert "\x1b]8;;" not in out
    assert EVIL in out


def test_clipping_keeps_head_and_tail(screen):
    body = "HEAD " + "x" * 5000 + " TAIL"
    _turn(_user(body))
    out = screen.getvalue()
    assert "HEAD" in out and "TAIL" in out and "chars omitted" in out


def test_bidi_override_is_visible(screen):
    _turn(_user("safe ‮ evas"))
    assert "‮" not in screen.getvalue() and "⟨U+202E⟩" in screen.getvalue()


def test_envelope_fields_presented_as_reported(screen):
    _turn(_user("boom", is_error=True), _result(duration_ms=4200, num_turns=2, is_error=False))
    out = screen.getvalue()
    assert "error (reported)" in out
    assert "4.2s · 2 turn(s) — as reported by the cli" in out


def test_footer_survives_wrong_field_types(screen):
    _turn(_result(duration_ms="4200", num_turns=[2]))
    assert "?s · ? turn(s)" in screen.getvalue()


# --- T7e ------------------------------------------------------------------

def test_streamed_text_block_is_bounded(screen, monkeypatch):
    monkeypatch.setattr(config, "CHAT_MAX_TEXT_BLOCK", 100)
    r = TurnRenderer()
    r.feed({"type": "stream_event", "event": {"type": "content_block_start", "content_block": {"type": "text"}}})
    for _ in range(50):
        r.feed({"type": "stream_event", "event": {"type": "content_block_delta",
                                                  "delta": {"type": "text_delta", "text": "0123456789"}}})
        assert len(r._text) <= 100
    r.feed({"type": "stream_event", "event": {"type": "content_block_stop"}})
    r.finish()
    assert "400 more chars not shown" in screen.getvalue()


def test_final_text_block_is_bounded(screen, monkeypatch):
    monkeypatch.setattr(config, "CHAT_MAX_TEXT_BLOCK", 300)
    _turn(_assistant({"type": "text", "text": "START " + "y " * 5000 + " END"}))
    out = screen.getvalue()
    assert "START" in out and "END" in out and "chars omitted" in out


def test_non_dict_content_blocks_do_not_crash(screen):
    _turn(_assistant("just a string", 42, {"type": "text", "text": "ok"}), _user("r"))
    assert "ok" in screen.getvalue()


def test_dropped_events_are_announced(screen):
    r = TurnRenderer()
    r.dropped = 3
    r.finish()
    assert "3 malformed event(s) dropped" in screen.getvalue()
