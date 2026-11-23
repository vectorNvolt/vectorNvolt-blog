"""Scripted GatewayPort for tests: queued developer replies, recorded output.

Usage::

    gw = ScriptedGateway(replies=["yes, run it", "/quit"], confirms=[True], choices=[0])
    gw.feed("another answer")            # append more replies later
    reply = gw.ask("S2 clarify", "Which process?")

Reply script entries (consumed by ``ask`` in order):
  * ``str``      "plain text" -> Reply(text). A string starting with "/" is a command:
                 "/show 2" -> Reply(text="2", command="/show"); "/quit" -> Reply(text="", quit=True,
                 command="/quit") when ``allow_quit`` is set (the quit is treated as confirmed),
                 otherwise Reply(text="", command="/quit").
  * ``Reply``    returned as is.
  * ``callable`` called as f(source, prompt) and must return a str or Reply (dynamic replies).
When the script is exhausted ``ask`` raises TerminalClosed (EOF on stdin), which is what the real
gateway does; tests of SUSPENDED rely on it.

``confirm`` consumes ``confirms`` (bool), then falls back to ``default_confirm``.
``choose`` consumes ``choices`` (int index), then falls back to ``default_choice``.
Without an explicit ``default_confirm`` / ``default_choice`` an exhausted ``confirms`` /
``choices`` script raises TerminalClosed (like EOF), so a loop that keeps asking fails clearly
instead of spinning on a default.

Timeouts: the sentinel ``TIMEOUT`` (module constant, also ``ScriptedGateway.TIMEOUT``) may stand
in ``replies``, ``confirms`` or ``choices``: the call raises ReplyTimeout, as the real gateway does
when ``timeout_s`` expires. Scripting it for a call without ``timeout_s`` is a test error
(AssertionError: the real call would wait forever). Every call's ``timeout_s`` is recorded in
``timeouts`` as (kind, source, timeout_s).

Recording (all lists of plain tuples/dicts, in call order):
  events       [(kind, source, text)]  kind in say | llm | notice | ask | choose | confirm
  said(source=None, include_llm=True) -> list[str]
  notices      [(source, code, text)]
  asked        [(source, prompt)]
  conversations [(source, "enter"|"exit")]
  attached     the path given to attach_run (transcript is appended to it as JSONL if set)
"""

import json
from contextlib import contextmanager
from pathlib import Path
from typing import Any, Iterator, Sequence

from agent_graph.errors import ReplyTimeout, TerminalClosed
from agent_graph.ports import Reply

TIMEOUT = object()
_UNSET: Any = object()


class RecordingSink:
    """No-op TurnSink: keeps the events it was fed (what ``ScriptedGateway.turn`` yields)."""

    def __init__(self) -> None:
        self.events: list[dict] = []
        self.dropped = 0

    def feed(self, ev: dict) -> None:
        self.events.append(ev)

    def finish(self) -> None:
        pass


class ScriptedGateway:
    TIMEOUT = TIMEOUT

    def __init__(self, replies: Sequence[Any] = (), *, confirms: Sequence[Any] = (),
                 choices: Sequence[Any] = (), default_confirm: Any = _UNSET, default_choice: Any = _UNSET):
        self._replies = list(replies)
        self._confirms = list(confirms)
        self._choices = list(choices)
        self.default_confirm = default_confirm      # _UNSET: exhausted script -> TerminalClosed
        self.default_choice = default_choice
        self.timeouts: list[tuple[str, str, "float | None"]] = []
        self.events: list[tuple[str, str, str]] = []
        self.notices: list[tuple[str, str, str]] = []
        self.asked: list[tuple[str, str]] = []
        self.conversations: list[tuple[str, str]] = []
        self.turns: list[tuple[str, bool, "RecordingSink"]] = []    # (source, show_text, sink) per turn()
        self.attached: Path | None = None

    # ---- scripting helpers
    def feed(self, *replies: Any) -> None:
        self._replies.extend(replies)

    @property
    def remaining(self) -> int:
        return len(self._replies)

    def said(self, source: str | None = None, include_llm: bool = True) -> list[str]:
        kinds = ("say", "llm") if include_llm else ("say",)
        return [t for k, s, t in self.events if k in kinds and (source is None or s == source)]

    # ---- recording
    def _record(self, kind: str, source: str, text: str) -> None:
        self.events.append((kind, source, text))
        if self.attached is not None:
            with open(self.attached, "a", encoding="utf-8") as f:
                f.write(json.dumps({"kind": kind, "source": source, "text": text}) + "\n")

    # ---- GatewayPort
    def attach_run(self, transcript_path: Path) -> None:
        self.attached = Path(transcript_path)
        self.attached.parent.mkdir(parents=True, exist_ok=True)
        with open(self.attached, "a", encoding="utf-8") as f:
            for kind, source, text in self.events:
                f.write(json.dumps({"kind": kind, "source": source, "text": text}) + "\n")

    def say(self, source: str, text: str, *, from_llm: bool = False) -> None:
        self._record("llm" if from_llm else "say", source, text)

    def notice(self, source: str, code: str, text: str) -> None:
        self.notices.append((source, code, text))
        self._record("notice", source, f"{code}: {text}")

    @staticmethod
    def _timeout(item: Any, source: str, timeout_s: "float | None") -> None:
        if item is TIMEOUT:
            assert timeout_s is not None, f"scripted TIMEOUT at {source!r} but the call has no timeout_s"
            raise ReplyTimeout(source, timeout_s)

    def ask(self, source: str, prompt: str, *, allow_quit: bool = False,
            commands: Sequence[str] = (), timeout_s: "float | None" = None) -> Reply:
        self.asked.append((source, prompt))
        self.timeouts.append(("ask", source, timeout_s))
        self._record("ask", source, prompt)
        if not self._replies:
            raise TerminalClosed("scripted gateway: no more replies (EOF)")
        item = self._replies.pop(0)
        self._timeout(item, source, timeout_s)
        if callable(item):
            item = item(source, prompt)
        if isinstance(item, Reply):
            return item
        text = str(item)
        if text.startswith("/"):
            cmd, _, rest = text.partition(" ")
            if cmd == "/quit":
                return Reply(text="", quit=allow_quit, command="/quit")
            return Reply(text=rest.strip(), command=cmd)
        return Reply(text=text)

    def choose(self, source: str, prompt: str, options: Sequence[str], *,
               timeout_s: "float | None" = None) -> int:
        self._record("choose", source, prompt + " | " + " / ".join(options))
        self.timeouts.append(("choose", source, timeout_s))
        if self._choices:
            idx = self._choices.pop(0)
        elif self.default_choice is not _UNSET:
            idx = self.default_choice
        else:
            raise TerminalClosed(f"scripted gateway: no more choices at {source!r} (EOF)")
        self._timeout(idx, source, timeout_s)
        if not 0 <= idx < len(options):
            raise IndexError(f"scripted choice {idx} out of range for {len(options)} options")
        return idx

    def confirm(self, source: str, prompt: str, *, timeout_s: "float | None" = None) -> bool:
        self._record("confirm", source, prompt)
        self.timeouts.append(("confirm", source, timeout_s))
        if self._confirms:
            item = self._confirms.pop(0)
        elif self.default_confirm is not _UNSET:
            item = self.default_confirm
        else:
            raise TerminalClosed(f"scripted gateway: no more confirms at {source!r} (EOF)")
        self._timeout(item, source, timeout_s)
        return bool(item)

    @contextmanager
    def turn(self, source: str, *, show_text: bool = True) -> Iterator["RecordingSink"]:
        sink = RecordingSink()
        self.turns.append((source, show_text, sink))
        yield sink

    @contextmanager
    def conversation(self, source: str) -> Iterator[None]:
        self.conversations.append((source, "enter"))
        try:
            yield
        finally:
            self.conversations.append((source, "exit"))
