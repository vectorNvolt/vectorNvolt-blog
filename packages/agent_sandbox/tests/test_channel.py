"""IP-3 ingress: the exec channel's line cap and the event schema (T7e, T9a shape)."""
import struct

import pytest

from agent_sandbox.services import container as ct
from agent_sandbox.services.gateway import _parse_event, valid_event


# --- gateway: guarded loads + schema ------------------------------------------

def test_deeply_nested_json_is_dropped_not_fatal():
    assert _parse_event("[" * 100000) is None


def test_invalid_json_is_dropped():
    assert _parse_event("{not json") is None
    assert _parse_event("42") is None


def test_known_event_shapes_pass():
    assert _parse_event('{"type":"assistant","message":{"content":[{"type":"text","text":"hi"}]}}')
    assert _parse_event('{"type":"user","message":{"content":[{"type":"tool_result","content":"x","is_error":true}]}}')
    assert _parse_event('{"type":"result","result":"done","is_error":false,"duration_ms":12,"num_turns":1}')
    assert _parse_event('{"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"text_delta","text":"a"}}}')
    assert _parse_event('{"type":"system","subtype":"init"}')


@pytest.mark.parametrize("ev", [
    {"type": "shutdown"},                                                    # unknown type
    {"type": "assistant"},                                                   # missing message
    {"type": "assistant", "message": "text"},                                # wrong type
    {"type": "user", "message": {"content": [{"text": "no type"}]}},         # block without type
    {"type": "user", "message": {"content": [{"type": "tool_result", "is_error": "yes"}]}},
    {"type": "result", "duration_ms": [1]},
    {"type": "result", "num_turns": True},                                   # bool is not an int here
    {"type": "result", "result": 7},
    {"type": "stream_event", "event": {"delta": {}}},                        # event without type
])
def test_malformed_events_are_refused(ev):
    assert not valid_event(ev)


# --- container: line cap ------------------------------------------------------

class _FakeSock:
    def __init__(self, chunks):
        self._chunks = list(chunks)

    def recv(self, n):
        if not self._chunks:
            return b""
        return self._chunks.pop(0)


def _frame(payload: bytes, stream=1) -> bytes:
    return struct.pack(">BxxxL", stream, len(payload)) + payload


def _session(chunks, max_line=64, tty=False):
    s = object.__new__(ct.ExecSession)
    s._tty, s._raw, s._stdout, s.stderr, s._max_line = tty, b"", "", "", max_line
    s._sock = _FakeSock(chunks)
    return s


def test_readline_returns_bounded_lines():
    s = _session([_frame(b'{"a":1}\n{"b":2}\n')])
    assert s.readline(1) == '{"a":1}'
    assert s.readline(1) == '{"b":2}'


def test_readline_raises_on_line_without_newline_over_cap():
    s = _session([_frame(b"x" * 40), _frame(b"x" * 40)])
    with pytest.raises(ct.LineTooLong):
        s.readline(1)


def test_readline_raises_on_complete_line_over_cap():
    s = _session([_frame(b"y" * 100 + b"\n")])
    with pytest.raises(ct.LineTooLong):
        s.readline(1)


def test_demux_refuses_oversized_frame_header():
    s = _session([struct.pack(">BxxxL", 1, 1 << 30) + b"abc"])
    with pytest.raises(ct.LineTooLong):
        s.readline(1)


def test_stderr_frames_do_not_count_as_stdout():
    s = _session([_frame(b"warn\n", stream=2), _frame(b"ok\n")])
    assert s.readline(1) == "ok"
    assert s.stderr == "warn\n"
