# agent_sandbox

A LangGraph harness that drives the Claude CLI inside a gVisor-sandboxed container.
Used by the *Agent sandbox* chapter (`chapters/02_agent-sandbox/`); a post's front
matter (`code_tag` + `code_package`) pins the exact commit of this package it describes.

    docker build -t claude-agent:latest -f Dockerfile.agent .
    python -m venv .venv && . .venv/bin/activate
    pip install -r requirements-lock.txt && pip install -e . --no-deps
    agent-sandbox

    pip install -e .[dev] && pytest        # sanitiser, schema, line cap, reply check

| #  | node               | kind | purpose                                                        |
|----|--------------------|------|-----------------------------------------------------------------|
| 1  | `select_agent`     |      | ask the user which agent to work with                           |
| 2  | `check_agent`      |      | does container `agent-<name>` exist? → 3 if not, else 4          |
| 3  | `create_container` |      | `docker run --runtime=runsc` from `Dockerfile.agent`             |
| 4  | `auth_gateway`     | loop | relay `claude auth login` ⇄ user until `claude auth status` ok  |
| 5  | `ask_task`         |      | ask the user what they want to do                                |
| 6  | `exchange`         | loop | persistent `claude` stream-json session ⇄ user until `/quit`     |

User I/O goes through `src/agent_sandbox/services/gateway.py` only (`interrupt()` under
the hood), so the transport can be swapped (CLI → chat/webhook) without touching nodes.

## Rich rendering (node 6)

The exchange loop keeps one `claude -p --input-format stream-json --output-format stream-json`
process alive per container (non-TTY exec session, Docker frames demuxed in
`services/container.py`), so the CLI holds the conversation itself. Each turn's NDJSON
events are fed to `services/render.py::TurnRenderer`, which shows a spinner while the
model thinks, a live plain-text tail while text streams, the finished answer as themed
markdown, tool calls / tool results as panels, and a `duration · turns` footer.

Colours come from `PALETTE` / `THEME` in `services/render.py`; edit them there.

Everything the CLI sends is untrusted output of a process on a writable rootfs, so the
renderer is fenced (`chapters/03_agent-dashboard/posts/02-improve-sec-rich-rendering-if`):
`services/sanitize.py` turns every control sequence, C0/C1 byte and bidi/zero-width
character into a visible marker (`␛[2J`, `⟨U+202E⟩`) before Rich sees it; tool names are
markup-escaped; links render with their URL; text blocks and tool panels are bounded
(head *and* tail kept); the gateway drops events that do not match the stream-json
schema and closes the session on an over-long line or a timeout; `main.py` flushes the
tty input queue before every `input()` and refuses a reply that carries control bytes.

| env                    | default | purpose                                                       |
|------------------------|---------|---------------------------------------------------------------|
| `CHAT_PERMISSION_MODE` | `auto`  | `claude --permission-mode` for the session, see below         |
| `CHAT_TURN_TIMEOUT`    | `600`   | seconds to wait for a turn's `result` event                   |
| `CHAT_CLOSE_TIMEOUT`   | `10`    | seconds to wait for the CLI to exit after `/quit`             |
| `CHAT_MAX_LINE`        | `1048576` | bytes per NDJSON line from the CLI; over it the session is reset |
| `CHAT_MAX_TEXT_BLOCK`  | `20000` | chars of one text block that are rendered (head + tail)       |

### Tool permissions

The gateway only relays text, so there is nobody to answer a permission prompt. The
session therefore always runs with `--permission-prompts none`: any tool call that
*would* prompt is denied instead of blocking, and shows up as a red `tool result · error`
panel. `CHAT_PERMISSION_MODE` decides which calls get that far:

| value               | effect                                                                  |
|---------------------|-------------------------------------------------------------------------|
| `auto` (default)    | Claude Code's auto mode: a classifier approves safe actions, the rest are denied |
| `bypassPermissions` | no checks at all, every tool call runs — the gVisor sandbox is the only boundary |
| `acceptEdits`, `plan`, … | the other CLI modes, if you need them                              |
| empty string        | CLI default mode: effectively chat-only, all tool calls denied          |

    CHAT_PERMISSION_MODE=bypassPermissions agent-sandbox   # let the agent do anything inside the container
    CHAT_PERMISSION_MODE= agent-sandbox                    # text only

Other packages can depend on this one (path dependency during development, a
`packages/agent_sandbox` git-tag reference once pinned) to build on top of it.
