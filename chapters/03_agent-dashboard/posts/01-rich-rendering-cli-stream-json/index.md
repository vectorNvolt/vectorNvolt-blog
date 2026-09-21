---
title: Rich rendering of the Claude CLI stream-json session
type: dev
code_tag: ch03_ag-dsh/p01_rich-rnd__post_code
code_package: agent_sandbox
slug: rich-rendering-cli-stream-json
status: draft
tags: [langgraph, agent-sandbox, agent-dashboard, rich]
custom_excerpt: "Node 6 of the harness stops spawning one `claude -p` per turn and keeps a single stream-json process alive inside the sandbox. A rich renderer turns its NDJSON events into a live terminal view — and the same change is what gives the agent a memory and the API a warm prompt cache."
feature_image:
---

## Where this post starts

The [chapter 02 harness](/minimal-sandbox-exec-langgraph-harness/) ended with node 6 running `claude -p --output-format text` once per turn through `exec_run`, printing whatever came back with a bare `print()`. It worked, and it had two problems that are really one problem. Every turn was a fresh process, so the agent forgot the previous turn the moment it answered. And the only thing the operator saw was the final text — no sign that the model was thinking, no tool calls, no tool results, no timing.

This post replaces node 6. One `claude` process per container, started once and kept alive across turns, speaking newline-delimited JSON in both directions. The harness feeds each event to a renderer built on [Rich](https://github.com/Textualize/rich): a spinner while the model thinks, a live tail while text streams, the finished answer as themed markdown, tool calls and results as panels, a footer with duration and turn count. Because the process lives on, the CLI keeps the conversation itself — which is also exactly the shape the Anthropic prompt cache wants.

Everything below is in `packages/agent_sandbox/src/agent_sandbox/`. The graph, the state, nodes 1–5 and the container hardening from chapter 02 are untouched.

<style>
.id-col-table th:first-child,
.id-col-table td:first-child {
  white-space: nowrap;
  width: 1%;
}
</style>

## What changed

<div class="id-col-table">

| File | Change |
|---|---|
| `services/render.py` | **new** — `PALETTE`, `THEME`, a themed `Console`, and `TurnRenderer` |
| `services/container.py` | `ExecSession(tty=False)`: demuxes Docker's framed stdout/stderr, adds `readline()` and `close_stdin()` |
| `services/gateway.py` | `start_chat()`, `send_chat_turn()`, `close_chat()` — the persistent session |
| `config.py` | `CHAT_CMD`, `CHAT_PERMISSION_MODE` (default `auto`), `CHAT_TURN_TIMEOUT`, `CHAT_CLOSE_TIMEOUT` |
| `nodes/n06_exchange.py` | uses the session instead of `send_prompt()`; `/quit` closes it |
| `pyproject.toml` | `rich>=13` |

</div>

## The building blocks

{{island:blocks}}

Hover or click a badge for what crosses each link. Reading left to right:

- **Operator terminal.** Two channels, as before: questions reach the operator through `interrupt()` answered by `input()` in `main.py` (B-1); everything the agent produces reaches the terminal through the `Console` in `render.py` (B-2). The second channel is new — in chapter 02 the answer was a `print()` after the turn ended, now it is painted *during* the turn.
- **Host process.** `n06_exchange` is still the only node that talks to the agent. It calls `gateway.send_chat_turn()`, which owns the loop: write one JSON line, read events until `result`, hand every event to `TurnRenderer.feed()` (B-3). The gateway reads and writes through an `ExecSession` (B-4) that `container.py` keeps in a module-level dict, keyed by container id, so it outlives graph passes and interrupts.
- **Docker Engine.** The session is an exec instance created with `stdin=True, tty=False` and started with `socket=True`. Without a TTY, Docker multiplexes stdout and stderr onto one socket with 8-byte frame headers (B-5); the login relay from chapter 02 still uses `tty=True` because `claude auth login` needs a terminal, but NDJSON does not want one — a TTY would echo stdin back and translate `\n` to `\r\n`.
- **Agent container.** The exec instance is a single long-lived `claude -p --input-format stream-json --output-format stream-json` process (B-6). It holds the conversation in memory, runs tools inside the gVisor sandbox under `--permission-mode auto`, and is the only party that ever talks to the Anthropic API (B-7). The harness never sees an API key, a system prompt or a tool definition; it sees events.

## One turn, step by step

{{island:flow}}

The sequence is one exchange turn in which the model calls a tool once — the shape of the third turn in the probe further down. Steps S-1…S-9 are the harness's side; the two API calls in the middle are the CLI's.

1. **S-1.** The previous pass of `exchange()` ended on `interrupt("Your reply (or /quit):")`. `main.py` collects the reply with `input()` and re-invokes the graph with `Command(resume=text)`; the node runs with `turn == "agent"`, takes the last user message from `state["transcript"]`, and calls `send_chat_turn()`.
2. **S-2.** One JSON line goes down the socket: `{"type": "user", "message": {"role": "user", "content": text}, ...}\n`. On the very first pass `start_chat()` opened the session; every later pass reuses it.
3. **S-3.** The CLI appends the message to its conversation and calls the API with the system prompt, the tool definitions and the whole history. This is where the prompt cache earns its keep — see below.
4. **S-4.** With `--include-partial-messages` the CLI forwards the API's stream as `stream_event`s. `content_block_start` of a `text` block stops the spinner and opens a transient `Live`; each `content_block_delta` appends to a plain-text tail; `content_block_stop` closes the `Live` and prints the block once as themed `Markdown`.
5. **S-5.** The `assistant` event carries the finished message. A `tool_use` block becomes a cyan panel with the tool's JSON input; the spinner switches to `running tool…`.
6. **S-6.** The tool runs inside the container. Nobody can answer a permission prompt through this channel, so the session runs with `--permission-prompts none` and lets `--permission-mode auto` decide: a classifier approves safe actions, anything else is denied instead of blocking the process.
7. **S-7.** The result comes back as a `user` event with `tool_result` blocks — a grey panel, or a red `tool result · error` panel when `is_error` is set. The spinner returns to `thinking…` while the CLI calls the API a second time with the history plus the tool result.
8. **S-8.** `result` ends the turn: the renderer prints `7.1s · 2 turn(s)` and `send_chat_turn()` returns `ev["result"]`. The node appends it to `transcript` as an agent message and flips `turn` to `"user"`.
9. **S-9.** The next pass runs `ask_user()` first — an `interrupt()` — so the graph pauses with the CLI process alive and its history intact. `/quit` calls `close_chat()`: shut down the socket's write side (EOF on the CLI's stdin), drain until the process exits, drop the session.

Events the renderer ignores: the `system` init event (model, tools, `permissionMode`, `session_id`), later `system` status events, and `rate_limit_event`. They go through `feed()` and fall out of the `type` dispatch — a later post's dashboard will want them.

## The code

### `config.py` — the session command

```python
# Persistent chat session: one `claude` process per container speaking NDJSON both ways.
# The gateway only relays text, so nobody can answer a permission prompt: `--permission-prompts
# none` auto-denies anything that would prompt instead of blocking the process. Which tool calls
# reach a prompt is decided by CHAT_PERMISSION_MODE (`claude --permission-mode`):
#   auto               (default) classifier approves safe actions, the rest are denied
#   bypassPermissions  no checks at all — every tool call runs (gVisor is the only boundary)
#   ""                 CLI default mode — effectively chat-only, all tool calls denied
CHAT_PERMISSION_MODE = os.getenv("CHAT_PERMISSION_MODE", "auto")
CHAT_CMD = [CLAUDE_BIN, "-p", "--output-format", "stream-json", "--input-format", "stream-json",
            "--verbose", "--include-partial-messages", "--permission-prompts", "none"] \
    + (["--permission-mode", CHAT_PERMISSION_MODE] if CHAT_PERMISSION_MODE else [])
CHAT_TURN_TIMEOUT = float(os.getenv("CHAT_TURN_TIMEOUT", "600"))    # a turn with tool calls can be long
CHAT_CLOSE_TIMEOUT = float(os.getenv("CHAT_CLOSE_TIMEOUT", "10"))
```

`--verbose` is required by the CLI for `stream-json` output in `-p` mode; `--include-partial-messages` is what turns on the `stream_event`s that make the live tail possible.

### `container.py` — a non-TTY exec session

`ExecSession.__init__` gained a `tty` flag, passed to both `exec_create(c.id, cmd, stdin=True, tty=tty)` and `exec_start(self.exec_id, socket=True, tty=tty)`, and three buffers: `_raw` (undecoded bytes, possibly a partial frame), `_stdout` (decoded text waiting for a newline) and `stderr`. The login relay's `read_until()` is unchanged; the session protocol uses the new methods:

```python
    def readline(self, timeout: float) -> str | None:
        """Next newline-terminated stdout line (non-TTY sessions).
        Returns None on timeout, "" on EOF. stderr frames accumulate in `self.stderr`."""
        deadline = time.monotonic() + timeout
        while True:
            nl = self._stdout.find("\n")
            if nl >= 0:
                line, self._stdout = self._stdout[:nl], self._stdout[nl + 1:]
                return line
            if time.monotonic() >= deadline:
                return None
            try:
                chunk = self._sock.recv(65536)
            except socket.timeout:
                continue
            if not chunk:
                return ""
            self._raw += chunk
            self._demux()

    def _demux(self) -> None:
        if self._tty:
            self._stdout += self._raw.decode(errors="replace")
            self._raw = b""
            return
        while len(self._raw) >= 8:
            stream, size = struct.unpack(">BxxxL", self._raw[:8])
            if len(self._raw) < 8 + size:
                return
            payload, self._raw = self._raw[8:8 + size], self._raw[8 + size:]
            if stream == 2:
                self.stderr += payload.decode(errors="replace")
            else:
                self._stdout += payload.decode(errors="replace")

    def close_stdin(self) -> None:
        """Send EOF on the process's stdin (what `docker exec` does on CloseWrite)."""
        try:
            self._sock.shutdown(socket.SHUT_WR)
        except OSError:
            pass
```

The frame header is Docker's attach protocol: one byte for the stream (1 = stdout, 2 = stderr), three padding bytes, a big-endian 32-bit payload length. `_demux()` only consumes a frame once all of it has arrived, so a `recv()` that ends mid-frame — or mid-line — is harmless; the remainder is picked up on the next call. `readline()` returns three distinct things and the gateway relies on all three: a line, `None` when the per-call timeout elapsed, `""` when the process closed its stdout.

### `gateway.py` — the persistent session

```python
# --- persistent chat session --------------------------------------------------
# One `claude -p --input-format stream-json --output-format stream-json` process per
# container, kept alive across turns so the CLI holds the conversation itself.
# stdin: one JSON user message per line; stdout: NDJSON events, `result` ends a turn.

def start_chat(c) -> None:
    ct.open_session(c, config.CHAT_CMD, tty=False)


def send_chat_turn(c, text: str) -> str:
    """Send one user turn, render the streamed events, return the final answer text."""
    s = ct.get_session(c)
    if s is None:
        return "[no chat session]"
    s.write(json.dumps({"type": "user", "message": {"role": "user", "content": text},
                        "parent_tool_use_id": None, "session_id": "default"}) + "\n")
    r = TurnRenderer()
    deadline = time.monotonic() + config.CHAT_TURN_TIMEOUT
    try:
        while (left := deadline - time.monotonic()) > 0:
            line = s.readline(timeout=left)
            if line == "":
                ct.close_session(c)
                return f"[claude exited {s.exit_code()}]\n{_clean(s.stderr)}"
            if not line or not line.strip():
                continue
            try:
                ev = json.loads(line)
            except json.JSONDecodeError:
                continue
            r.feed(ev)
            if ev.get("type") == "result":
                answer = ev.get("result") or ""
                return f"[claude error]\n{answer}" if ev.get("is_error") else answer
        return "[claude turn timed out]"
    finally:
        r.finish()


def close_chat(c) -> None:
    s = ct.get_session(c)
    if s is None:
        return
    s.close_stdin()
    deadline = time.monotonic() + config.CHAT_CLOSE_TIMEOUT
    while (left := deadline - time.monotonic()) > 0 and s.readline(timeout=left) != "":
        pass
    ct.close_session(c)
```

`send_chat_turn()` is the whole protocol. The renderer is created per turn and always finished in the `finally`, so a timeout or a crashed CLI never leaves a spinner or a `Live` region running on the terminal. The return value is the plain text of the answer — what the node stores in `transcript` — while everything the operator *sees* was already printed by the renderer on the way through.

### `render.py` — theme and renderer

Colours live in one place. `PALETTE` is a 16-colour terminal scheme; `THEME` maps Rich style names to it, including Rich's own `markdown.*` names, which is how headings, code spans, lists and links inside the model's answer pick up the palette without any per-call styling:

```python
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


def _md(text: str) -> Markdown:
    return Markdown(text, code_theme="ansi_dark", inline_code_theme="ansi_dark")

_TOOL_INPUT_MAX = 1500
_TOOL_RESULT_MAX = 1200
```

`code_theme="ansi_dark"` makes fenced code blocks use the terminal's own ANSI colours rather than a Pygments palette with hard-coded RGB, so they match whatever the terminal's scheme is. The two `_MAX` constants clip tool inputs and results in the panels — a `cat` of a large file should not scroll the conversation off the screen.

The renderer is one class with one public method per direction: `feed(ev)` in, terminal output out.

```python
class TurnRenderer:
    def __init__(self) -> None:
        self._status: Status | None = Status(Text("thinking…", style="status"), console=console)
        self._status.start()
        self._live: Live | None = None
        self._text = ""
        self._streamed_text = False

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

    # --- streaming text ------------------------------------------------------

    def _stream(self, e: dict) -> None:
        kind = e.get("type")
        if kind == "content_block_start":
            block = e.get("content_block") or {}
            if block.get("type") == "text":
                self._begin_text()
            elif block.get("type") == "tool_use":
                self._end_text()
                self._set_status(f"calling {block.get('name', 'tool')}…")
        elif kind == "content_block_delta":
            delta = e.get("delta") or {}
            if delta.get("type") == "text_delta":
                if self._live is None:
                    self._begin_text()
                self._text += delta.get("text", "")
                self._live.update(self._tail())
        elif kind == "content_block_stop":
            self._end_text()

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
        lines = self._text.splitlines()
        keep = max(console.size.height - 2, 3)
        return Text("\n".join(lines[-keep:]), style="tail")

    def _end_text(self) -> None:
        if self._live is not None:
            self._live.stop()
            self._live = None
            if self._text.strip():
                console.print(_md(self._text))
            self._text = ""

    # --- completed blocks ----------------------------------------------------

    def _assistant(self, msg: dict) -> None:
        self._end_text()
        for b in msg.get("content") or []:
            if b.get("type") == "text":
                if not self._streamed_text and b.get("text"):
                    self._stop_status()
                    console.print(_md(b["text"]))
                self._streamed_text = False
            elif b.get("type") == "tool_use":
                self._stop_status()
                body = _clip(json.dumps(b.get("input", {}), indent=2, ensure_ascii=False), _TOOL_INPUT_MAX)
                console.print(Panel(Text(body, style="tool.body"), title=f"[tool.title]tool · {b.get('name', '?')}[/]",
                                    border_style="tool.border", expand=False))
                self._set_status("running tool…")

    def _tool_results(self, msg: dict) -> None:
        for b in msg.get("content") or []:
            if not isinstance(b, dict) or b.get("type") != "tool_result":
                continue
            self._stop_status()
            err = bool(b.get("is_error"))
            body = _clip(_block_text(b.get("content")), _TOOL_RESULT_MAX) or "(empty)"
            kind = "error" if err else "result"
            console.print(Panel(Text(body, style=f"{kind}.body"),
                                title=f"[{kind}.title]tool result" + (" · error" if err else "") + "[/]",
                                border_style=f"{kind}.border", expand=False))
        self._set_status("thinking…")

    def _result(self, ev: dict) -> None:
        self.finish()
        secs = (ev.get("duration_ms") or 0) / 1000
        foot = f"{secs:.1f}s · {ev.get('num_turns', '?')} turn(s)"
        if ev.get("is_error"):
            foot += " · error"
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
```

Three design points are worth calling out.

**Only one live region at a time.** Rich's `Status` and `Live` both repaint a region at the bottom of the terminal, and two of them fight. Every transition goes through `_stop_status()` / `_end_text()` first, and `_set_status()` refuses to start a spinner while a `Live` is up. The result is a strict alternation: spinner → tail → markdown → spinner → panel → spinner → … → footer.

**Stream a tail, print once.** A `Live` region taller than the terminal makes Rich re-emit the lines that scrolled off, duplicating output. So while text streams, the `Live` shows only the last `height − 2` lines, in plain blue, and is `transient` — it disappears when stopped. The full block is then printed exactly once as `Markdown`. The operator sees words arrive, and ends up with a properly rendered answer in the scrollback instead of a smeared one.

**Fallback without partial messages.** `_streamed_text` remembers whether a text block was already shown by the tail. If the CLI runs without `--include-partial-messages`, no `stream_event`s arrive and `_assistant()` prints the text block itself; with them, it skips it — no double output either way.

### `n06_exchange.py`

```python
def exchange(state: AgentState) -> AgentState:
    c = ct.client().containers.get(state["container_id"])
    if state.get("turn", "agent") == "agent":
        if ct.get_session(c) is None:
            gw.start_chat(c)
        last_user = next(m["content"] for m in reversed(state["transcript"]) if m["role"] == "user")
        answer = gw.send_chat_turn(c, last_user)
        return {"turn": "user", "transcript": [{"role": "agent", "content": answer}]}

    nxt = gw.ask_user("Your reply (or /quit):")
    if nxt.lower() in config.EXIT_WORDS:
        gw.close_chat(c)
        return {"done": True}
    return {"turn": "agent", "transcript": [{"role": "user", "content": nxt}]}
```

The node shape is the one chapter 02 established: two turns alternate on the self-loop, the user pass interrupts *first* so a resume never re-invokes the CLI. The only differences are `start_chat()` on the first agent pass and `close_chat()` on `/quit`.

## Why the agent doesn't forget

Chapter 02's `send_prompt()` ran `sh -c "printf %s <text> | claude -p"` through `exec_run` — a brand-new process for every turn, given only the latest message. The harness's `transcript` had the whole conversation; the model never saw more than one line of it.

Now the conversation lives in exactly one place, and it is not the graph state:

- **The CLI process holds it.** `claude -p` with `--input-format stream-json` is a session: each user message on stdin is appended to the conversation it already has, and every API call carries all of it. The `system` init event and every `result` event report the CLI's own `session_id`; across the three turns of the probe below it stayed `23a32c6d-…`.
- **The exec session outlives graph passes.** `container._sessions` is a module-level dict from container id to `ExecSession`. LangGraph's `interrupt()` pauses the graph and `MemorySaver` checkpoints the *state*, but the harness and the graph are the same Python process, so a socket sitting next to the state is simply still there when `Command(resume=…)` comes back. The socket is never part of `AgentState` — it couldn't be checkpointed anyway.
- **`transcript` is the harness's copy.** The node still appends both sides to `state["transcript"]`, but reads it only to find the last user message. It exists for the harness — and for the dashboard this chapter is heading towards — not for the model.

The probe: three turns through `start_chat()` / `send_chat_turn()` against the running `agent-agent-1` container, second turn asking for something only the first turn said.

<div class="id-col-table">

| Turn | User message | Answer |
|---|---|---|
| 1 | "My name is Balázs and my favourite number is 42. Reply with one short sentence only." | "Nice to meet you, Balázs — 42 is a great choice, and I'll keep it in mind." |
| 2 | "What is my favourite number? Reply with one short sentence only." | "Your favourite number is 42." |
| 3 | "Run `ls -la ~` with the Bash tool and tell me in one sentence how many entries there are." | "`ls -la ~` shows 9 entries in total, or 7 real ones if you exclude `.` and `..`." |

</div>

The flip side: the memory is exactly as durable as the process. If the harness exits, the exec instance's stdin closes and the CLI exits with it; `check_agent` will happily reattach to the container next run, but node 6 starts a fresh session with no history. The CLI also writes its session to disk under the container's `~/.claude/`, so a `--resume <session_id>` on `CHAT_CMD` is the obvious next step — it isn't implemented here.

## Why this shape gets prompt caching for free

Anthropic's [prompt caching](https://docs.anthropic.com/en/docs/build-with-claude/prompt-caching) is prefix caching: a request can mark breakpoints with `cache_control`, and if the exact byte sequence of system prompt, tools and messages up to a breakpoint matches a recent request, that prefix is read from the cache instead of being re-processed — at a tenth of the price and with a shorter time to first token. The catch is the word *exact*: anything that changes earlier in the prompt invalidates everything after it.

A multi-turn session in one process is the ideal client for this. Every request is the previous request plus an appended user message (or tool result); the system prompt and tool definitions at the front never change; the conversation only ever grows at the end. The Claude Code CLI sets the breakpoints itself, so the harness contributes nothing except *not restarting the process*. Chapter 02's one-process-per-turn design could only ever cache the static system prompt and tools, because there was no history to reuse.

The same probe, reading `usage` out of each `result` event:

<div class="id-col-table">

| Turn | API calls | `input_tokens` (uncached) | `cache_creation_input_tokens` | `cache_read_input_tokens` | `output_tokens` |
|---|---|---|---|---|---|
| 1 | 1 | 2 | 0 | 18,196 | 35 |
| 2 | 1 | 2 | 5,598 | 12,678 | 12 |
| 3 (tool) | 2 | 4 | 518 | 36,577 | 118 |

</div>

Read the first column against the others: across three turns and four API calls, **eight** input tokens were processed uncached. The 18k tokens of turn 1 were already cached before the session started — the cache is keyed by content, and an earlier session on the same container had the same system prompt and tools within the TTL. Turn 3's two calls (one to request the tool, one with the tool result) show the append-only pattern at work: each reads roughly 18k from cache, and the two together write only 518 new tokens — the tool call and its result. Turn 2 is the exactness rule at work in the other direction: 5.6k tokens were re-written rather than read, so part of the previous request's tail did not match byte for byte; a prefix survives only as far as it is identical, and the CLI's prompt is not entirely static between calls. `cache_creation.ephemeral_1h_input_tokens` in the same events shows the CLI is using the one-hour cache tier, which is also the window an operator has to type the next reply before the whole prefix is re-written rather than read — a cost, not a failure.

What would break it: restarting the CLI (a different `session_id` and, if the cwd or tool set changed, a different prefix), and editing the conversation in place, which this protocol cannot do — there is no event for it.

## Tool permissions

The gateway relays text and nothing else, so there is nobody on this channel to click "allow". `CHAT_CMD` therefore always carries `--permission-prompts none`: any tool call that would have prompted is denied instead, and shows up as a red `tool result · error` panel. `CHAT_PERMISSION_MODE` decides which calls get that far:

<div class="id-col-table">

| `CHAT_PERMISSION_MODE` | Effect |
|---|---|
| `auto` (default) | Claude Code's auto mode: a classifier approves safe actions, the rest are denied |
| `bypassPermissions` | no checks at all, every tool call runs — the gVisor sandbox is the only boundary |
| `acceptEdits`, `plan`, … | the other CLI modes, if you need them |
| empty string | CLI default mode: effectively chat-only, every tool call is denied |

</div>

    CHAT_PERMISSION_MODE=bypassPermissions agent-sandbox   # let the agent do anything inside the container
    CHAT_PERMISSION_MODE= agent-sandbox                    # text only

Whichever mode is set, the check runs inside the container, by a `claude` binary on a rootfs the agent can write — the chapter 02 caveat on T9 applies unchanged. What the sandbox guarantees is where a tool *can* run, not whether it *should*.

## Try it

    cd packages/agent_sandbox
    python -m venv .venv && . .venv/bin/activate
    pip install -e .
    agent-sandbox

Pick an agent name (an existing container is reattached, a new one walks through `claude auth login`), then give it a task that mixes prose and tools — *list the files in your home directory and summarise them in a markdown table* — and watch the spinner, the tail, the panels and the footer arrive in that order. Ask a follow-up that depends on the first answer; `/quit` ends the session. Colours need a real TTY; piped through `cat` the renderer falls back to plain text.

## What this leaves open

Against the chapter 02 inventory, this post moves nothing on the boundary and a little on the channel:

- **T1e — terminal query → stdin.** Still open, and worth being precise about: Rich does *not* sanitise escape sequences. A `\x1b]52;…\x07` (OSC 52 clipboard write) or `\x1b[6n` (cursor position report) inside the model's markdown reaches the terminal, and the terminal's reply lands on the `input()` that follows. The `_clean` regex from the login relay is not applied to answers. That filter belongs at IP-1 and is the next thing to add to `render.py`.
- **T7e — unbounded read.** `readline()` grows `_stdout` until a newline arrives; a CLI that emits a very long line without one grows it without limit. `CHAT_TURN_TIMEOUT` does bound the *time* a turn may take — an improvement on chapter 02, where `ct.exec()` never passed its `timeout` — but after a timeout the session is left open with unread events queued, and the next turn will read them first.
- **Observability.** The `system`, `rate_limit_event` and `result.usage` events are dropped on the floor. They are precisely the data an operator dashboard wants, and the renderer's `feed()` is precisely where a second consumer would attach. That is where this chapter goes next.
