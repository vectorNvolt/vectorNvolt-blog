# agent_sandbox

A LangGraph harness that drives the Claude CLI inside a gVisor-sandboxed container.
Used by the *Agent sandbox* chapter (`chapters/02_agent-sandbox/`); a post's front
matter (`code_tag` + `code_package`) pins the exact commit of this package it describes.

    docker build -t claude-agent:latest -f Dockerfile.agent .
    python -m venv .venv && . .venv/bin/activate
    pip install -e .
    agent-sandbox

| #  | node               | kind | purpose                                                        |
|----|--------------------|------|-----------------------------------------------------------------|
| 1  | `select_agent`     |      | ask the user which agent to work with                           |
| 2  | `check_agent`      |      | does container `agent-<name>` exist? → 3 if not, else 4          |
| 3  | `create_container` |      | `docker run --runtime=runsc` from `Dockerfile.agent`             |
| 4  | `auth_gateway`     | loop | relay `claude auth login` ⇄ user until `claude auth status` ok  |
| 5  | `ask_task`         |      | ask the user what they want to do                                |
| 6  | `exchange`         | loop | `claude -p` ⇄ user until `/quit`                                 |

User I/O goes through `src/agent_sandbox/services/gateway.py` only (`interrupt()` under
the hood), so the transport can be swapped (CLI → chat/webhook) without touching nodes.

Other packages can depend on this one (path dependency during development, a
`packages/agent_sandbox` git-tag reference once pinned) to build on top of it.
