# agent-graph (`agent_graph`)

Python package that runs a developer-defined process (a YAML file in `/workspace/.process/`) as a supervised agent graph inside the agent container. One static state machine (S1-S8) is the same for every process; it compiles the process into a dynamic LangGraph machine and runs it under an execution engine (E1-E7). Claude is reached only through the `claude` CLI. Spec: `design/01_user_story/uss-008_create_agentic_graph_4_process.md`, `design/02_use_case/usc-008_run_agent_graph_process.yml`, `design/03_sys_req/agent-graph/`.

## States

Keep in sync with `src/agent_graph/nodes/nNN_*.py` and `src/agent_graph/graph.py`.

| State | File | Next | Purpose |
|---|---|---|---|
| S1 start | `n01_start.py` | S2, S3 or S6 (resume), S7 | Active-run lock, start preconditions (SFR-AGG-040), process listing. `--resume` loads `run.json`. |
| S2 clarify | `n02_clarify.py` | S3, S7, S8 | A Claude instance matches the developer's intent to one process and asks back; creates the runID and the run folder. Quit or no match: `INTENT_REJECTED`. |
| S3 parse_process | `n03_parse_process.py` | S4, S7 | Validate the process, skills and snippets; write `index.json`. |
| S4 bind_input | `n04_bind_input.py` | S5, S7 | Bind input files (the developer chooses between several candidate sets), ask for `note` sources, check input; write `binding.json`. |
| S5 build_graph | `n05_build_graph.py` | S6, S7, S8 | Compile the dynamic machine, write `graph.json`, show the outline, wait for the start confirmation. |
| S6 execution | `n06_execution.py` | S7, S8 | Thin wrapper around `engine.run_engine` (E1-E7). |
| S7 resolution | `n07_resolution.py` | S1-S6, S8 | Every error ends here. A Claude instance (Read only) explains and proposes fixes; each change is applied only after the developer approves it and is recorded in `resolution/<n>/`. Re-entry follows the hash comparison (`process/reentry.py`). |
| S8 report | `n08_report.py` | END | Run summary on the terminal and in `.output/<runID>/report.md`; exit state and exit code. |

The run ends only on completion, on the developer's explicit quit (S2 or S7), or by suspension (Ctrl-C, SIGTERM, closed terminal: exit status 130, `--resume` continues).

| Exit state | Exit code |
|---|---|
| `COMPLETED` | 0 |
| `NOT_STARTED` (usage error, precondition on quit) | 1 |
| `BUILD_FAILED`, `CONTEXT_OVER_BUDGET`, `NODE_FAILED`, `BUDGET_EXCEEDED`, `LLM_UNAVAILABLE`, `OUTPUT_INVALID` | 2 |
| `INTENT_REJECTED`, `PROCESS_INVALID`, `INPUT_INVALID` | 3 |
| `SUSPENDED` | 130 |

## How to run

Inside the agent container, started from the host through `enter_agent` (as uid 1001, with the terminal attached):

```
sudo enter_agent <agentID> -- agent_graph run
sudo enter_agent <agentID> -- agent_graph run --resume <runID>
```

No process is named on the command line: it is agreed in the dialog. `--resume <runID>` is the only option; an unknown argument or a run without a checkpoint is a usage error (stderr, exit 1, nothing created). `python -m agent_graph run` is equivalent. The agent image installs the package (`packages/agent-image/Dockerfile.agent` copies `src/agent_graph` to `/opt/agent_graph`, adds a `.pth` file and an `agent_graph` wrapper in `/usr/local/bin`).

Workspace layout (all under `/workspace`):

```
.process/            process files (apiVersion: agent-graph/v1)
.input/              input data; only the bound files are read
identity/.skills/    skill pool        identity/.snippet/   snippet pool (at least one layer: agent)
.agent/<runID>/      run.json binding.json index.json graph.json history.jsonl run_facts.jsonl
                     gateway.jsonl checkpoint.sqlite overrides.json nodes/ work/ dialogs/ resolution/ tmp/
.output/<runID>/     the output targets and report.md
```

`examples/` holds a small complete workspace (one process with a procedure, a cognitive and a human step). Copy its folders into `/workspace` and run `agent_graph run`.

Environment: `AGENT_GRAPH_WORKSPACE` (default `/workspace`), `CLAUDE_CONFIG_DIR`, `AGENT_GRAPH_CLAUDE_CMD` (default `claude`), `AGENT_GRAPH_MODEL`, `AGENT_GRAPH_EFFORT`. Model and effort are pinned in `run.json` at S2, so a resumed run uses the same values.

## Resolution (S7)

* The Claude instance has the Read tool only. It answers with JSON (`schemas/dialog_reply.schema.yml`); a proposal is `edit_file`, `override` or `choose_process`.
* The runtime shows each proposal and asks y/n. Only `.process/`, `.input/`, `identity/` and the run's `overrides.json` can change; every change is recorded in `resolution/<n>/` (`changes.jsonl`, `before/`, `after/`, `hashes.json`).
* Before a run folder exists (errors of S1 and S2), and when Claude is unavailable, S7 is a text-only menu (check again, set an override, quit).
* Re-entry: nothing changed or only overrides: the failed state; process or pool changed: S3; input or binding changed: S4; another process: S2 with a new runID (the old run is kept). S6 restarts at the earliest changed node, else at the node that failed, with the resolution summary as a run fact.
* `overrides.json`: `{"retry": n | {node: n}, "timeout": s | {node: s}, "budgets": {"max_steps", "max_llm_calls", "timeout"}, "step_budget": {node: tokens}}` (read by `engine/budgets.py`).

## Terminal output

The gateway owns one `rich` Console (`rich==15.0.0`, with `markdown-it-py`, `Pygments`, `mdurl`; pinned in `pyproject.toml` and `packages/agent-image/requirements.*`).

* `[source]` tags, prompts and notices are styled; notices by code (`WARNING` warn, `LLM_UNAVAILABLE` and `RESOLUTION_LIMIT` error, `PROGRESS` dim).
* Claude text is sanitised (`services/sanitize.py`: control and bidi characters become visible markers) and rendered as Markdown. `gateway.jsonl` records the sanitised text with its markers.
* `Gateway.turn(source, show_text=...)` streams claude stream-json events through `TurnRenderer`: spinner, tool-call panels (input clipped to 1500 characters), tool-result panels (clipped to 1200, red on error) and a footer with duration and turns. The S2 and S7 dialogs use `show_text=False` because the reply is JSON; its `say` text is printed once by `say()`.
* `claude` gets `--include-partial-messages` only when an event sink is attached. The flag is not part of `Launch.invariant_argv`, so `launch_hash` is unchanged.
* Off a TTY the output is plain `[source] line` with no ANSI.
* The agent image sets `ENV COLORTERM=truecolor` (`packages/agent-image/Dockerfile.agent`): `docker exec -t` sets only `TERM=xterm`, so `rich` would fall back to 8 colours and most of the hex palette would show as white or grey. The supported terminal (Windows Terminal) is truecolor; with another one, override per call (`enter_agent <id> -- env COLORTERM= agent_graph run`), or use `NO_COLOR=1` for no colour.

## Tests

```
pip install -e '.[test]'
pytest
```

The tests use `tests/fakes/fake_claude.py` instead of the real CLI; none touches the network. The end-to-end tests of `tests/wp6_static/` need the execution engine (`agent_graph.engine`), which is present in this package.

## Layout

| Here | Role |
|---|---|
| `src/agent_graph/main.py`, `__main__.py` | command line, signals, exit status |
| `src/agent_graph/graph.py`, `nodes/` | static master S1-S8 |
| `src/agent_graph/process/`, `compose/` | process front-end, context composition |
| `src/agent_graph/dynamic/`, `engine/` | dynamic machine, execution engine |
| `src/agent_graph/services/` | gateway (the only terminal owner; `rich` rendering in `render.py`, `sanitize.py`), Claude CLI, checkpoint, lock, report, resolution store |
| `src/agent_graph/prompts/`, `schemas/` | prompt texts, JSON/YAML schemas |
| `examples/` | example workspace |
| `tests/` | per-work-package tests with fakes |

## Open points

- Writes the `claude` CLI itself makes to `/tmp` (a tmpfs) are outside the runtime's control; all runtime and child-process temporary files use `.agent/<runID>/tmp` (`TMPDIR`).
- The S2, S4 and S7 dialogs use the Read tool without a path restriction inside the container; they cannot write.
- S4 asks the `note` questions directly from the process text (no Claude instance).
- Exit code 130 for SUSPENDED is proposed (USS-008 leaves it open).
