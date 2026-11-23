"""Reader for the claude CLI ``--output-format stream-json`` output (one JSON object per line).

Covers: SFR-AGG-020 (usage per assistant event, compaction events), SFR-AGG-019 (session id),
SSR-AGG-009 (a line over the cap is an error, never silently dropped).

Event shapes used (see design/tools/cache_probe.py):
  {"type":"system","subtype":"init","session_id":...}
  {"type":"assistant","message":{"content":[{"type":"text","text":...}],"usage":{...}}}
  {"type":"system","subtype":"compact_boundary","compact_metadata":{...}}
  {"type":"result","subtype":...,"is_error":bool,"result":str,"session_id":...}
Lines that are not JSON objects (banners, garbage) are skipped.
"""

import json
from dataclasses import dataclass, field
from typing import IO, Any, Iterator

from ..errors import ClaudeTurnError
from ..ports import Usage


def iter_ndjson(stream: IO[bytes], cap: int) -> Iterator[dict]:
    """Yield each JSON-object line of ``stream``. A line longer than ``cap`` bytes raises
    ClaudeTurnError (the rest of the line is not read)."""
    while True:
        raw = stream.readline(cap + 1)
        if not raw:
            return
        if len(raw) > cap and not raw.endswith(b"\n"):
            raise ClaudeTurnError(f"stream-json line longer than {cap} bytes")
        line = raw.strip()
        if not line.startswith(b"{"):
            continue
        try:
            obj = json.loads(line)
        except (json.JSONDecodeError, UnicodeDecodeError):
            continue
        if isinstance(obj, dict):
            yield obj


def _int(v: Any) -> int:
    return v if isinstance(v, int) and not isinstance(v, bool) else 0


def parse_usage(usage: dict | None) -> Usage:
    usage = usage or {}
    creation = usage.get("cache_creation") or {}
    return Usage(
        input_tokens=_int(usage.get("input_tokens")),
        cache_creation_input_tokens=_int(usage.get("cache_creation_input_tokens")),
        cache_read_input_tokens=_int(usage.get("cache_read_input_tokens")),
        ephemeral_1h_input_tokens=_int(creation.get("ephemeral_1h_input_tokens")),
        ephemeral_5m_input_tokens=_int(creation.get("ephemeral_5m_input_tokens")),
    )


# --- event schema ---------------------------------------------------------------
# The shape half of trusting the stream: an event is forwarded to the renderer only if its type is
# known and every field the renderer reads has the expected type. A spec is {field: (type | nested
# spec, required)}; a list spec is [item spec]; a tuple is "any of".
_STR_OR_NONE = (str, type(None))
_BLOCK = {"type": (str, True), "text": (str, False), "name": (str, False),
          "content": ((str, list), False), "is_error": (bool, False)}
_MESSAGE = {"content": ((str, [_BLOCK]), False)}
EVENT_SCHEMA = {
    "system": {},
    "stream_event": {"event": ({"type": (str, True),
                                "content_block": ({"type": (str, True), "name": (str, False)}, False),
                                "delta": ({"type": (str, True), "text": (str, False)}, False)}, True)},
    "assistant": {"message": (_MESSAGE, True)},
    "user": {"message": (_MESSAGE, True)},
    "result": {"result": (_STR_OR_NONE, False), "is_error": (bool, False),
               "duration_ms": ((int, float), False), "num_turns": (int, False)},
}


def _conforms(value: Any, spec: Any) -> bool:
    if isinstance(spec, dict):
        if not isinstance(value, dict):
            return False
        for name, (fspec, required) in spec.items():
            if name not in value:
                if required:
                    return False
                continue
            if not _conforms(value[name], fspec):
                return False
        return True
    if isinstance(spec, list):
        return isinstance(value, list) and all(_conforms(v, spec[0]) for v in value)
    if isinstance(spec, tuple):
        return any(_conforms(value, alt) for alt in spec)
    return isinstance(value, spec) and not (spec is int and isinstance(value, bool))


def valid_event(ev: Any) -> bool:
    return isinstance(ev, dict) and ev.get("type") in EVENT_SCHEMA and _conforms(ev, EVENT_SCHEMA[ev["type"]])


@dataclass
class TurnAccumulator:
    """Collects what the runtime needs from the events of one turn. With a ``sink`` (a TurnSink),
    every schema-valid event is also forwarded to ``sink.feed``; an event of a known type that does
    not conform is counted in ``dropped`` (and ``sink.dropped``); unknown types are ignored."""
    init_session: str | None = None
    result: dict | None = None
    usages: list[Usage] = field(default_factory=list)
    compactions: list[dict] = field(default_factory=list)
    assistant_text: list[str] = field(default_factory=list)
    sink: Any = None
    dropped: int = 0

    def feed(self, ev: dict) -> None:
        self._accumulate(ev)
        if self.sink is None or ev.get("type") not in EVENT_SCHEMA:
            return
        if valid_event(ev):
            self.sink.feed(ev)
        else:
            self.dropped += 1
            self.sink.dropped = self.dropped

    def _accumulate(self, ev: dict) -> None:
        kind = ev.get("type")
        if kind == "system":
            sub = ev.get("subtype")
            if sub == "init" and isinstance(ev.get("session_id"), str):
                self.init_session = ev["session_id"]
            elif sub == "compact_boundary":
                meta = ev.get("compact_metadata")
                self.compactions.append(dict(meta) if isinstance(meta, dict) else {})
        elif kind == "assistant":
            msg = ev.get("message")
            msg = msg if isinstance(msg, dict) else {}
            if isinstance(msg.get("usage"), dict):
                self.usages.append(parse_usage(msg["usage"]))
            content = msg.get("content")
            for block in content if isinstance(content, list) else []:
                if isinstance(block, dict) and block.get("type") == "text" \
                        and isinstance(block.get("text"), str):
                    self.assistant_text.append(block["text"])
        elif kind == "result":
            self.result = ev

    @property
    def session_id(self) -> str:
        if self.result and isinstance(self.result.get("session_id"), str) and self.result["session_id"]:
            return self.result["session_id"]
        return self.init_session or ""

    @property
    def text(self) -> str:
        if self.result and isinstance(self.result.get("result"), str):
            return self.result["result"]
        return "\n".join(self.assistant_text)

    @property
    def is_error(self) -> bool:
        return bool(self.result and self.result.get("is_error"))
