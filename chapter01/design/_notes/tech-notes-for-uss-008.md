# Tech notes — USS-008

> Source story: [`01_user_story/uss-008_create_agentic_graph_4_process.md`](01_user_story/uss-008_create_agentic_graph_4_process.md)
> Status: draft 1.
> Scope: constraints derived from the story plus a read-only review of two prototypes (2026-11-10). Not a design; the story stays authoritative.

## 1. Runtime and platform constraints

| Constraint | Value |
|---|---|
| Container | gVisor sandbox `agent-<agentID>` (USS-002) |
| User | `agent`, uid 1001 |
| Entry | `enter_agent <agentID> -- agent_graph run [--process <name>] [--resume <runID>]` (the `-- <cmd>` form is an open item) |
| Package / venv | Python package `agent_graph` in `/opt/venv` (USS-003) |
| Orchestration | LangGraph |
| LLM backend | `claude` CLI, headless, agent's own subscription login; no model broker, no API SDK |
| Network | Egress only to the Claude endpoint (USS-004) |
| Writable paths | `/workspace` and `/home/agent/.claude` only |
| Read-only by convention | `.process/`, `.input/`, `identity/` (enforcement is an open item) |
| Concurrency | One active run per agent entity; a second run exits 1 `NOT_STARTED` |
| Persistence | Workspace lives on the host (`Workspace-Agents/<agentID>`), survives container removal (USS-006); resume works after container recreate |
| User channel | One terminal, attached by `enter_agent` |

## 2. Architecture constraints

- Static master graph (S1-S8, C1) is identical for every agent and process; only the dynamic graph differs.
- Dynamic slave graph is compiled per process (S5) as its own LangGraph graph with its own checkpoint thread.
- S6 drives it by streaming with `Command(resume=...)`. The dynamic graph pauses with `interrupt()` and reports through the stream.
- Dynamic graph never does terminal I/O and never ends the run itself.
- S1 is the sole terminal reader/writer. All questions, approvals, progress lines and notices go through it.
- Dynamic graph changes state only on a command from S6; every transition is reported as a status.
- Full command/status history is persisted in `.agent/<runID>/`.

| Command (static to dynamic) | Status (dynamic to static) |
|---|---|
| `START` | `NODE_STARTED(node)` |
| `PAUSE` / `RESUME` | `NODE_DONE(node, products, summary)` |
| `ANSWER(payload)` | `NODE_READY(node, needs)` |
| `CONTEXT(node, ref)` | `NEED_INPUT(node, question)` |
| `RETRY(node)` | `NODE_FAILED(node, reason, retryable)` |
| `SKIP(node)` (only `optional`) | `COMPLETED(output_ref)` |
| `ABORT(reason)` | `ABORTED` |

`NODE_READY` is emitted before every cognitive node; the node waits for `CONTEXT` (C1 runs in between).

## 3. Process definition constraints (S4/S5)

| Rule | On violation |
|---|---|
| `apiVersion: agent-graph/v0` (known schema version only) | `PROCESS_INVALID` |
| Node kind in `exec`, `cognitive`, `checkpoint`, `gate`, `communicate`, `memory` | `PROCESS_INVALID` |
| `entry` and `finish` defined | `PROCESS_INVALID` |
| Cycles only if marked as loop | `PROCESS_INVALID` |
| Unresolved reference | `PROCESS_INVALID`, findings with line numbers |
| `consumes` = `<nodeID>.<productID>` of a node that always runs earlier | `PROCESS_INVALID` |
| Declared skills exist in `identity/.skills/` with valid `SKILL.md` | `PROCESS_INVALID` |
| Declared snippets exist in `identity/.snippet/` with valid front matter | `PROCESS_INVALID` |
| `exclude_skills` names neither a skill the same node declares nor an unknown skill | `PROCESS_INVALID` |
| Snippet pool has at least one `layer: agent` snippet | `PROCESS_INVALID` |
| Input matches process `inputs` contract (S3) | `INPUT_INVALID` |

- Invalid process or input is rejected before any dynamic node runs; exit 3.
- S5 writes the resolved graph to `.agent/<runID>/graph.json`. Compile failure: exit 2 `BUILD_FAILED`, partial graph kept.
- No process-level skill exclusion; only per node.

## 4. Cognitive node constraints

- One `claude -p --output-format stream-json` session per node, started fresh at node entry.
- Own cwd: `.agent/<runID>/nodes/<nodeID>/` (generated `CLAUDE.md`, `.claude/skills/`, `context.md`, `context.json`, `transcript.jsonl`, session ID).
- Session ID is captured from the stream and stored in the run checkpoint.
- Retries and developer answers continue the same session via `--resume <sessionID>`; a retry gets the failure reason as its next turn.
- `agent_graph run --resume <runID>` continues the same session.
- No conversation history crosses node boundaries; C1 decides what crosses.
- `~/.claude/CLAUDE.md` holds only L0 or is empty; `~/.claude/skills/` is empty.
- The runtime places no `CLAUDE.md` in `/workspace` or `/workspace/.agent`.
- Output is validated against the node's declared schema before the next node runs.
- A handoff note (done, decisions, open points) is required besides the output.
- Claude Code compacts inside the same session near the node budget; each compaction is recorded in `context.json`.

## 5. Context and skill management constraints (C1)

CLAUDE.md layers (snippets from the pool plus generated parts, each layer size-capped):

| Layer | Source |
|---|---|
| L0 agent | All `layer: agent` snippets |
| L1 process | Process `snippets:` |
| L2 step | Node `snippets:`, `auto: true` step snippets matching the node goal, generated step contract |
| L3 run | Generated from run state (developer answers, decisions) |

- Over the cap: drop auto-picked snippets first, then shorten the lowest layer.
- Order inside a layer is declaration order; auto-picked snippets last.
- A node `CLAUDE.md` contains only pool snippets plus generated step contract and run facts.

Skill selection order:

1. Declared skills (`skills:`), always staged.
2. Remove `exclude_skills` (never staged, never ranked).
3. Rank remaining pool skills by name, description, tags vs node goal.
4. Cap at `max_skills` (node, else process, else default 3). Unstaged skills are invisible to the node.

Context pack `context.md`, filled in this order within `context.budget`:

1. Node goal and output schema.
2. Developer answers and decisions that apply.
3. Consumed work products (`consumes:`) and `context.reads`.
4. Handoff notes of direct upstream nodes.
5. Matching run memory entries.

Rules:
- Pointer over payload: leftovers are listed by path so Claude can `Read` them.
- Summarize, do not truncate: over-share items become a summary (cognitive, cached per file hash) or, for structured data, schema plus sample.
- `context.json` records `CLAUDE.md` layers with snippet IDs, staged and excluded skills, consumed products, context items with token counts, left-out items and reason, compactions, actual token usage.
- Mandatory context (goal, schema, declared reads) over budget after summarizing: exit 2 `CONTEXT_OVER_BUDGET`, node not started.

## 6. State, persistence and resume

- Graph state holds references (paths, short summaries, structured results), not payloads.
- Work products live in `.agent/<runID>/work/<nodeID>/`. Path is given in the L2 step contract (cognitive) or passed as argument/env var (`exec`).
- Before `NODE_DONE` the runtime checks every declared product: exists and matches schema. Otherwise `NODE_FAILED` (retryable, reason goes into the node's next turn).
- A node sees only the products it consumes, not the whole `work/`.
- `finish` node products matching process `outputs` are copied to `.output/<runID>/` and checked by S7.
- Resume: continue from the last checkpoint; skip nodes whose products are complete; never repeat completed `exec` nodes.
- Checkpointer must be durable (file or SQLite). `MemorySaver` does not satisfy resume.
- Ctrl-C or container stop leaves the checkpoint in `.agent/<runID>/` (exit state `SUSPENDED`).

## 7. Budgets and failure mapping

Budgets: `max_steps`, `max_llm_calls`, `timeout`, per-node token budget (`context.budget`). S6 enforces them; exceeding one aborts the dynamic graph with a reported reason, `BUDGET_EXCEEDED`.

| Exit | Exit state(s) | Notes |
|---|---|---|
| 0 | `COMPLETED` | Outputs in `.output/<runID>/` |
| 1 | `NOT_STARTED` | No/ambiguous process file, another run active; nothing written |
| 3 | `INTENT_REJECTED`, `PROCESS_INVALID`, `INPUT_INVALID` | Before any dynamic node runs |
| 2 | `BUILD_FAILED` | Partial graph kept |
| 2 | `NODE_FAILED`, `ABORTED` | Checkpoint kept, resumable |
| 2 | `BUDGET_EXCEEDED` | Checkpoint kept |
| 2 | `LLM_UNAVAILABLE` | Not logged in or endpoint unreachable; suspended at the node, resumable |
| 2 | `OUTPUT_INVALID` | Output kept in `.output/<runID>/` marked invalid |
| 2 | `CONTEXT_OVER_BUDGET` | Node not started |
| n/a | `SUSPENDED` | Terminal closed or container stopped; resume with `--resume` |

No rollback; outputs of failed runs are kept.

## 8. Dependency constraints

| Package | Status | Version / note |
|---|---|---|
| langgraph | pinned in both prototype locks | 1.2.18 |
| langgraph-checkpoint | pinned | 4.2.0 |
| langgraph-prebuilt | pinned | 1.1.0 |
| rich | pinned in both prototype locks; adopted by agent-graph | 15.0.0 |
| markdown-it-py | adopted by agent-graph (rich dependency) | 4.2.0 |
| Pygments | adopted by agent-graph (rich dependency) | 2.21.0 |
| mdurl | adopted by agent-graph (markdown-it-py dependency) | 0.1.2 |
| PyYAML | pinned | 6.0.3 |
| pydantic | pinned | 2.13.5 |
| `langgraph-checkpoint-sqlite` | missing, needed | Or a file-based saver as fallback |
| JSON Schema validator (e.g. `jsonschema`) | missing, needed | S3, S4, S7, work-product and node output checks |
| Claude Agent SDK | missing, only if chosen over the CLI | Open item in the story |

Image gap: the existing `Dockerfile.agent` (earlier prototype) is node:22-slim with the claude CLI only. It has no Python, no `/opt/venv`, and does not pin uid 1001.

## 9. Lessons from the prototypes (do / don't)

- Prototype A: an earlier private LangGraph prototype (YAML to `StateGraph`).
- Prototype B: an earlier private prototype (host-side Rich gateway).

| Reuse | Source |
|---|---|
| Dispatch dict + build loop as S5 seed | A `flow_engine.py:63-90` |
| argv-list subprocess, no `shell=True` | A |
| Typed-state reducer pattern | A `myTutorials/state.py` |
| Control-sequence neutralising (shown as visible markers) | B `services/sanitize.py` (for S1); ported (control and bidi characters become visible markers) |
| Rich rendering of stream-json | B `services/render.py` (for S1); ported |
| NDJSON reader with line cap | B `services/container.py:98-201` |
| Event schema guard | B `services/gateway.py:114-156` |
| interrupt to `Command(resume)` driver loop, `read_reply` flush/recheck | B `main.py:11-50` |

| Avoid | Why |
|---|---|
| `input()` / `print()` inside nodes (A); direct Rich writes outside the gateway (B) | Violates S1-only terminal I/O |
| Untyped `StateGraph(dict)` | Use typed state |
| `cwd="."` | Not workspace-bounded; use the node directory |
| `--output-format text` | Loses the session ID |
| Unchecked CLI returncode, stderr used as the answer | Must map to `LLM_UNAVAILABLE` |
| Unrestricted dotted-path import from YAML | Needs an allowlist |
| `MemorySaver` | No resume |
| One chat session per container; session silently dropped on timeout or line overflow (B `gateway.py:175,192`) | Conflicts with one session per node and history kept |
| Unguarded write to a dead CLI process (B `gateway.py:166-167`) | Check liveness, map to failure |
| Package or folder named `langgraph` (A `run_yaml_flow.py:8`) | Shadows the library |

## 10. Items to verify before implementation

| Item | Check |
|---|---|
| claude CLI flags | In the image's CLI version: `--resume`, `--setting-sources`, `--add-dir`, `--permission-prompts`; the stream-json field that carries the session ID |
| Instruction leakage | How to stop `CLAUDE.md` and skill loading from parent dirs and user level (story open item: Claude Code loading rules) |
| LangGraph 1.2.18 semantics | Separately compiled dynamic graph with its own thread: `interrupt()` and `Command(resume)` driven from the master, subgraph vs independent graph |
| Token counting | Local estimate vs count API, for `context.budget` and layer caps |

Other open points (checkpoint store, backend choice, `enter_agent` command form, memory scope, skill ranking, schema format) are tracked in the story's "Open items"; not repeated here.
