"""stream-json reader (SFR-AGG-019/020, SSR-AGG-009)."""

import io

import pytest

from agent_graph.errors import ClaudeTurnError
from agent_graph.ports import Usage
from agent_graph.services.stream_json import TurnAccumulator, iter_ndjson, parse_usage


def test_iter_ndjson_yields_objects_and_skips_garbage():
    data = b'banner\n{"a": 1}\n\n[1, 2]\n{broken\n{"b": 2}\n'
    assert list(iter_ndjson(io.BytesIO(data), 1000)) == [{"a": 1}, {"b": 2}]


def test_iter_ndjson_last_line_without_newline():
    assert list(iter_ndjson(io.BytesIO(b'{"a": 1}'), 1000)) == [{"a": 1}]


def test_iter_ndjson_overlong_line_is_an_error_not_a_drop():
    data = b'{"a": 1}\n{"x": "' + b"y" * 200 + b'"}\n'
    it = iter_ndjson(io.BytesIO(data), 50)
    assert next(it) == {"a": 1}
    with pytest.raises(ClaudeTurnError):
        next(it)


def test_iter_ndjson_line_exactly_at_cap_is_accepted():
    line = b'{"k": "' + b"v" * 20 + b'"}'
    assert list(iter_ndjson(io.BytesIO(line + b"\n"), len(line) + 1)) == [{"k": "v" * 20}]


def test_parse_usage_with_nested_cache_creation():
    u = parse_usage({"input_tokens": 3, "cache_creation_input_tokens": 100,
                     "cache_read_input_tokens": 50,
                     "cache_creation": {"ephemeral_1h_input_tokens": 100, "ephemeral_5m_input_tokens": 0}})
    assert u == Usage(3, 100, 50, 100, 0)
    assert parse_usage(None) == Usage()
    assert parse_usage({"input_tokens": "x"}) == Usage()


def test_accumulator_collects_turn():
    acc = TurnAccumulator()
    for ev in [
        {"type": "system", "subtype": "init", "session_id": "s-init"},
        {"type": "assistant", "message": {"content": [{"type": "text", "text": "hi"}],
                                           "usage": {"input_tokens": 1, "cache_read_input_tokens": 9}}},
        {"type": "assistant", "message": {"content": [], "usage": {"input_tokens": 2}}},
        {"type": "system", "subtype": "compact_boundary", "compact_metadata": {"trigger": "auto"}},
        {"type": "result", "is_error": False, "result": "final", "session_id": "s-res"},
    ]:
        acc.feed(ev)
    assert acc.session_id == "s-res"
    assert acc.text == "final"
    assert [u.input_tokens for u in acc.usages] == [1, 2]
    assert acc.usages[0].cache_read_input_tokens == 9
    assert acc.compactions == [{"trigger": "auto"}]
    assert not acc.is_error


class _Sink:
    def __init__(self):
        self.events, self.dropped = [], 0

    def feed(self, ev):
        self.events.append(ev)


def test_accumulator_forwards_valid_events_and_counts_malformed():
    sink = _Sink()
    acc = TurnAccumulator(sink=sink)
    good = [
        {"type": "stream_event", "event": {"type": "content_block_delta",
                                           "delta": {"type": "text_delta", "text": "h"}}},
        {"type": "assistant", "message": {"content": [{"type": "text", "text": "hi"},
                                                       {"type": "tool_use", "name": "Read", "input": {}}]}},
        {"type": "user", "message": {"content": [{"type": "tool_result", "content": "x"}]}},
        {"type": "result", "is_error": False, "result": "hi", "session_id": "s", "duration_ms": 5, "num_turns": 1},
    ]
    bad = [
        {"type": "assistant", "message": "not a dict"},
        {"type": "stream_event", "event": {"delta": {}}},
        {"type": "result", "duration_ms": [1]},
    ]
    for ev in good[:2] + bad + good[2:]:
        acc.feed(ev)
    acc.feed({"type": "rate_limit_event"})           # unknown type: ignored, not counted
    assert sink.events == good
    assert sink.dropped == acc.dropped == 3
    assert acc.session_id == "s" and acc.text == "hi"       # accumulation is unchanged


def test_accumulator_without_sink_accumulates_only():
    acc = TurnAccumulator()
    acc.feed({"type": "assistant", "message": "bad"})
    assert acc.dropped == 0


def test_valid_event_gate():
    from agent_graph.services.stream_json import valid_event
    assert valid_event({"type": "system", "subtype": "init"})
    assert not valid_event({"type": "result", "num_turns": True})
    assert not valid_event({"type": "nope"}) and not valid_event("x")


def test_accumulator_falls_back_to_init_session_and_assistant_text():
    acc = TurnAccumulator()
    acc.feed({"type": "system", "subtype": "init", "session_id": "s-init"})
    acc.feed({"type": "assistant", "message": {"content": [{"type": "text", "text": "a"},
                                                            {"type": "text", "text": "b"}]}})
    assert acc.session_id == "s-init"
    assert acc.text == "a\nb"
    assert acc.result is None
