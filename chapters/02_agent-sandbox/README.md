# Agent sandbox

Code for the *Agent sandbox* chapter. Posts live in `posts/`, the code in
`src/agent_sandbox/`. Each post links to the git tag
`agent-sandbox/post-NN-<slug>` that matches the code it describes.

    docker build -t claude-agent:latest -f Dockerfile.agent .
    python -m venv .venv && . .venv/bin/activate
    pip install -e .
    agent-sandbox

Posts can embed interactive diagrams ("islands") as small Preact components. Source
lives per post at `posts/<slug>/islands/src/*.entry.jsx`; `npm run build:islands`
bundles each into a self-contained `posts/<slug>/islands/dist/<name>.js` (Preact
included, no external requests) that the publisher inlines into the Ghost post next
to a `<div id="...">` mount point in the post's markdown.

    npm install
    npm run build:islands

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
