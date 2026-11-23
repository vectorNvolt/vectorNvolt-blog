---
title: "Your Specs, Your Process, Real Physics: Integrating ngspice into a Deterministic Agentic System #ch02p03"
type: dev
code_tag: ch01-ch02/publication
code_package: agent-exec
code_url: https://github.com/vectorNvolt/vectorNvolt-blog/tree/ch01-ch02/publication/chapter02/packages/agent-exec
slug: exec-ngspice
status: published
tags: [exec, ngspice, agent-exec, simulation, go]
custom_excerpt: "ngspice as a declared tool in agent-exec, run in a disposable container with a spice-safe input policy and a record of every run, shown on a DC-DC boost converter flow. Process and tool contract are deterministic; the verdict is the model's. Unit-tested on a mock engine, not run on a live daemon."
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
#more-tool:not([open]) ~ :not(h2, p:has(> a[id]:empty:only-child), #more-tool ~ h2 ~ *),
#more-refuse:not([open]) ~ :not(h2, p:has(> a[id]:empty:only-child), #more-refuse ~ h2 ~ *),
#more-process:not([open]) ~ :not(h2, p:has(> a[id]:empty:only-child), #more-process ~ h2 ~ *),
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


## Why a spec-driven flow needs a real simulator

**The setting.** Hardware teams own two artifacts: **specs** (hard electrical requirements with pass/fail limits) and **processes** (who runs which check, in what order, and what happens on failure). When introducing AI agents into circuit design, that process must stay owned by the engineering team—not improvised by a language model.

**The problem.** A language model can size components and draft a SPICE netlist in seconds, accelerating early design exploration. But an LLM claiming a circuit meets spec is worth nothing unless a real physics simulator produced the numbers. An estimated ripple figure is a hallucination, not a measurement.

**The risk.** Letting an agent run a simulator unsupervised creates three operational hazards:

- **Security and IP exposure:** SPICE netlists can execute arbitrary shell commands or load external C libraries, and an unrestricted container can leak proprietary designs over the network.
- **Runaway compute:** Unbounded transient simulations can exhaust memory, disk, or CI time.
- **Zero auditability:** If a design passes review, you cannot prove which exact netlist, parameters, and simulator command produced the result.

**The architecture.** Below, I break down how I wired `ngspice` into an agentic workflow using three guardrails—illustrated through a DC-DC boost converter design loop:

- **Team-owned orchestration:** A declarative process graph controls step transitions; the model only operates inside bounded steps.
- **Disposable execution:** Every simulation runs in an offline, root-read-only container with strict CPU, memory, output, and netlist-policy limits.
- **Cryptographic provenance:** Every run outputs a tamper-evident record—exact command, SHA-256 input hashes, exit code, and failure cause.

The worked example is a DC-DC boost converter demo.

## The architecture: the model decides inside a step, the graph decides the rest

**Core design rule:** The model decides inside a step; the graph decides everything around it.

**The split.** Three parties act around a single `ngspice` run. The process graph, driven by `agent-graph`, decides which step comes next. The model decides inside one step: it sizes components, writes netlists, and reads results. `agent-exec` decides how a simulation runs—the command, the mounts, and the resource limits. The model chooses none of those. Click the boxes below for details.

{{island:ngspice}}

<!--kg-card-begin: html-->
<details class="sec-more" id="more-arch"><summary><span class="sec-more-show">Show the full section</span><span class="sec-more-hide">Hide the section</span></summary></details>
<!--kg-card-end: html-->

**One run.** In the `simulate` step the graph starts the Claude CLI with the step prompt and the step's allow-list (`--allowedTools`), then waits for it to exit. The model runs `run_ephemeral_container tool ngspice` itself through its Bash tool, with an exec id and a netlist. `agent-exec` validates the request, creates a container, starts it, waits, and writes the record. The model reads `result.json` and the `.meas` values from `sim.log`, writes `sim.md` and the step outcome. The graph routes on that outcome and never reads the exec directory.

**The package.** `agent-exec` is a Go 1.22 module with three binaries: `run_ephemeral_container`, which runs as uid 1001, `ephemeral-authz`, a Docker engine authorization plugin, and `agent-init`. Its subcommands are `run`, `build`, `install`, `tool`, `list [--json]` and `sweep --label k=v`. A profile's kind must match the subcommand. `agent-graph` checks the tool input names in a process file against `run_ephemeral_container list --json`.

## Why ngspice runs as a tool kind, not as code the model executes

A profile is an operator-written file that states which program may run, with which arguments, inputs and limits, so a simulation becomes a fill-in-the-blanks request instead of a shell command. It is needed because the agent has to run circuit simulations on its own, and nobody wants the model composing the command line for a program that reads and writes files.

**The decision.** The model never gets raw shell access to `ngspice`. Instead, `ngspice` is declared as a restricted tool in `/workspace/.tools/ngspice/profile.yml`, and the model can only supply an execution ID, input file paths and typed `--param` values that are validated against the profile.

<!--kg-card-begin: html-->
<details class="sec-more" id="more-tool"><summary><span class="sec-more-show">Show the full section</span><span class="sec-more-hide">Hide the section</span></summary></details>
<!--kg-card-end: html-->

**What the profile fixes.** A tool profile defines `entry`, `argv`, `inputs`, `input_policy`, `params`, and `limits`. The entry point is fixed and `argv` is required. `argv` accepts only literal tokens and three placeholders: `{out}`, `{in.<name>}`, and `{param.<name>}`. Nothing can be appended after expansion, preventing the model from injecting extra CLI flags. `validateToolArgv` enforces this grammar at load time, `ExpandToolArgv` performs substitution, and `Param.Check` validates parameter values.

**Two profiles.** The package README defines a single-pass reference run:

```yaml
# Reference profile argv
argv: ["ngspice", "-b", "-n", "-r", "{out}/sim.raw", "-o", "{out}/sim.log", "{in.netlist}"]
```

The boost-converter demo profile uses a wrapper image to capture both `.meas` logs and raw waveforms cleanly:

```yaml
# Demo profile (/workspace/.tools/ngspice/profile.yml)
name: ngspice
kind: tool
image_id: sha256:000...   # placeholder, replaced by the ID of exec-ngspice:1 after the build
ref: exec-ngspice:1
entry: fixed
argv: [ngspice-run, "{out}", "{in.netlist}"]
inputs:
  netlist:
    path_under: [src/, .input/, .output/exec/]
    glob: "*.cir"
  include:
    path_under: [src/, .input/]
    glob: "*.lib"
    multiple: true
    optional: true
input_policy: spice-safe
limits:
  mem: 512m
  cpus: 1
  pids: 128
  timeout_s: 600
  out_max: 256m
```

Compared with the reference profile, the demo profile uses the image `exec-ngspice:1` and the wrapper `argv` `ngspice-run {out} {in.netlist}` instead of calling `ngspice` directly. It sets `input_policy: spice-safe`, limits of 512 MiB memory, 1 CPU, 128 PIDs, a 600 s timeout and `out_max` 256m, and takes the netlist (`*.cir`) from `src/`, `.input/` or `.output/exec/`, plus optional `*.lib` includes from `src/` or `.input/`. Code-read.

**The wrapper.** `ngspice-run` is baked into the demo Dockerfile. It invokes `ngspice` twice: once without `-r` to flush `.meas` metrics into `out/sim.log`, and once with `-r` to write `out/sim.raw` and `out/sim_raw.log` (working around batch-mode `.meas` behavior when `-r` is passed).

**Inputs.** Files under `src/` are mounted in place and hashed. Other files, such as generated netlists in `.input/`, are staged into `in/<name>` with their SHA-256 digests recorded, up to 1 GiB.


## What the container and the policy refuse

**Policy first.** Because SPICE `.control` blocks act as full scripting environments capable of invoking OS commands and loading C shared libraries, `input_policy: spice-safe` runs `applyInputPolicy` across the netlist and its dependencies before any container is created. It blocks `shell`, `source`, and `codemodel` inside `.control` blocks, as well as any `.include` or `.lib` directive not explicitly allow-listed. A violation exits with code `125` and cause `policy` without creating an execution directory.

<!--kg-card-begin: html-->
<details class="sec-more" id="more-refuse"><summary><span class="sec-more-show">Show the full section</span><span class="sec-more-hide">Hide the section</span></summary></details>
<!--kg-card-end: html-->

**Defense in depth.** `applyInputPolicy` is a lexical filter, not a full SPICE parser—for example, it does not inspect XSPICE `file=` parameters. Treat it as a fast pre-flight check; the container sandbox is the real security boundary.

**The container.** Building on the isolation model from [disposable containers]({{link:disposable-exec}}), `agent-exec` configures every tool run with `NetworkMode: none`, private IPC, all Linux capabilities dropped, `no-new-privileges`, a read-only root filesystem, `AutoRemove`, an empty entrypoint, and user `1001:1001`. The working directory is `/work`, with a minimal environment (`HOME=/home/run`, `LANG=C.UTF-8`, `TZ=UTC`, `OUT`, and `IN`). Ephemeral `tmpfs` mounts provide 512 MiB each for `/work` and `/tmp` (`/tmp` is mounted `noexec` unless `tmp_exec` is set) and 128 MiB for `/home/run`. Memory, CPU, and PID ceilings come directly from the profile.

**Mounts.** `src/` is mounted read-only; `build/` is mounted read-only if `build_dir` is configured; `.output/exec/<id>/in` is mounted read-only; and `.output/exec/<id>/out` is the sole read-write directory.

**Limits and outcomes.** Hard maximums are 2 GiB memory, 3600 s timeout, and 1 GiB `out_max`; a profile that asks for more is rejected:

| Event | Exit Code | Cause |
|---|---|---|
| Timeout exceeded | `124` | `timeout` |
| `out_max` exceeded (polled every 1 s) | `137` | `out_cap` |
| Out of memory | `137` | `oom` |
| `SIGTERM` or `SIGINT` sent to tool | `130` | `cancel` |
| `ngspice` exits non-zero | Simulator exit code | `exit` |

**Pre-start verification.** Before starting the container, `Verify` inspects the created container config against the execution plan. Any mismatch aborts the run with cause `verify`.

## The boost-converter process: specs in, a written record out

**Process inputs and outputs.** Before any simulation runs, the engineering team supplies three read-only inputs. At the end of the loop, the workflow produces an electrical design that meets the requirements, plus an auditable bundle of artifacts.

- **Inputs (team-owned):** Electrical pass/fail limits (`HW_REQ.md`), an immutable SPICE testbench template (`bench.cir`), and the workflow state machine (`.process/boost-design.yml`).
- **Outputs (agent-generated, simulator-backed):** Six corner netlists (`.input/boost_design/*.cir`), raw simulation logs and waveforms (`.output/exec/<id>/out/`), and a final compliance report (`REPORT.md`) backed by `.meas` numbers.

**The demo.** The DC-DC boost converter demo is an ideal synchronous boost converter: 9 to 15 V in, 24 V out at 2 A. `HW_REQ.md` holds HW-BST-01 to HW-BST-08, and the process is `.process/boost-design.yml`. The sequence diagram shows one pass through it. Click a step for details.

<!--kg-card-begin: html-->
<details class="sec-more" id="more-process"><summary><span class="sec-more-show">Show the full section</span><span class="sec-more-hide">Hide the section</span></summary></details>
<!--kg-card-end: html-->

<!--kg-card-begin: html-->
<div id="ngspice-loop"></div>
<!--kg-card-end: html-->

**The limits.** The ones I use here:

- `vout_avg` in [23.52, 24.48] V.
- `vout_pp` at most 0.240 V.
- `il_pp / il_avg` at most 0.30 at 2 A.
- Settling: `|vout_avg - vout_avg_prev|` at most 0.120 V.
- E6 component values at least 1.1x the minimum.
- One L and one C across six corners, `v09_l02` to `v15_l20`.

**The testbench.** `bench.cir` is an immutable SPICE template parameterized with `<VIN>`, `<D_TON>`, `<L>`, `<C>`, `<RLOAD>`, `<IL0>`, and `<VOUT0>`. It runs `.tran 10n 1m 0 10n uic` and evaluates `.meas` statements for `vout_avg`, `vout_avg_prev`, `vout_pp`, `il_avg`, and `il_pp` across the 0.8 ms to 0.9 ms and 0.9 ms to 1.0 ms windows.

**The steps.** All five workflow nodes are cognitive steps where the model acts within strict file boundaries:

1. `calc`: Computes analytical component values and duty cycles.
2. `netlist`: Generates six corner netlists under `/workspace/.input/boost_design/<corner>.cir`.
3. `simulate`: The model runs `ngspice` only through the allow-listed command `Bash(run_ephemeral_container tool ngspice:*)` with sequential IDs (`bst<N>`), parses `.meas` output, and inspects waveforms via `readraw.py` (also allow-listed). It writes `sim.md`.
4. `evaluate`: Has `Read` and `Write` only, no Bash. It compares the metrics in `sim.md` against `HW_REQ.md` and `design.md` and outputs a compliance table ending in `pass` or `fail`.
5. `report`: Generates `REPORT.md` concluding with `RESULT: PASS`.

**Transitions.** A clean `simulate` advances to `evaluate`, whereas a `tool_error` routes back to `netlist` to fix syntax or policy errors. An `evaluate` `fail` routes back to `calc` to resize L and C. The model writes the outcome (`ok`, `tool_error`, `pass`, `fail`) to `result.json` in its step directory; the graph only checks that it is a declared outcome before following the matching edge.

**Guardrails.** The process states more rules than I list here. The key ones are: never modify `bench.cir` or `HW_REQ.md`, run `ngspice` only through the declared tool, never reuse an execution ID, and base pass/fail judgments strictly on measured `.meas` outputs. The allow-list is passed to the Claude CLI as `--allowedTools`; that the CLI enforces it is read from code, not tested.

**Reading results.** This happens in the `simulate` step. The Go runner intentionally treats simulator output as opaque bytes—it does not parse `sim.log` or `sim.raw`. The model reads the exec `result.json` (`exit`, `cause`), collects the `.meas` values from `sim.log`, and uses `readraw.py` (Python standard library only) with `--at`, `--svg`, and `--vars` to inspect `sim.raw`. It writes the values into `sim.md` and sets `ok` only if every corner has exit 0, cause `ok` and all five values; otherwise `tool_error`. `evaluate` judges from `sim.md` without running anything.

## What stays deterministic and what does not

| Deterministic (Enforced in Go) | Non-Deterministic (LLM Judgment) |
|---|---|
| `argv` expansion from `profile.yml` | Sizing of L and C |
| Mounts, `spice-safe` policy outcome, and resource limits | Generated netlist text |
| `result.json` (`exit`, `cause`, exec dir, image ID, timestamps) | Choice of execution ID (`bst<N>`) |
| Input SHA-256 hashes and expanded `argv` in `meta.json` | Parsing `.meas` values from `sim.log` |
| Fixed `LANG` and `TZ` environment | Final pass/fail verdict and `REPORT.md` |
| Unique execution IDs (reuse exits with `125`) | Step outcome that drives routing (`ok`, `tool_error`, `pass`, `fail`) |

Each run produces a machine-readable audit record alongside `meta.json`:

```json
// Example .output/exec/<id>/result.json
{
  "exit": 0,
  "cause": "ok",
  "exec_dir": "/workspace/.output/exec/bst1",
  "image_id": "sha256:...",
  "start": "2026-11-18T10:00:00.123456789Z",
  "end": "2026-11-18T10:00:04.123456789Z"
}
```

`result.json` holds exactly these fields: `exit`, `cause`, `exec_dir`, `image_id`, `start` and `end`. It holds no user name, home directory or host name. `exec_dir` is a path inside the container's workspace (default root `/workspace`, whatever `AGENT_EXEC_WORKSPACE` holds), so a run outside a container with a host-path root would write that host path. `meta.json` holds the caller-supplied `--in` paths, labels, the agent process PID (label `ephemeral.pid`) and the expanded `argv`, all `/workspace` paths; container output such as `sim.log` is opaque bytes the Go code does not scrub, so check it before sharing a record. This is design/code reading (`internal/sequence/phase.go`), not Observed.

**Closing the deterministic loop.** By design, `evaluate` is currently a model step—there is no Go or Python assertion checker verifying `.meas` numbers against `HW_REQ.md` yet. Here, "deterministic" guarantees the execution contract and input provenance, not the final engineering sign-off. Replacing model-judged evaluation with a deterministic `.meas` spec checker is the logical next milestone.

**What I do not claim.** `ngspice` is run without custom RNG seeds or result caching, so bit-identical waveform output across runs is not guaranteed. Reproducibility here is strictly scoped: identical input SHA-256 digests and identical `argv` execution. Auto-generated execution IDs follow `e<base36>-<8hex>` via `crypto/rand`, ensuring every run stays isolated.

**Key takeaways:**

- Put the execution contract in code and isolate LLM judgment to bounded steps—and document the boundary explicitly.
- An AI agent's "meets spec" claim is only as trustworthy as the `.meas` line behind it and the hash linking it to the netlist.
- Pairing sandboxed simulation with deterministic spec assertions removes the last subjective link in automated hardware verification.

## What this post leaves out

I name these so they are not read as covered:

- The container model and the authorization plugin (the previous post, [disposable containers]({{link:disposable-exec}})).
- Params reaching the netlist. Today they reach only `argv` (open item USS-009).
- Slot accounting, which can race across concurrent tool processes.
- npm and conda manifests.

## Run it yourself

The accompanying code, the `agent-exec` package, is a reference implementation of ephemeral containers for agent code execution in Go. Its `run_ephemeral_container tool` subcommand runs one declared tool, here ngspice, in a disposable container and writes a record of the run.

```
CGO_ENABLED=0 go build ./cmd/...
go vet ./... && go test ./...
```

The tests use a fake engine, so they do not run ngspice.

These steps are documented for a standard Ubuntu 26.04 "foundation" VM, which gives an immediate local sandbox to inspect the network gate without external cloud dependencies. I created the VM with Multipass (Hyper-V) on Windows. 

*Target Environment:* The commands talk to the Docker engine. They read the tool profile, `src/` and `.input/`, mount `src/` read-only, and write only under `.output/exec/<id>/`.

*Full runbook:* the [Multipass and Lima runbook]({{link:howto}}) holds the setup steps; I do not copy them here.


<!--kg-card-begin: html-->
<div style="border-left:4px solid #b91c1c;background:color-mix(in srgb, #b91c1c 10%, transparent);padding:12px 16px;margin:1em 0;border-radius:4px;">

**Run this evaluation harness strictly within an isolated, disposable environment!** This [repository]({{link:code}}) is an architectural reference pattern designed to benchmark isolation boundaries, not an off-the-shelf production system. Hardening an untrusted-code sandbox for enterprise multi-tenancy—including custom seccomp profiles, network egress filtering, and tenant quotas—requires environment-specific design.

</div>
<!--kg-card-end: html-->


## Code references

These are the places in the code where the decision to run ngspice as a bounded tool shows up. I describe what each one does.

<!--kg-card-begin: html-->
<details class="sec-more" id="more-code"><summary><span class="sec-more-show">Show the full section</span><span class="sec-more-hide">Hide the section</span></summary></details>
<!--kg-card-end: html-->

In this post's package, `agent-exec`:

- [tool.go]({{link:code}}/internal/profile/tool.go) holds `validateTool`, `validateToolArgv`, `ExpandToolArgv` and `Param.Check`, plus the constant `InputPolicySpiceSafe`.
- [inputpolicy]({{link:code}}/internal/inputpolicy) holds the spice-safe policy and its tests `TestSpiceSafeAccepts`, `TestSpiceSafeRejects` and `TestSpiceSafeScansIncludeFiles`.
- [sequence.go]({{link:code}}/internal/sequence/sequence.go) holds `Execute`, `validateInputs`, `stage` and `generateExecID`.
- [phase.go]({{link:code}}/internal/sequence/phase.go) holds `startAndWait`, where the timeout, output cap and cancel outcomes are decided.
- [kind.go]({{link:code}}/internal/kind/kind.go) holds `NewPlan`, `ToolEnv`, `Mounts` and `Verify`.
- [limits.go]({{link:code}}/internal/limits/limits.go) holds the limit ceilings.
- [tool_test.go]({{link:code}}/tests/tool_test.go) holds `TestToolRunReference`, `TestToolRunNonZeroExitPassedThrough`, `TestToolRunTimeoutPassedThrough`, `TestToolStagesInputsOutsideSrc`, `TestToolParams`, `TestToolUsageErrorsCreateNothing` and `TestToolInputPolicyRefusesBeforeExecDir`.
- [authz_contract_test.go]({{link:code}}/tests/authz_contract_test.go) holds `TestToolBodiesAreAllowedByPlugin`.

The demo files (`test/TC1_boost_converter/workspace/`: `.tools/ngspice/profile.yml`, `.tools/ngspice/Dockerfile`, `.process/boost-design.yml`, `src/ngspice/boost_design/bench.cir`, `src/ngspice/boost_design/HW_REQ.md`, `src/ngspice/readraw.py`) are in the repository outside the `agent-exec` package, so I name them without a link.

In `agent-graph`, also outside this package and named without a link: `services/claude_cli.py` starts `claude -p` with the step's `--allowedTools`, and `dynamic/router.py` routes on the step outcome.


## References

- Source of the facts in this post: the package README and code of `agent-exec` in [the code tag]({{link:code}}).
- ngspice manual and project site: [ngspice documentation](https://ngspice.sourceforge.io/docs.html).
- Docker documentation on container run settings: [docker container run](https://docs.docker.com/reference/cli/docker/container/run/).


To reproduce, clone the tag `ch01-ch02/publication`, which the six chapter 1 and chapter 2 posts share.

<!-- TODO(author): add 40-hex sha of ch01-ch02/publication -->
```
git clone --branch ch01-ch02/publication https://github.com/vectorNvolt/vectorNvolt-blog
```
