---
title: "Stop Letting AI Agents Improvise Business Logic: Enforcing Deterministic Flows in LangGraph #ch02p01"
type: dev
code_tag: ch01-ch02/publication
code_package: agent-graph
code_url: https://github.com/vectorNvolt/vectorNvolt-blog/tree/ch01-ch02/publication/chapter02/packages/agent-graph
slug: exec-deterministic-flows
status: published
tags: [exec, langgraph, determinism, agents, python]
custom_excerpt: "I argue for keeping the order of business steps in a developer-defined process file enforced by LangGraph, and I state what each design choice in the agent-graph package costs me. The model decides only inside a step, and the post names what it still decides and what a reviewer must still read."
feature_image:
links:
  disposable-exec: /exec-disposable-containers/
  howto: https://github.com/vectorNvolt/vectorNvolt-blog/blob/ch01-ch02/publication/chapter02/howto/README.md
---

<!--kg-card-begin: html-->
<style>
details.sec-more {
  margin: 0.8em 0 1.6em;
}
details.sec-more summary {
  display: inline-block;
  cursor: pointer;
  list-style: none;
  padding: 0.35em 0.95em;
  font-size: 0.85em;
  font-weight: 600;
  color: #2e86c1;
  border: 1px solid color-mix(in srgb, #2e86c1 40%, transparent);
  border-radius: 999px;
  background: color-mix(in srgb, #2e86c1 8%, transparent);
  transition: background 0.15s ease;
}
details.sec-more summary:hover {
  background: color-mix(in srgb, #2e86c1 18%, transparent);
}
details.sec-more summary::-webkit-details-marker {
  display: none;
}
details.sec-more summary::before {
  content: "▸";
  display: inline-block;
  margin-right: 0.5em;
  transition: transform 0.15s ease;
}
details.sec-more[open] summary::before {
  transform: rotate(90deg);
}
details.sec-more[open] .sec-more-show,
details.sec-more:not([open]) .sec-more-hide {
  display: none;
}
/* One line per section: add every new #more-<id> here. */
#more-arch:not([open]) ~ :not(h2, p:has(> a[id]:empty:only-child), #more-arch ~ h2 ~ *),
#more-model:not([open]) ~ :not(h2, p:has(> a[id]:empty:only-child), #more-model ~ h2 ~ *),
#more-gates:not([open]) ~ :not(h2, p:has(> a[id]:empty:only-child), #more-gates ~ h2 ~ *),
#more-code:not([open]) ~ :not(h2, p:has(> a[id]:empty:only-child), #more-code ~ h2 ~ *),
#more-refs:not([open]) ~ :not(h2, p:has(> a[id]:empty:only-child), #more-refs ~ h2 ~ *) {
  display: none;
}

/* ---- Inline code --------------------------------------------------------
   The theme's light chip stays light in dark mode. A tint of the text colour
   works in both modes; !important because the theme's selector is unknown. */
:not(pre) > code {
  background: color-mix(in srgb, currentColor 10%, transparent) !important;
}
</style>
<!--kg-card-end: html-->


## Why I Keep Business Logic Out of the LLM

Many AI agent demos fail in production since teams let the language model decide which engineering process step happens next and which tool to be used. When a workflow involves money, compliance, or human approvals, probabilistic routing is a liability.

**The risk.** When a engineering process runs as a standard AI agent, the usual design gives the language model a goal and a set of tools and lets it choose its own path. For any workflow with an approval, a compliance check, or a hand-off between roles, that means the model chooses the order of the steps—and may skip one, repeat one, or invent one nobody authorized. I treat this as a systems design risk, not a model-quality problem, because a better model changes the odds, not the guarantee.

**What it costs:**
- **Repeatability:** The same input can follow a different execution path on another run.
- **Cost:** A model that controls its own loop can keep running until an external hard limit cuts it off.
- **Liability:** When a required step is skipped, the logs show a plausible conversation rather than a broken rule.

**My alternative approach:** Write the process once, as a declarative file a business owner can read: which steps exist, who or what performs each one, and which step may follow which. A runtime compiles that file into LangGraph graph that enforces the process programmatically. The model still does what it is good at, but the boundary is the specific process step, and it must report an outcome from a predefined list the file allows. The runtime—not the model—picks the next step, tracks the budget, and refuses any transition the file does not permit.

**What this promises:** Fixed execution order, bounded cost, and an auditable record per run. It does not guarantee perfect prose—the model can still write a weak summary inside its assigned step—but it guarantees the workflow cannot go off-rails. The rest of this post shows how my `agent-graph` package builds this on LangGraph, and where it stops.


## What deterministic means here

**My definition:** I do not mean the model returns identical text twice. I mean the order of steps, the allowed outcomes, the budgets, and the retry rules come from a process file and are enforced outside the model. Tools are a separate layer: the runtime passes them to the CLI as flags. Inside a step, the model decides; everywhere else, the graph decides.

**A concrete example:** Consider `examples/.process/summarize-notes.yml`. It chains three step types:
1. A **procedure** step (`extract`) that runs a deterministic script.
2. A **cognitive** step (`write-summary`) that asks the model for a templated summary.
3. A **human** step (`review`) where `hil_ok` ends the run and `hil_nok` loops back to `write-summary`.

Procedure and human steps invoke no model at all. A cognitive step is the only place inside a process where an LLM runs. Each edge carries one condition: `done` for a step that declares no outcomes, `hil_ok` or `hil_nok` for a human step. The grammar also accepts `outcome == <value>` for a cognitive step that declares `outcomes`, and this example does not use it.

```yaml
# Abridged from examples/.process/summarize-notes.yml: steps shortened, transitions verbatim
steps:
  - id: 1
    name: extract
    type: procedure
    # reads, produces, instruction omitted
    run: "python .process/scripts/count_lines.py {input.Source} {work}"

  - id: 2
    name: write-summary
    type: cognitive
    # reads, produces, instruction, budget omitted

  - id: 3
    name: review
    type: human
    # reads, instruction omitted

transitions:
  entry: extract
  edges:
    - {from: extract, to: write-summary, when: done}
    - {from: write-summary, to: review, when: done}
    - {from: review, to: END, when: hil_ok}
    - {from: review, to: write-summary, when: hil_nok}
```

## Why a process file and not an agent loop

**The problem:** I need an execution order I can inspect and prove *before* a run starts, not reconstruct from logs afterward.

**The alternatives:** I weighed two options by architectural reasoning rather than benchmarks:
- **A standard LangGraph agent loop**, where the model picks the next node, requires the least authoring effort—but the execution order only exists in hindsight.
- **A hand-written Python graph per workflow** fixes the execution order, but every workflow ends up carrying its own supervision, retry, and budget logic, and a business owner cannot read it.

**My choice:** A YAML process file treated strictly as data. The design rule for the package is that a process definition must never execute code of its own, and every process runs under the exact same supervising state machine.


**What it costs:** Upfront authoring effort and rigidity: a process cannot improvise around an edge case nobody wrote down. When an unexpected failure occurs, the package responds with `S7` human-in-the-loop resolution—where a developer approves each fix—rather than model improvisation.


**Why I accept it:** For approvals and compliance checks, a run that halts and asks a person is a much safer failure mode than one that continues down an unapproved path.

## The architecture: a 3-tier split across macro and micro control

Supervision must not vary by workflow, or a process author could weaken safety controls—yet inside an individual cognitive step, the model still needs room to reason and use tools to complete its task. `agent-graph` separates deterministic orchestration from model reasoning using a **3-tier architecture** split into two **macro** and one **micro** layer.

- **Tier 1 — Macro / Static (Supervisor graph, `S1`–`S8` in `graph.py`):** Fixed code that is identical for every process (`test_master_graph_is_the_same_for_every_process`). Its `TRANSITIONS` table defines which supervisor state may follow which, and `_route` raises a `RuntimeError` for anything else.
- **Tier 2 — Macro / Dynamic (Compiled process graph + Engine `E1`–`E7`):** Built at runtime in state `S5`, compiling one node per step and edges derived strictly from the YAML process file (`test_compile_adds_no_node_the_process_does_not_define`). The engine within `S6` drives this graph through eight commands and six statuses, and the state machine never terminates a run on its own (`DynamicMachine`).
- **Tier 3 — Micro (Claude agent internal flow):** Isolated inside each cognitive step. Within this tier, Claude runs its own internal reasoning and tool-use loop to fulfill the step's goal and write `result.json`, with zero visibility into—or authority over—the macro graphs above it. 
Each cognitive step runs as an isolated `claude -p` invocation. To give Claude Code's automatic prompt caching the best chance at prefix reuse, `agent-graph` keeps the opening structure of every call identical—using the same launch flags, working directory, and a system.md scoped only to the process and worker pool—while pushing step-specific `CLAUDE.md`, context, and prompt inputs later in the sequence. The repository does not set explicit cache markers or TTLs, and benchmarking the actual cache hit rate is planned for a follow-up post.

Click the boxes below for details: **green** is the static master graph, **red** is the cognitive step where the micro-level Claude loop runs, and **blue** belongs to a separate package, `agent-exec`.

{{island:det-arch}}

<!--kg-card-begin: html-->
<details class="sec-more" id="more-arch"><summary><span class="sec-more-show">Show the full section</span><span class="sec-more-hide">Hide the section</span></summary></details>
<!--kg-card-end: html-->

**My design:** The static master, states S1 to S8 in `graph.py`, is the same for every process. Its `TRANSITIONS` table lists which state may follow which, and `_route` raises `RuntimeError` for anything else. S5 compiles the process into a second graph with one node per step and edges only from the process (`test_compile_adds_no_node_the_process_does_not_define`). The engine, E1 to E7, drives it through eight commands and six statuses, and the machine never ends the run itself (`DynamicMachine`).

**The alternative:** Generating a single combined graph per process with supervision baked in, or letting the micro-level Claude agent emit the next workflow node directly. Both are simpler to wire up, but either lets a process alter how it is supervised or lets the model improvise the business sequence.

**What it costs:** Maintaining two macro graphs, an engine bridge between them, a command protocol, and a strict schema boundary around the micro tier. 

**Why I accept it:** Each tier has one job. A process file can configure only the retry, timeout, and budget values the runtime exposes, and the micro agent can reason freely without ever escaping the macro guardrails.

## Why keep a micro-level agent loop inside each step

**The problem:** If you force every low-level action—reading a file, inspecting a malformed input, or refining a draft—into its own YAML node, the process file becomes unreadable and brittle.

**What the micro tier buys us:** Autonomy where it is safe and bounded. Inside a single cognitive step, Claude still operates as a real agent: it can explore allowed context, chain tool calls, recover from minor formatting errors, and iterate until the step's artifact is ready. Because the macro tiers above it cap execution time, token budgets, and valid exit outcomes, you keep the adaptability of an LLM for messy local tasks without giving it control over business-level routing.

## Why the model returns a label, not a next node

After a node finishes, `select_edge` picks exactly one outgoing edge; zero matches or multiple matches raise a `RoutingError`. The diagram below shows a single cognitive attempt. Click the boxes for details.

{{island:det-attempt}}

<!--kg-card-begin: html-->
<details class="sec-more" id="more-model"><summary><span class="sec-more-show">Show the full section</span><span class="sec-more-hide">Hide the section</span></summary></details>
<!--kg-card-end: html-->

**The problem:** A model allowed to name its own next step can name one the workflow never authorized.

**The alternative:** Let the model return a target node name and have the runtime follow it. That leaves too much room for creative routing.

**My choice:** The model writes `result.json`, validated against `node_result.schema.yml`, containing an `outcome` label and a handoff note. The cognitive step in `step_cognitive.py` rejects any outcome not listed in the step's `outcomes`, an empty `handoff.done` field, or a missing declared product (an output artifact required by the step), and `run_attempts` in `dynamic/journal.py` runs the retry loop around those checks (`test_missing_product_and_bad_outcome_are_schema_failures`). The developer wires the outgoing edge for each valid label in advance.

**What it costs:** The model cannot route dynamically, and returning an invalid label counts as a failure that burns a retry and extra model calls.

**Why I accept it:** A retry is bounded. An unexpected route is not.

**Where the model still decides:** Model agency is restricted to three explicit boundaries:
1. **Process selection (`S2`):** Claude may propose a process ID from the catalog, which the developer must confirm (`test_s2_unknown_process_id_from_claude_is_not_accepted`).
2. **Step execution:** Inside any cognitive step, the generated content belongs to the model.
3. **Error resolution (`S7`):** Claude may propose `edit_file`, `override`, or `choose_process`, applied only after developer confirmation and restricted to `.process`, `.input`, or `identity` (`test_changes_outside_the_allowed_folders_are_refused`).

**Tool boundaries:** `ClaudeCli` passes `runtime.tools` via `--tools` and step-level tools via `--allowedTools` (`test_argv_order_and_invariant_part`). 

**What a human reviewer must still audit:** A step's `instruction` field is free text passed as the Goal block. No interpreter enforces rules written inside prompt prose; only `PRINT('...')` directives in `success` and `failure` are parsed. Any business rule written only in prompt text is a suggestion, not a control. Because the runtime guarantees step order, outcomes, products, and budgets—not prompt compliance—a reviewer must still inspect the instruction text and outcome list. Any rule that truly matters belongs in the graph as a step, an outcome, and an edge.

## Why procedure and human steps run no model

**The problem:** A step that must behave the same every time should not depend on a model, and a command string is an easy place for surprises.

**My choice:** 
- **Procedure steps** execute an explicit argument list with `shell=False` (`test_sandbox_exec_runs_an_argument_list_not_a_shell`). The executable must appear in `PROCEDURE_ALLOWED_EXECUTABLES` (default `python`, `python3`, `/opt/venv/bin/python`, overridable through `AGENT_GRAPH_PROC_EXES`), inline code and `-m` flags are blocked, and scripts must live under `.process/scripts/`.
- **Human steps** require explicit confirmation, and a rejection loops back to the target step with reviewer feedback in a fresh session (`test_human_rejection_loops_back_with_feedback_and_a_new_session`).

- **What it costs:** No shell one-liners and no inline scripts. Every small deterministic operation has to be saved as a standalone script file under `.process/scripts/`. I accept that authoring friction because a file in version control can be reviewed.

## Why I validate first and bound everything after

**The problem:** Catching structural workflow bugs mid-run wastes tokens and leaves partial side effects behind.

**My choice:** State `S3` validates the entire process graph before any cognitive step runs. (`S2` can already hold a Claude dialog, to propose a process ID.) `check_transitions` flags `missing_edge`, `dup_edge`, `unknown_outcome`, `unreachable`, and `dead_end` errors, alongside checks for file reads, tool subsets, declared products, and isolation settings. Any validation failure routes to `S7` as `PROCESS_INVALID`.

<!--kg-card-begin: html-->
<details class="sec-more" id="more-gates"><summary><span class="sec-more-show">Show the full section</span><span class="sec-more-hide">Hide the section</span></summary></details>
<!--kg-card-end: html-->

**The alternative:** Find errors when a run reaches them, after tokens are spent. 

**What validation costs:** A stricter authoring loop: a process containing an unreachable step is rejected before startup, even if that branch would never have executed.

Once a run starts, three runtime gates enforce hard bounds:
- **Budgets:** Defaults are capped at `50` steps, `50` model calls, `4` hours per run, and `600` seconds per step, configurable via `overrides.json` (`test_gate_counts_steps_and_llm_calls_and_refuses_at_the_limit`). Intentional cycles—such as the review loop—pass static validation and are bounded at runtime by `max_steps` (`test_loops_are_bounded_by_max_steps`). *Trade-off:* `max_steps` is a blunt ceiling; it cannot distinguish a productive revision loop from a stuck one.
- **Retries (`E4`):** Failed attempts retry in the same session with the failure reason appended, up to the step's `retry` setting or `DEFAULT_RETRY` (`2` attempts total) (`test_failed_attempt_is_retried_in_the_same_session_with_the_reason`). Context-window, model-unavailable, and budget errors do not retry. Once retries are exhausted, execution suspends and escalates to a human in `S7` (`test_retry_exhaustion_escalates_with_every_attempt_and_suspends`). *Trade-off:* Retrying inside the same session preserves context, which can also cause the model to repeat the same mistake.
- **Output checks (`E5`):** Enforces `non_empty`, `max_words`, `min_words`, `json_valid`, `yaml_valid`, and template conformance. Failures are flagged as `OUTPUT_INVALID`, and the artifact is preserved and marked invalid for inspection.

## Why side effects are journaled

**The problem:** Resuming an interrupted workflow that re-executes an already completed side effect breaks determinism.

**My choice:** `NodeRun.once` journals each side effect in the `dyn_journal` table inside `checkpoint.sqlite`. Running `agent_graph run --resume <runID>` picks up where the process left off without repeating completed work (`test_resume_after_the_process_died_mid_node_repeats_nothing`).

**What it costs:** Extra SQLite state and strict discipline: any side effect written outside `NodeRun.once` bypasses the journal..

## Why isolation is a separate tool

A procedure step can request sandboxed execution by setting `isolate: true` alongside a `profile`. `ContainerExec.run_isolated` then invokes the Go binary `run_ephemeral_container` as a subprocess (located by an explicit argument, then `AGENT_GRAPH_EXEC_TOOL`, then `PATH`, then the default `/usr/local/bin/run_ephemeral_container`).

**My choice:** Zero Python bindings into `agent-exec`—the two packages meet strictly at a CLI boundary. Keeping them decoupled prevents the Python orchestrator from becoming tightly coupled to the Go runner's internals.

**What it costs:** Subprocess overhead and a mandatory profile check. Validation queries `run_ephemeral_container list --json`, and setting `isolate: true` without a valid profile raises `isolate_profile_required` and fails before execution (`test_isolate_true_without_a_profile_fails`). How the container itself is locked down is the subject of my [next post]({{link:disposable-exec}}).

## Key takeaways

- Keep business step ordering in a declarative file and enforce it in code—you cannot audit a probabilistic choice after the fact.
- Every architectural guardrail buys auditability or a hard resource bound at the cost of authoring effort, rigidity, or extra machinery.
- A deterministic graph guarantees execution order, allowed outcomes, artifacts, and budgets—but a human still needs to review the step instructions and outcome definitions.

## What this post leaves out

- How an isolated step is built and limited. That is my [next post]({{link:disposable-exec}}).
- Prompt wording, context composition, and the skill and snippet pools.
- The model login.

## Run it yourself

The accompanying code, the `agent-graph` package, is a reference implementation of a supervised runner for YAML-defined processes. Its `agent_graph run` command runs one, and `agent_graph run --resume <runID>` continues a suspended run.

These steps are documented for a standard Ubuntu 26.04 "foundation" VM, which gives an immediate local sandbox to inspect the network gate without external cloud dependencies. I created the VM with Multipass (Hyper-V) on Windows. 

*Target Environment:* The commands run inside the agent container, started through `enter_agent`, and call the `claude` CLI. They read `.process/` and `.input/` and write under `.agent/<runID>/` and `.output/<runID>/`. The agent image build (`Dockerfile.agent`) copies `src/agent_graph` to `/opt/agent_graph`, adds a `.pth` file, and installs an `agent_graph` wrapper in `/usr/local/bin`.

*Full runbook:* the [Multipass and Lima runbook]({{link:howto}}) takes a machine to a running agent container. Part A prepares the machine and builds the agent image, and Part B (step 11) runs agent-graph.


<!--kg-card-begin: html-->
<div style="border-left:4px solid #b91c1c;background:color-mix(in srgb, #b91c1c 10%, transparent);padding:12px 16px;margin:1em 0;border-radius:4px;">

**Run this evaluation harness strictly within an isolated, disposable environment!** This repository is an architectural reference pattern designed to benchmark isolation boundaries, not an off-the-shelf production system. Hardening an untrusted-code sandbox for enterprise multi-tenancy—including custom seccomp profiles, network egress filtering, and tenant quotas—requires environment-specific design.

</div>
<!--kg-card-end: html-->


## Code references

These are the places in the code where the deterministic-flow decision shows up. I name functions and tests, not line numbers.

<!--kg-card-begin: html-->
<details class="sec-more" id="more-code"><summary><span class="sec-more-show">Show the full section</span><span class="sec-more-hide">Hide the section</span></summary></details>
<!--kg-card-end: html-->

In this post's package, `agent-graph`:

- [graph.py]({{link:code}}/src/agent_graph/graph.py) holds `TRANSITIONS`, `_route` and `build_master`.
- [router.py]({{link:code}}/src/agent_graph/dynamic/router.py), [builder.py]({{link:code}}/src/agent_graph/dynamic/builder.py) and [driver.py]({{link:code}}/src/agent_graph/dynamic/driver.py) hold `select_edge`, `compile_dynamic` and `DynamicMachine`.
- [journal.py]({{link:code}}/src/agent_graph/dynamic/journal.py) holds `NodeRun.once` and `run_attempts`.
- [transitions.py]({{link:code}}/src/agent_graph/process/transitions.py) and [validate.py]({{link:code}}/src/agent_graph/process/validate.py) hold `check_transitions`, the other static checks and `INLINE_CODE_ALLOWED`.
- [step_cognitive.py]({{link:code}}/src/agent_graph/dynamic/step_cognitive.py) holds `run_cognitive`, with the `result.json`, outcome and product checks.
- [e4_failure_policy.py]({{link:code}}/src/agent_graph/engine/e4_failure_policy.py) and [budgets.py]({{link:code}}/src/agent_graph/engine/budgets.py) hold `e4_failure_policy`, the budgets and the `overrides.json` handling.
- [container.py]({{link:code}}/src/agent_graph/services/container.py) and [claude_cli.py]({{link:code}}/src/agent_graph/services/claude_cli.py) hold `SandboxExec`, `ContainerExec` and `ClaudeCli.build_argv`.
- [summarize-notes.yml]({{link:code}}/examples/.process/summarize-notes.yml) is the example process.
- [test_wp4_claude_cli.py]({{link:code}}/tests/wp4_services/test_wp4_claude_cli.py), [test_wp5_units.py]({{link:code}}/tests/wp5_execution/test_wp5_units.py), [test_wp5_machine.py]({{link:code}}/tests/wp5_execution/test_wp5_machine.py), [test_wp5_engine.py]({{link:code}}/tests/wp5_execution/test_wp5_engine.py), [test_wp6_master.py]({{link:code}}/tests/wp6_static/test_wp6_master.py), [test_wp6_states.py]({{link:code}}/tests/wp6_static/test_wp6_states.py) and [test_wp6_resolution.py]({{link:code}}/tests/wp6_static/test_wp6_resolution.py) hold the tests named above.

`agent-exec` shares this tag with the six chapter 1 and chapter 2 posts, but I give no link for it here.


## References

- Source of the facts in this post: the README and code of `agent-graph` in [the code tag]({{link:code}}).
- Stated intent of the design: `design/01_user_story/uss-008_create_agentic_graph_4_process.md` and the `design/03_sys_req/agent-graph/` notes in the repository.
- LangGraph documentation: [langchain-ai.github.io/langgraph](https://langchain-ai.github.io/langgraph/).

To reproduce, clone the tag `ch01-ch02/publication`, which the six chapter 1 and chapter 2 posts share.

<!-- TODO(author): add 40-hex sha of ch01-ch02/publication -->
```
git clone --branch ch01-ch02/publication https://github.com/vectorNvolt/vectorNvolt-blog
```
