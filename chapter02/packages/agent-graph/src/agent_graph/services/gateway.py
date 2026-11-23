"""The gateway: the only owner of the developer terminal.

Covers: SFR-AGG-038 (one conversation at a time, every message tagged with its source,
transcript in gateway.jsonl once the run folder exists, no other module reads or writes
the terminal), SSR-AGG-001 (single terminal owner), SFR-AGG-036 (EOF -> TerminalClosed),
SFR-AGG-016/decision 19 (concurrent conversations queue, they do not preempt).

Output format: every line is prefixed with the styled tag ``[<source>] ``, written through one rich
Console owned here (no ANSI and no Live off a TTY). Text that came from Claude (``from_llm=True``) and
notices are sanitised first (control sequences become visible markers); LLM text is then rendered as
markdown on a TTY. ``turn()`` streams one Claude turn (spinner, live tail, tool panels, footer).
Explicit commands: ``/quit`` (only where ``allow_quit``; always confirmed with y/N) and any
command listed in ``commands`` (e.g. ``/resolved``, ``/show``). Any other input, including text
that starts with "/", is plain text.

Bounded waits (SFR-AGG-016, human steps): ``ask``/``choose``/``confirm`` take ``timeout_s``; the
whole call (re-asks and the /quit confirmation included) must get a valid reply before it expires,
else ``ReplyTimeout`` is raised and a ``timeout`` record is written. When stdin has a real file
descriptor every read goes through ``_FdReader`` (``select`` + ``os.read``, own line buffer);
otherwise ``_ThreadReader`` reads on request in one daemon thread, with at most one read
outstanding, so no thread keeps consuming input on its own. Input that was typed after a timeout
and before the next prompt is dropped (recorded as ``stale``), so a late "y" never answers a
different question.
"""

import io
import json
import os
import queue
import select
import sys
import threading
import time
from contextlib import contextmanager
from pathlib import Path
from typing import IO, Any, Callable, Iterator, Sequence

from rich.console import Console

from ..errors import ReplyTimeout, TerminalClosed
from ..ports import Reply
from .render import THEME, TurnRenderer, markdown, notice_style, strip_controls, tag_text, tagged_line
from .sanitize import sanitize

__all__ = ["Gateway", "strip_controls"]


class Gateway:
    def __init__(self, stdin: IO[str] | None = None, stdout: IO[str] | None = None,
                 clock: Callable[[], float] = time.time, *, force_terminal: bool | None = None):
        self._stdin = stdin if stdin is not None else sys.stdin
        self._stdout = stdout if stdout is not None else sys.stdout
        self._clock = clock
        # The one rich Console of the process; off a TTY it emits no escape codes and no Live.
        self._console = Console(file=self._stdout, theme=THEME, highlight=False, emoji=False,
                                force_terminal=force_terminal)
        self._conv = threading.RLock()      # one conversation at a time (queue, no preemption)
        self._out = threading.Lock()        # serializes terminal writes and transcript appends
        self._buffer: list[dict[str, Any]] = []
        self._transcript: Path | None = None
        self._input = _make_reader(self._stdin)
        self._stale = False                     # a read timed out: drop input typed before the next prompt

    # ------------------------------------------------------------------ transcript
    def attach_run(self, transcript_path: Path) -> None:
        """Flush the in-memory transcript to ``transcript_path`` and append from now on."""
        with self._out:
            path = Path(transcript_path)
            path.parent.mkdir(parents=True, exist_ok=True)
            self._transcript = path
            pending, self._buffer = self._buffer, []
            self._append_records(pending)

    def _append_records(self, records: Sequence[dict[str, Any]]) -> None:
        if not records or self._transcript is None:
            return
        with open(self._transcript, "ab") as f:
            for rec in records:
                f.write((json.dumps(rec, sort_keys=True, ensure_ascii=False) + "\n").encode("utf-8"))
            f.flush()
            os.fsync(f.fileno())

    def _record(self, kind: str, source: str, text: str) -> None:
        rec = {"ts": self._clock(), "kind": kind, "source": source, "text": text}
        if self._transcript is None:
            self._buffer.append(rec)
        else:
            self._append_records([rec])

    # ------------------------------------------------------------------ output
    def _write(self, source: str, text: str, style: str = "say") -> None:
        """Plain lines, each with the styled source tag. ``soft_wrap``: no rich re-wrapping, the
        terminal wraps; off a TTY the output is the same text without any escape codes."""
        lines = text.split("\n") if text else [""]
        for line in lines:
            self._console.print(tagged_line(source, line, style), soft_wrap=True)
        self._stdout.flush()

    def _write_markdown(self, source: str, text: str) -> None:
        """LLM text as themed markdown below its tag line. Off a TTY: plain tagged lines."""
        if not self._console.is_terminal:
            self._write(source, text)
            return
        self._console.print(tag_text(source), soft_wrap=True)
        self._console.print(markdown(text))
        self._stdout.flush()

    def say(self, source: str, text: str, *, from_llm: bool = False) -> None:
        if from_llm:
            text = sanitize(text)
        with self._conv, self._out:
            if from_llm:
                self._write_markdown(source, text)
            else:
                self._write(source, text)
            self._record("llm" if from_llm else "say", source, text)

    def notice(self, source: str, code: str, text: str) -> None:
        """Not part of a conversation: printed at once (never blocks on a running dialog)."""
        text = sanitize(text)
        with self._out:
            self._write(source, f"{code}: {text}", notice_style(code))
            self._record("notice", source, f"{code}: {text}")

    @contextmanager
    def conversation(self, source: str) -> Iterator[None]:
        """Hold the terminal for one conversation; other conversations wait their turn."""
        with self._conv:
            yield

    @contextmanager
    def turn(self, source: str, *, show_text: bool = True) -> Iterator[TurnRenderer]:
        """One streamed Claude turn: holds the conversation and the terminal, yields a renderer
        (``feed(event)``, ``dropped``) and finishes it on exit. Nothing is transcribed here: the
        caller still ``say``s the final text once (``show_text=False`` when the streamed text is
        not what the operator should read, e.g. a JSON dialog reply)."""
        with self._conv, self._out:
            renderer = TurnRenderer(self._console, show_text=show_text)
            try:
                yield renderer
            finally:
                renderer.finish()
                self._stdout.flush()

    # ------------------------------------------------------------------ input
    @staticmethod
    def _deadline(timeout_s: "float | None") -> "float | None":
        return None if timeout_s is None else time.monotonic() + max(0.0, float(timeout_s))

    def _readline(self, source: str, deadline: "float | None" = None, timeout_s: "float | None" = None) -> str:
        with self._out:
            prompt = tag_text(source)
            prompt.append(" > ", style="prompt")
            self._console.print(prompt, end="", soft_wrap=True)
            self._stdout.flush()
        if self._stale:
            self._drop_stale(source)
        line = self._input.readline(deadline)
        if line is None:
            self._stale = True                  # a late reply must not answer the next prompt
            with self._out:
                self._write(source, "(no reply in time)", "notice.warn")
                self._record("timeout", source, f"no reply within {timeout_s:g}s" if timeout_s is not None
                             else "no reply in time")
            raise ReplyTimeout(source, timeout_s if timeout_s is not None else 0.0)
        if line == "":
            raise TerminalClosed("end of input on the developer terminal")
        line = line.rstrip("\r\n")
        with self._out:
            self._record("reply", source, line)
        return line

    def _drop_stale(self, source: str) -> None:
        """After a timeout: discard what was typed before this prompt (late answers to the expired one)."""
        self._stale = False
        for line in self._input.drain():
            with self._out:
                self._record("stale", source, line.rstrip("\r\n"))

    def ask(self, source: str, prompt: str, *, allow_quit: bool = False,
            commands: Sequence[str] = (), timeout_s: "float | None" = None) -> Reply:
        deadline = self._deadline(timeout_s)
        with self._conv:
            with self._out:
                self._write(source, prompt, "prompt")
                self._record("ask", source, prompt)
            while True:
                line = self._readline(source, deadline, timeout_s).strip()
                if line.startswith("/"):
                    cmd, _, rest = line.partition(" ")
                    if cmd == "/quit":
                        if not allow_quit:
                            self._hint(source, "/quit is not available here")
                            continue
                        left = None if deadline is None else max(0.0, deadline - time.monotonic())
                        if self.confirm(source, "Quit and leave the run as it is?", timeout_s=left):
                            return Reply(text="", quit=True, command="/quit")
                        continue
                    if cmd in commands:
                        return Reply(text=rest.strip(), command=cmd)
                return Reply(text=line)

    def _hint(self, source: str, text: str) -> None:
        with self._out:
            self._write(source, text, "notice.warn")
            self._record("say", source, text)

    def choose(self, source: str, prompt: str, options: Sequence[str], *,
               timeout_s: "float | None" = None) -> int:
        """Numbered choice; returns the 0-based index. Re-asks on invalid input."""
        deadline = self._deadline(timeout_s)
        with self._conv:
            body = "\n".join([prompt] + [f"  {i}) {opt}" for i, opt in enumerate(options, start=1)])
            with self._out:
                self._write(source, body, "prompt")
                self._record("choose", source, body)
            while True:
                line = self._readline(source, deadline, timeout_s).strip()
                if line.isdigit() and 1 <= int(line) <= len(options):
                    return int(line) - 1
                self._hint(source, f"Enter a number from 1 to {len(options)}.")

    def confirm(self, source: str, prompt: str, *, timeout_s: "float | None" = None) -> bool:
        """Explicit y/n; anything else (including an empty line) is asked again."""
        deadline = self._deadline(timeout_s)
        with self._conv:
            with self._out:
                self._write(source, f"{prompt} (y/n)", "prompt")
                self._record("confirm", source, prompt)
            while True:
                line = self._readline(source, deadline, timeout_s).strip().lower()
                if line in ("y", "yes"):
                    return True
                if line in ("n", "no"):
                    return False
                self._hint(source, "Answer y or n.")


# ---------------------------------------------------------------------- line readers
def _make_reader(stdin: IO[str]) -> "_FdReader | _ThreadReader":
    """A reader on the real file descriptor when ``stdin`` has one, else a reader thread."""
    try:
        fd = stdin.fileno()
        os.fstat(fd)
    except (AttributeError, OSError, ValueError, io.UnsupportedOperation):
        return _ThreadReader(stdin)
    return _FdReader(fd, getattr(stdin, "encoding", None) or "utf-8")


class _FdReader:
    """Line reader on a file descriptor with ``select`` for the deadline. All input goes through
    this reader (never through the text wrapper's buffer), so a line that arrives after a timeout
    stays here for the next read; nothing else consumes it."""

    def __init__(self, fd: int, encoding: str):
        self._fd = fd
        self._enc = encoding
        self._buf = b""
        self._eof = False

    def _take_line(self) -> "str | None":
        i = self._buf.find(b"\n")
        if i >= 0:
            line, self._buf = self._buf[:i + 1], self._buf[i + 1:]
            return line.decode(self._enc, errors="replace")
        if self._eof:
            line, self._buf = self._buf, b""
            return line.decode(self._enc, errors="replace")      # "" at EOF
        return None

    def _fill(self, wait: "float | None") -> bool:
        """Read what is available; False when ``wait`` seconds passed without input."""
        if wait is not None:
            ready, _, _ = select.select([self._fd], [], [], max(0.0, wait))
            if not ready:
                return False
        chunk = os.read(self._fd, 65536)
        if chunk:
            self._buf += chunk
        else:
            self._eof = True
        return True

    def readline(self, deadline: "float | None") -> "str | None":
        """One line ("" at EOF), or None when ``deadline`` (time.monotonic) passed first."""
        while True:
            line = self._take_line()
            if line is not None:
                return line
            wait = None if deadline is None else max(0.0, deadline - time.monotonic())
            if not self._fill(wait):
                return None

    def drain(self) -> "list[str]":
        """Complete lines that are available now (buffered or readable without waiting)."""
        while not self._eof and self._fill(0.0):
            pass
        out = []
        while b"\n" in self._buf:
            out.append(self._take_line() or "")
        return out


class _ThreadReader:
    """Fallback for a ``stdin`` without a file descriptor: one daemon thread reads a line only
    when a read was requested, so at most one read is outstanding. A read that timed out stays
    outstanding and its line is handed to the next ``readline`` (or dropped by ``drain``); the
    thread never reads ahead on its own."""

    def __init__(self, stdin: IO[str]):
        self._stdin = stdin
        self._lines: "queue.Queue[str]" = queue.Queue()
        self._requests: "queue.Queue[None]" = queue.Queue()
        self._pending = False                   # a request was made and its line not yet taken
        self._thread: "threading.Thread | None" = None

    def _run(self) -> None:
        while True:
            self._requests.get()
            try:
                line = self._stdin.readline()
            except (OSError, ValueError):
                line = ""
            self._lines.put(line)

    def readline(self, deadline: "float | None") -> "str | None":
        if deadline is None and self._thread is None:
            return self._stdin.readline()       # no timer needed and no thread yet: read directly
        if self._thread is None:
            self._thread = threading.Thread(target=self._run, name="gateway-stdin", daemon=True)
            self._thread.start()
        if not self._pending:
            self._pending = True
            self._requests.put(None)
        try:
            line = self._lines.get(timeout=None if deadline is None else max(0.0, deadline - time.monotonic()))
        except queue.Empty:
            return None
        self._pending = False
        return line

    def drain(self) -> "list[str]":
        out = []
        try:
            while True:
                out.append(self._lines.get_nowait())
                self._pending = False
        except queue.Empty:
            pass
        return [x for x in out if x]            # "" is EOF: the next read sees it again
