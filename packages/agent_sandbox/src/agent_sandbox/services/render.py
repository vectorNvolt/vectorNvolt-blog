"""Rich rendering of one chat turn from the CLI's stream-json events.

Event flow per turn (with --include-partial-messages):
  stream_event  content_block_start / content_block_delta(text_delta) / content_block_stop
  assistant     finished message (content blocks: text, tool_use)
  user          tool results
  result        end of turn

Every string taken out of an event is untrusted: it was written by a process on a
writable rootfs, or by a file that process read. Each one goes through `sanitize()`
before Rich sees it, is bounded in size, and — where it is interpolated into Rich
*markup* rather than passed as `Text` — through `markup.escape()` as well. Links
render with their URL visible. Envelope fields (`is_error`, `duration_ms`,
`num_turns`) are shown as what the CLI reported, not as facts the harness checked.
"""
from __future__ import annotations

import json

from rich.console import Console
from rich.live import Live
from rich.markdown import Markdown
from rich.markup import escape
from rich.panel import Panel
from rich.status import Status
from rich.text import Text
from rich.theme import Theme

from agent_sandbox import config
from agent_sandbox.services.sanitize import sanitize

PALETTE = {
    "background": "#1b1918", "foreground": "#e4ded7",
    "black": "#262322", "red": "#d65d4e", "green": "#87a987", "yellow": "#e0a35e",
    "blue": "#6f8f9e", "magenta": "#b07d9b", "cyan": "#709d94", "white": "#e4ded7",
    "bright_black": "#524c48", "bright_red": "#e5786a", "bright_green": "#9cb89c",
    "bright_yellow": "#ebb475", "bright_blue": "#83a2b0", "bright_magenta": "#bf93ad",
    "bright_cyan": "#82aba2", "bright_white": "#faf8f5",
}
_P = PALETTE

THEME = Theme({
    "tail": _P["blue"],
    "status.spinner": _P["yellow"],
    "status": _P["yellow"],
    "tool.border": _P["cyan"],
    "tool.title": f"bold {_P['bright_cyan']}",
    "tool.body": _P["foreground"],
    "result.border": _P["bright_black"],
    "result.title": _P["bright_black"],
    "result.body": _P["foreground"],
    "error.border": _P["red"],
    "error.title": f"bold {_P['bright_red']}",
    "error.body": _P["bright_red"],
    "footer": _P["bright_black"],
    # markdown
    "markdown.text": _P["foreground"],
    "markdown.paragraph": _P["foreground"],
    "markdown.em": f"italic {_P['bright_magenta']}",
    "markdown.emph": f"italic {_P['bright_magenta']}",
    "markdown.strong": f"bold {_P['bright_white']}",
    "markdown.code": f"{_P['bright_green']} on {_P['black']}",
    "markdown.code_block": f"{_P['foreground']} on {_P['black']}",
    "markdown.block_quote": _P["magenta"],
    "markdown.item.bullet": f"bold {_P['cyan']}",
    "markdown.item.number": f"bold {_P['cyan']}",
    "markdown.hr": _P["bright_black"],
    "markdown.h1.border": _P["yellow"],
    "markdown.h1": f"bold {_P['bright_yellow']}",
    "markdown.h2": f"bold {_P['bright_yellow']}",
    "markdown.h3": f"bold {_P['yellow']}",
    "markdown.h4": f"bold {_P['yellow']}",
    "markdown.h5": _P["yellow"],
    "markdown.h6": _P["yellow"],
    "markdown.h7": _P["yellow"],
    "markdown.link": f"underline {_P['bright_blue']}",
    "markdown.link_url": _P["blue"],
    "markdown.s": f"strike {_P['bright_black']}",
})

console = Console(theme=THEME)


_TOOL_INPUT_MAX = 1500
_TOOL_RESULT_MAX = 1200
_TOOL_NAME_MAX = 64


def _clip(s: str, n: int) -> str:
    """Bound `s` to about `n` chars keeping the head *and* the tail: the half of a
    command that matters is as likely at the end as at the start."""
    if len(s) <= n:
        return s
    head, tail = n * 2 // 3, n // 3
    return f"{s[:head]}\n… ({len(s) - head - tail} chars omitted) …\n{s[-tail:]}"


def _md(text: str) -> Markdown:
    # hyperlinks=False: a link shows as `text (url)` instead of an OSC 8 sequence
    # whose target only the terminal knows. The block is sanitised and bounded
    # before markdown-it-py and Pygments run on it.
    return Markdown(_clip(sanitize(text), config.CHAT_MAX_TEXT_BLOCK), hyperlinks=False,
                    code_theme="ansi_dark", inline_code_theme="ansi_dark")


def _block_text(content) -> str:
    if isinstance(content, str):
        return content
    return "\n".join(str(b.get("text", "")) for b in content or []
                     if isinstance(b, dict) and b.get("type") == "text")


def _tool_name(name) -> str:
    """Sanitised, single-line, bounded. Where it goes into a Panel *title* — Rich
    markup, in which `[link=…]Read[/link]` would be obeyed — the caller also escapes."""
    name = sanitize(str(name or "?")).replace("\n", " ")
    return name[:_TOOL_NAME_MAX] + ("…" if len(name) > _TOOL_NAME_MAX else "")


class TurnRenderer:
    def __init__(self) -> None:
        self._status: Status | None = Status(Text("thinking…", style="status"), console=console)
        self._status.start()
        self._live: Live | None = None
        self._text = ""
        self._text_overflow = 0        # chars of a streamed block past CHAT_MAX_TEXT_BLOCK, not kept
        self._streamed_text = False
        self._finished = False
        self.dropped = 0               # events the gateway refused (malformed); shown in the footer

    # --- event dispatch ------------------------------------------------------

    def feed(self, ev: dict) -> None:
        t = ev.get("type")
        if t == "stream_event":
            self._stream(ev.get("event") or {})
        elif t == "assistant":
            self._assistant(ev.get("message") or {})
        elif t == "user":
            self._tool_results(ev.get("message") or {})
        elif t == "result":
            self._result(ev)

    def finish(self) -> None:
        self._stop_status()
        self._end_text()
        if self.dropped and not self._finished:
            console.print(Text(f"{self.dropped} malformed event(s) dropped", style="error.title"))
        self._finished = True

    # --- streaming text ------------------------------------------------------

    def _stream(self, e: dict) -> None:
        kind = e.get("type")
        if kind == "content_block_start":
            block = e.get("content_block") or {}
            if block.get("type") == "text":
                self._begin_text()
            elif block.get("type") == "tool_use":
                self._end_text()
                self._set_status(f"calling {_tool_name(block.get('name', 'tool'))}…")
        elif kind == "content_block_delta":
            delta = e.get("delta") or {}
            if delta.get("type") == "text_delta":
                if self._live is None:
                    self._begin_text()
                self._append(str(delta.get("text", "")))
                self._live.update(self._tail())
        elif kind == "content_block_stop":
            self._end_text()

    # A streamed block is bounded while it arrives: past the cap the deltas are
    # counted, not kept, so neither `_tail()` nor the final markdown pass grows
    # with what the model chooses to emit.
    def _append(self, s: str) -> None:
        room = config.CHAT_MAX_TEXT_BLOCK - len(self._text)
        if room > 0:
            self._text += s[:room]
        self._text_overflow += max(len(s) - room, 0)

    # While streaming, show only a plain-text tail in a transient Live: a Live taller than the
    # terminal re-emits scrolled-off lines and duplicates output. The full markdown is printed
    # once when the block ends.
    def _begin_text(self) -> None:
        self._stop_status()
        self._end_text()
        self._streamed_text = True
        self._live = Live(Text(""), console=console, transient=True, refresh_per_second=12,
                          vertical_overflow="crop")
        self._live.start()

    def _tail(self) -> Text:
        keep = max(console.size.height - 2, 3)
        # rsplit with a limit walks back only `keep` newlines, not the whole block.
        lines = self._text.rsplit("\n", keep)[-keep:]
        return Text(sanitize("\n".join(lines)), style="tail")

    def _end_text(self) -> None:
        if self._live is not None:
            self._live.stop()
            self._live = None
            if self._text.strip():
                console.print(_md(self._text))
            if self._text_overflow:
                console.print(Text(f"… ({self._text_overflow} more chars not shown)", style="footer"))
            self._text = ""
            self._text_overflow = 0

    # --- completed blocks ----------------------------------------------------

    def _assistant(self, msg: dict) -> None:
        self._end_text()
        content = msg.get("content")
        for b in content if isinstance(content, list) else []:
            if not isinstance(b, dict):
                continue
            if b.get("type") == "text":
                if not self._streamed_text and b.get("text"):
                    self._stop_status()
                    console.print(_md(str(b["text"])))
                self._streamed_text = False
            elif b.get("type") == "tool_use":
                self._stop_status()
                body = _clip(sanitize(json.dumps(b.get("input", {}), indent=2, ensure_ascii=False)), _TOOL_INPUT_MAX)
                console.print(Panel(Text(body, style="tool.body"),
                                    title=f"[tool.title]tool · {escape(_tool_name(b.get('name')))}[/]",
                                    border_style="tool.border", expand=False))
                self._set_status("running tool…")

    def _tool_results(self, msg: dict) -> None:
        content = msg.get("content")
        for b in content if isinstance(content, list) else []:
            if not isinstance(b, dict) or b.get("type") != "tool_result":
                continue
            self._stop_status()
            err = bool(b.get("is_error"))                     # the CLI's claim, shown as such — no decision hangs on it
            body = _clip(sanitize(_block_text(b.get("content"))), _TOOL_RESULT_MAX) or "(empty)"
            kind = "error" if err else "result"
            console.print(Panel(Text(body, style=f"{kind}.body"),
                                title=f"[{kind}.title]tool result" + (" · error (reported)" if err else "") + "[/]",
                                border_style=f"{kind}.border", expand=False))
        self._set_status("thinking…")

    def _result(self, ev: dict) -> None:
        self.finish()
        ms, turns = ev.get("duration_ms"), ev.get("num_turns")
        foot = f"{ms / 1000:.1f}s" if isinstance(ms, (int, float)) and not isinstance(ms, bool) else "?s"
        foot += f" · {turns if isinstance(turns, int) and not isinstance(turns, bool) else '?'} turn(s)"
        if ev.get("is_error"):
            foot += " · error"
        foot += " — as reported by the cli"
        console.print(Text(foot, style="footer"))

    # --- spinner -------------------------------------------------------------

    def _set_status(self, msg: str) -> None:
        if self._live is not None:
            return
        if self._status is None:
            self._status = Status(Text(msg, style="status"), console=console)
            self._status.start()
        else:
            self._status.update(Text(msg, style="status"))

    def _stop_status(self) -> None:
        if self._status is not None:
            self._status.stop()
            self._status = None
