"""Text rendering for the terminal (pure string functions; the gateway prints them).

Covers: SFR-AGG-038 (control sequences from Claude never reach the terminal raw),
SFR-AGG-011 (outline of the built graph), SSR-AGG-001 (render and gateway are the only
modules allowed to touch terminal I/O; this module returns strings and renderables, and
``TurnRenderer`` writes only to the rich Console the gateway injects; it never creates one).

Rich rendering of a streamed Claude turn (``TurnRenderer``) follows the event flow of
``--include-partial-messages``: stream_event (content blocks, text deltas), assistant (finished
message: text, tool_use), user (tool_result), result (end of turn). Every string taken out of an
event is untrusted: it goes through ``sanitize`` (visible markers) before rich sees it, is bounded
in size, and is passed through ``markup.escape`` where it is interpolated into markup. Envelope
fields (``is_error``, ``duration_ms``, ``num_turns``) are shown as what the CLI reported.
"""

import json
import re
from typing import Iterable

from rich.console import Console
from rich.live import Live
from rich.markdown import Markdown
from rich.markup import escape
from rich.panel import Panel
from rich.status import Status
from rich.text import Text
from rich.theme import Theme

from ..errors import Finding
from ..state import GraphSpec, ProcessDef
from .sanitize import sanitize

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
    # gateway lines
    "src": f"bold {_P['cyan']}",
    "say": _P["foreground"],
    "prompt": f"bold {_P['bright_yellow']}",
    "notice.warn": _P["yellow"],
    "notice.error": f"bold {_P['bright_red']}",
    "notice.progress": _P["bright_black"],
    "notice.info": _P["blue"],
    # turn renderer
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

TEXT_BLOCK_MAX = 20_000        # chars of one text block kept / rendered as markdown
TOOL_INPUT_MAX = 1500
TOOL_RESULT_MAX = 1200
TOOL_NAME_MAX = 64

# String-type escape sequences: OSC, DCS, SOS, PM, APC (terminated by BEL or ST).
_STRING_SEQ = re.compile(r"\x1b[\]PX^_].*?(?:\x07|\x1b\\|$)", re.DOTALL)
_CSI = re.compile(r"\x1b\[[0-?]*[ -/]*[@-~]")
_ESC_TWO = re.compile(r"\x1b[ -/]*[0-~]")
_CONTROLS = re.compile(r"[\x00-\x08\x0b-\x1f\x7f-\x9f]")


def strip_controls(text: str) -> str:
    """Remove terminal control sequences and control characters; keep newline and tab.

    ``\\r\\n`` becomes ``\\n``; a lone ``\\r`` is dropped (it could overwrite earlier output).
    """
    text = text.replace("\r\n", "\n")
    text = _STRING_SEQ.sub("", text)
    text = _CSI.sub("", text)
    text = _ESC_TWO.sub("", text)
    return _CONTROLS.sub("", text)


def format_findings(findings: Iterable[Finding]) -> str:
    """One line per finding: ``file:line: [code] message (step, item)``."""
    lines = []
    for f in findings:
        loc = ""
        if f.file:
            loc = f"{f.file}:{f.line}: " if f.line else f"{f.file}: "
        extra = ", ".join(x for x in (f"step {f.step}" if f.step else "",
                                      f"item {f.item}" if f.item else "") if x)
        lines.append(f"{loc}[{f.code}] {f.message}" + (f" ({extra})" if extra else ""))
    return "\n".join(lines)


def format_outline(spec: GraphSpec, proc: ProcessDef) -> str:
    """Outline of the built graph, in ``spec.order``: one block per node with type, reads,
    products and outgoing edges. Deterministic."""
    by_name = {n.name: n for n in spec.nodes}
    steps = {s.name: s for s in proc.steps}
    lines = [f"Process {proc.id}: {proc.title}", f"Entry: {spec.entry}"]
    for i, name in enumerate(spec.order, start=1):
        node = by_name.get(name)
        if node is None:
            continue
        step = steps.get(name)
        lines.append(f"{i}. {name} ({node.type})")
        if step is not None and step.instruction:
            first = step.instruction.strip().splitlines()[0] if step.instruction.strip() else ""
            if first:
                lines.append(f"   goal: {first}")
        if node.reads:
            lines.append("   reads: " + ", ".join(node.reads))
        if node.produces:
            lines.append("   produces: " + ", ".join(node.produces))
        for e in spec.edges:
            if e.src != name:
                continue
            cond = e.when if e.outcome is None else f"{e.when}={e.outcome}"
            lines.append(f"   -> {e.dst} [{cond}]")
    return "\n".join(lines)


# ---------------------------------------------------------------------- gateway line renderables
NOTICE_STYLES = {"WARNING": "notice.warn", "LLM_UNAVAILABLE": "notice.error",
                 "RESOLUTION_LIMIT": "notice.error", "PROGRESS": "notice.progress"}


def notice_style(code: str) -> str:
    return NOTICE_STYLES.get(code, "notice.info")


def tag_text(source: str) -> Text:
    """The visible, styled ``[source]`` tag (a Text: the source is never read as markup)."""
    return Text(f"[{source}]", style="src")


def tagged_line(source: str, line: str, style: str = "say") -> Text:
    """One output line: styled tag, a space, the line in ``style`` (no markup, no highlighting)."""
    t = tag_text(source)
    t.append(" ")
    t.append(line, style=style)
    return t


def markdown(text: str) -> Markdown:
    """Themed markdown of untrusted text. ``hyperlinks=False``: a link shows as ``text (url)``
    instead of an OSC 8 sequence whose target only the terminal knows."""
    return Markdown(_clip(sanitize(text), TEXT_BLOCK_MAX), hyperlinks=False,
                    code_theme="ansi_dark", inline_code_theme="ansi_dark")


def _clip(s: str, n: int) -> str:
    """Bound ``s`` to about ``n`` chars keeping the head and the tail."""
    if len(s) <= n:
        return s
    head, tail = n * 2 // 3, n // 3
    return f"{s[:head]}\n… ({len(s) - head - tail} chars omitted) …\n{s[-tail:]}"


def _block_text(content) -> str:
    if isinstance(content, str):
        return content
    return "\n".join(str(b.get("text", "")) for b in content or []
                     if isinstance(b, dict) and b.get("type") == "text")


def _tool_name(name) -> str:
    """Sanitised, single-line, bounded. In a Panel title (markup) the caller also escapes."""
    name = sanitize(str(name or "?")).replace("\n", " ")
    return name[:TOOL_NAME_MAX] + ("…" if len(name) > TOOL_NAME_MAX else "")


class TurnRenderer:
    """Renders the stream-json events of one Claude turn on the injected ``console``.

    ``show_text=False`` keeps spinner, tool panels, result panels and footer but shows no assistant
    text (the caller prints the text itself, e.g. a dialog whose reply is JSON). ``dropped`` is the
    number of malformed events refused upstream; it is announced when the turn ends."""

    def __init__(self, console: Console, *, show_text: bool = True) -> None:
        self._console = console
        self._show_text = show_text
        self._status: Status | None = Status(Text("thinking…", style="status"), console=console)
        self._status.start()
        self._live: Live | None = None
        self._text = ""
        self._text_overflow = 0        # chars of a streamed block past TEXT_BLOCK_MAX, not kept
        self._streamed_text = False
        self._finished = False
        self.dropped = 0

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
            self._console.print(Text(f"{self.dropped} malformed event(s) dropped", style="error.title"))
        self._finished = True

    # --- streaming text ------------------------------------------------------
    def _stream(self, e: dict) -> None:
        kind = e.get("type")
        if kind == "content_block_start":
            block = e.get("content_block") or {}
            if block.get("type") == "text":
                if self._show_text:
                    self._begin_text()
            elif block.get("type") == "tool_use":
                self._end_text()
                self._set_status(f"calling {_tool_name(block.get('name', 'tool'))}…")
        elif kind == "content_block_delta":
            delta = e.get("delta") or {}
            if delta.get("type") == "text_delta" and self._show_text:
                if self._live is None:
                    self._begin_text()
                self._append(str(delta.get("text", "")))
                self._live.update(self._tail())
        elif kind == "content_block_stop":
            self._end_text()

    # A streamed block is bounded while it arrives: past the cap the deltas are counted, not kept.
    def _append(self, s: str) -> None:
        room = TEXT_BLOCK_MAX - len(self._text)
        if room > 0:
            self._text += s[:room]
        self._text_overflow += max(len(s) - room, 0)

    # While streaming, show only a plain-text tail in a transient Live (a Live taller than the
    # terminal re-emits scrolled-off lines). The full markdown is printed once when the block ends.
    def _begin_text(self) -> None:
        self._stop_status()
        self._end_text()
        self._streamed_text = True
        self._live = Live(Text(""), console=self._console, transient=True, refresh_per_second=12,
                          vertical_overflow="crop")
        self._live.start()

    def _tail(self) -> Text:
        keep = max(self._console.size.height - 2, 3)
        lines = self._text.rsplit("\n", keep)[-keep:]
        return Text(sanitize("\n".join(lines)), style="tail")

    def _end_text(self) -> None:
        if self._live is not None:
            self._live.stop()
            self._live = None
            if self._text.strip():
                self._console.print(markdown(self._text))
            if self._text_overflow:
                self._console.print(Text(f"… ({self._text_overflow} more chars not shown)", style="footer"))
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
                if self._show_text and not self._streamed_text and b.get("text"):
                    self._stop_status()
                    self._console.print(markdown(str(b["text"])))
                self._streamed_text = False
            elif b.get("type") == "tool_use":
                self._stop_status()
                body = _clip(sanitize(json.dumps(b.get("input", {}), indent=2, ensure_ascii=False)),
                             TOOL_INPUT_MAX)
                self._console.print(Panel(Text(body, style="tool.body"),
                                          title=f"[tool.title]tool · {escape(_tool_name(b.get('name')))}[/]",
                                          border_style="tool.border", expand=False))
                self._set_status("running tool…")

    def _tool_results(self, msg: dict) -> None:
        content = msg.get("content")
        for b in content if isinstance(content, list) else []:
            if not isinstance(b, dict) or b.get("type") != "tool_result":
                continue
            self._stop_status()
            err = bool(b.get("is_error"))                     # the CLI's claim, shown as such
            body = _clip(sanitize(_block_text(b.get("content"))), TOOL_RESULT_MAX) or "(empty)"
            kind = "error" if err else "result"
            self._console.print(Panel(Text(body, style=f"{kind}.body"),
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
        self._console.print(Text(foot, style="footer"))

    # --- spinner -------------------------------------------------------------
    def _set_status(self, msg: str) -> None:
        if self._live is not None:
            return
        if self._status is None:
            self._status = Status(Text(msg, style="status"), console=self._console)
            self._status.start()
        else:
            self._status.update(Text(msg, style="status"))

    def _stop_status(self) -> None:
        if self._status is not None:
            self._status.stop()
            self._status = None
