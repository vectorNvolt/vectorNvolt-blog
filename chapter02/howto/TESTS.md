# Tests: boost converter demo runbook

## Tests

Where: VM, repo root. Install `uv` once:

```bash
curl -LsSf https://astral.sh/uv/install.sh | sh
source $HOME/.local/bin/env              # or open a new shell
```

```bash
export PATH="$PATH:/usr/local/go/bin"
(cd packages/agent-exec && CGO_ENABLED=0 go build ./cmd/... && go vet ./... && go test ./...)
(cd packages/agent-image && go test ./...)
uv venv --python 3.14 ~/.venvs/agent-graph
source ~/.venvs/agent-graph/bin/activate
uv pip install langgraph==1.2.18 pyyaml==6.0.3 pytest pytest-timeout
uv pip install -e packages/agent-graph --no-deps
env -u AGENT_GRAPH_PROC_EXES python -m pytest packages/agent-graph/tests -q --timeout=60
```

Check: all commands exit 0.
