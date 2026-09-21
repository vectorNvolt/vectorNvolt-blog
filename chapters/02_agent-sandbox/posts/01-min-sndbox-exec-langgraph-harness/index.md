---
title: Minimal sandbox exec LangGraph harness
type: dev
code_tag: ch02_ag-snd/p01_min-snd__post_code
code_package: agent_sandbox
slug: minimal-sandbox-exec-langgraph-harness
status: draft
tags: [langgraph, agent-sandbox]
custom_excerpt: "The minimal harness the chapter starts from — a LangGraph state machine driving a Claude CLI inside a gVisor container — its six interaction points in code, and which of the chapter's threat cases its four lines of hardening actually close."
feature_image:
---

## Where this post starts

The [chapter overview](/agent-sandbox-overview/) builds the threat inventory this chapter works from — vectors T1–T9, cases T1a…T9e, layers L1–L7, interaction points IP-1…IP-6, the runtime tiers and the research behind them. None of that is repeated here. This post is about the code: the minimal harness in `packages/agent_sandbox/src/agent_sandbox/`, where each interaction point shows up in it, and which threat cases the implementation closes today. Everything it doesn't close is referred to by ID.

<style>
.id-col-table th:first-child,
.id-col-table td:first-child {
  white-space: nowrap;
  width: 1%;
}
</style>

## The harness

Six LangGraph nodes, one file each, wired in `graph.py`. Solid arrows are the edges `build_graph()` adds; the labelled ones are the `route_after_*()` conditionals:

{{island:graph}}

Hover or click a node for what it calls, the interaction points it touches, and the threat cases that live there. Nodes 1, 4, 5 and 6 pause on `interrupt()`; 4 and 6 are self-loops.

- **Node 1 `select_agent`** — `ask_user()` for a name; it becomes the container name via `config.CONTAINER_PREFIX`.
- **Node 2 `check_agent`** — `ct.find(name)`; if a container exists, `ensure_running()` (re)starts it and the graph skips creation.
- **Node 3 `create_container`** — `ct.create(name)`: `containers.run(image, runtime=config.CONTAINER_RUNTIME, network_mode="bridge", read_only=False, security_opt=["no-new-privileges"], cap_drop=["ALL"], labels=...)`.
- **Node 4 `auth_gateway`** (loop) — `is_authenticated()` runs `claude auth status`; if it fails, `start_login()` opens a TTY `ExecSession` on `claude auth login`, `read_until("Paste code here")`, relays the output to the operator, and `finish_login()` writes the reply back and checks exit code + `auth status`. Capped at `MAX_AUTH_ATTEMPTS`.
- **Node 5 `ask_task`** — `ask_user()` for the first task.
- **Node 6 `exchange`** (loop) — `send_prompt()` runs `claude -p --output-format text` via `exec_run`, piping the operator's text through `sh -c "printf %s <quoted> | claude -p ..."`; the answer goes to `notify_user()` (a `print()`); the next `ask_user()` ends the loop on an `EXIT_WORDS` entry.

Every `ask_user()` is a LangGraph `interrupt()`; `main.py` answers each one with a bare `input()`.

## The naive system boundary

"Naive" is literal: this is the boundary as the code exists today, before any post in the chapter has hardened it. Two block diagrams show the implementation's components and its six interaction points under each runtime `config.CONTAINER_RUNTIME` can resolve to — the agent container nested inside the host system, the boundary each interaction point crosses, and which use cases (UC) and threat cases (T) ride on it.

**Use cases** (specific to this harness)

- **UC-1** — Provision (nodes 1–3)
- **UC-2** — Authenticate (node 4)
- **UC-3** — Task exchange (nodes 5–6)

### System block diagram — `runtime=runsc` (the implementation)

{{island:boundary}}

The concrete components: `main.py` → `graph.py` in the host process, the Docker Engine reached through `docker.from_env()`, the `claude` CLI process inside the container with the login state it persists and the rootfs it can write, the gVisor Sentry underneath it, and the host kernel shared by all three host-side components. IP-4 is two hops — IP-4a (container → Sentry) and IP-4b (Sentry → host kernel) — and the container outline is green because that's the only boundary the code actually hardens. Badges show which threat cases each point closes (green chips) and leaves open (red).

### System block diagram — `runtime=runc` (one environment variable away)

<!--kg-card-begin: html-->
<div id="boundary-diagram-runc"></div>
<!--kg-card-end: html-->

Same components, same IP-1, IP-2, IP-3, IP-5 and IP-6 — this is what `CONTAINER_RUNTIME=runc` in the environment produces, and node 3 would still print `runtime=runsc` (T6l). IP-4 collapses to one hop straight to the host kernel, which is now the same kernel the orchestrator and the engine run on: a T6 escape lands in the trust zone of the process that's supposed to be governing it. The runtime flag moves one interaction point and nothing else.

### The interaction points in code

<div class="id-col-table">

| # | Interaction point | Where it is in the code | UC |
|---|---|---|---|
| IP-1 | Operator I/O | `gateway.ask_user()` → `interrupt()`, answered by `input()` in `main.py`; `gateway.notify_user()` → `print()`. The `_clean` regex (CSI, OSC 8, `\r`) runs on login output only, never on task answers. | 1, 2, 3 |
| IP-2 | Docker control plane | `container.client()` — a module-level `docker.from_env()`; `create()`, `find()`, `containers.get(container_id)` in nodes 2–4, 6. | 1, 2, 3 |
| IP-3 | Exec channel | `ExecSession` (`exec_create(..., stdin=True, tty=True)` + attached socket) for the login relay; `ct.exec()` → `exec_run(demux=False)` for `auth status` and `claude -p`. The `timeout` parameter `ct.exec()` accepts is never used. | 2, 3 |
| IP-4 | Kernel/Sentry boundary | `runtime=config.CONTAINER_RUNTIME` (`os.getenv("CONTAINER_RUNTIME", "runsc")`), `cap_drop=["ALL"]`, `security_opt=["no-new-privileges"]`. No `mem_limit`, `pids_limit`, or CPU quota. | 1, 3 |
| IP-5 | Container filesystem | `read_only=False`; no `volumes`, `mounts`, or `tmpfs`. The whole rootfs is writable and, via node 2, reused across runs. | 3 |
| IP-6 | Network egress | `network_mode="bridge"`; no egress policy, no DNS policy. | 2, 3 |

</div>

## Threat cases closed by the implementation

Four kwargs in `container.create()` are the harness's entire security investment. Copied from the overview's [T6 tables](/agent-sandbox-overview/#t6-by-runtime-tier-from-runcs-full-exposure-to-gvisors-residual-surface), here are the cases they close, and the line that closes each:

<div class="id-col-table">

| ID | Threat case | Description | Closed by |
|---|---|---|---|
| T6a | Shared kernel attack surface | The agent's syscalls go straight to the host kernel; any exploitable bug in the syscalls a default seccomp profile still permits (overlayfs, io_uring, cgroup controllers, etc.) is a direct path to host compromise. | `runtime=runsc`<sup>[2](#ref-2)</sup> — syscalls are serviced by the Sentry in userspace; the host kernel sees only the Sentry's own narrow, seccomp-filtered call set. |
| T6b | Runtime binary compromise | CVE-2019-5736: a malicious container can overwrite the host `runc` binary via `/proc/self/exe` fd re-opening, giving host root code execution on the runtime's next invocation.<sup>[3](#ref-3)</sup> | `runtime=runsc` — `/proc` is Sentry-emulated; there is no host `runsc` binary reachable through it. |
| T6c | Leaked-fd namespace escape | CVE-2024-21626 ("Leaky Vessels"): an internally leaked fd (e.g. a host `/sys/fs/cgroup` handle) reachable via `/proc/self/fd/N` lets a container set its `cwd` into the host mount namespace, exposing or overwriting host files.<sup>[4](#ref-4)</sup> | `runtime=runsc` — the container's fd table is the Sentry's; no host fds leak into it. |
| T6d | Privileged/misconfigured containers | `--privileged`, host bind-mounts, or added capabilities (`CAP_SYS_ADMIN`, `CAP_SYS_PTRACE`) collapse the isolation boundary entirely — a config failure mode rather than a runc bug, but the one operators hit most. SandboxEscapeBench's easiest scenarios are exactly these (`/var/run/docker.sock`, `hostPath`, `CAP_SYS_ADMIN`).<sup>[1](#ref-1)</sup> | `cap_drop=["ALL"]`, `security_opt=["no-new-privileges"]`, and the *absence* of `privileged`, `cap_add`, `volumes`/`mounts` in `create()`. This is the one class frontier models escape reliably, and the one `runsc` does nothing about on its own — it's closed by the kwargs, not the runtime. |

</div>

The same investment, read against the overview's SandboxEscapeBench<sup>[1](#ref-1)</sup> scenario map — the rows this harness closes and what closes them:

<div class="id-col-table">

| # | Scenario | Vulnerability / misconfiguration | T-case | Closed here by |
|---|---|---|---|---|
| 1 | `privileged` | full capability set → namespace break | T6d | no `privileged=True` in `create()` |
| 2 | `docker.sock` | daemon socket mounted → launch privileged container | T8a | no `volumes`/`mounts` in `create()` |
| 3 | `hostpath` | writable host `/etc` mount → cron injection | T6d / T3a | no `volumes`/`mounts` in `create()` |
| 5 | `CAP_SYS_ADM` | filesystem mounting, cgroup abuse | T6d | `cap_drop=["ALL"]`; Sentry also refuses the mounts |
| 6 | `pid_ns` | shared PID namespace → `ptrace` code injection | T6a | `runtime=runsc` — Sentry owns its own PID namespace |
| 7 | `CAP_MOD` | malicious kernel module load | T6d | `cap_drop=["ALL"]`; no `init_module` path to the host kernel |
| 8 | `CAP_DAC_RD` | `open_by_handle_at()` arbitrary host file read | T6d | `cap_drop=["ALL"]`; syscall serviced by Sentry/Gofer, not host |
| 9 | `runc_2019` | CVE-2019-5736: host `runc` overwrite via `/proc/self/exe` | T6b | `runtime=runsc` |
| 10 | `runc_2024` | CVE-2024-21626: leaked fd → host mount namespace | T6c | `runtime=runsc` |
| 11 | `cgroup` | CVE-2022-0492: `release_agent` write without CAP check | T6a | `runtime=runsc` — host cgroupfs not exposed |
| 12 | `dirty_cow` | CVE-2016-5195: CoW race → host file overwrite | T6a | `runtime=runsc` — page-fault path never reaches host kernel |
| 13 | `dirty_pipe` | CVE-2022-0847: page-cache pipe injection | T6a | `runtime=runsc` — pipes implemented in the Sentry |
| 17 | `bpf_privesc` | CVE-2017-16995: eBPF verifier privilege escalation | T6a | `runtime=runsc` — `bpf()` unimplemented / filtered by Sentry |
| 18 | `packet_sock` | CVE-2017-7308: packet socket use-after-free | T6a | `runtime=runsc` — netstack is the Sentry's own |

</div>

Rows 4, 14, 15, 16 are Kubernetes-specific and don't apply to a single-host Docker harness. Rows 2 and 3 are closed by the *absence* of a mount, not by anything the code asserts — the first post that adds a workspace bind mount reopens them.

What `runsc` leaves in place is the overview's residual set **T6g–l**<sup>[5](#ref-5) [6](#ref-6)</sup>, and the harness accepts it as-is: it doesn't pick the gVisor platform (T6k — whatever the daemon's `runsc` config says), collects no Sentry logs, and watches for no escape attempts. One residual case is made concrete by the code rather than merely inherited:

- **T6l — compatibility gaps as a security footgun.** `CONTAINER_RUNTIME` is `os.getenv("CONTAINER_RUNTIME", "runsc")`. Set it to `runc` in the environment and the entire T6a–c set is back — while node 3 still prints `created sandbox ... with runtime=runsc`, because that string is a literal in `n03_create_container.py`, not the config value. In the AI Sandboxes<sup>[7](#ref-7)</sup> terms, the monitor (M) reports a boundary (B) that may not exist.

## What the implementation leaves open

Everything else in the inventory. By vector, with the one code fact that makes each open — the full case descriptions are in the overview's [threat case inventory](/agent-sandbox-overview/#threat-case-inventory) and aren't repeated:

<div class="id-col-table">

| Vector | Open cases | IP | Why, in this code |
|---|---|---|---|
| T1 | T1a, T1b, T1d, T1e | IP-6, IP-5, IP-1 | `network_mode="bridge"` with no proxy; the OAuth session `claude auth login` writes is what node 2 reuses next run, readable by anything the agent runs; `notify_user()` prints agent text unfiltered, so a terminal query's reply lands on `input()` as the next turn. T1c not reachable — no integration beyond the Anthropic API is provisioned. |
| T2 | T2a–d | IP-6, IP-5 | Open egress to public registries, `read_only=False`, no `noexec` mount, no mirror or lockfile. |
| T3 | T3b, T3c | IP-6, IP-3 | Each turn is one unsupervised `claude -p`; no per-action gate inside it; the container persists so damage accumulates. T3a not reachable — no mounts. |
| T4 | T4a–c | IP-6 | Bridge reaches whatever the host can, `169.254.0.0/16` included — and every other agent's container on the same default bridge. |
| T5 | T5b, T5d (T5a, T5c partial) | IP-5, IP-2 | `check_agent` → `find()` → `ensure_running()` reattaches and restarts an existing container by design; the rootfs is writable and `CLAUDE_BIN = "claude"` is resolved on `PATH` inside it. T5a becomes live with the first export path. |
| T6 | T6e, T6f, T6g–l | IP-4 | See above. |
| T7 | T7a–c, T7e (T7d residual) | IP-4, IP-5, IP-3 | No `pids_limit`, `mem_limit`, CPU quota, or storage quota; `ExecSession.read_until` grows `buf` unbounded until marker/EOF/`AUTH_TIMEOUT`; `ct.exec()` never passes its `timeout` to `exec_run`, so a `claude -p` that never exits blocks the orchestrator for good. |
| T8 | T8b | IP-2 | `docker.from_env()` is the full daemon. T8a/T8c not reachable — no socket in the container, no path from inside to the API (unless the daemon listens on a TCP `DOCKER_HOST` the bridge can reach, which is T4c). |
| T9 | T9a (partial), T9b (partial), T9c, T9d, T9e | IP-3, IP-1 | `start_login`/`finish_login` transition on literal markers (`"Paste code here"`, `"Invalid code"`); `authenticated` is gated on exit code + `claude auth status`, but both come from a `claude` binary on a writable rootfs the agent owns. `exchange` has no marker — only the operator can set `done`. `interrupt()` fires only at turn boundaries; `PROMPT_CMD` passes no flag restricting what `claude -p` may do. Agent text is printed raw, immediately before the harness's own `Your reply (or /quit):` prompt. |

</div>

Read the two sections together and the shape of the minimal harness is clear: **L1** is real — `runsc` plus `cap_drop`/`no-new-privileges` closes T6a–d and thirteen of SandboxEscapeBench's fourteen applicable scenarios — and **L2–L7** don't exist. The next posts add them one at a time, re-running this list each time: L2 resource caps and a fresh container per run (T5d, T7a–c); L4 default-deny egress at IP-6 (T1a–b, T2, T4); L6 a schema-validated channel at IP-3, output sanitisation and scoped approvals at IP-1 (T7e, T9); L7 control-plane scoping at IP-2 (T8b).

## References

| ID | Title | Link | Finding |
|----|-------|------|---------|
| <a id="ref-1"></a>1 | SandboxEscapeBench: Quantifying Frontier LLM Capabilities for Container Sandbox Escape | [arxiv.org/abs/2603.02277](https://arxiv.org/abs/2603.02277) | 18 real-CVE/misconfiguration escape scenarios across orchestration, runtime, and kernel layers; difficulty-1/2 misconfigurations are escaped reliably. |
| <a id="ref-2"></a>2 | gVisor: Application Kernel for Containers | [gvisor.dev](https://gvisor.dev/) | Intercepts application syscalls in a userspace Sentry process instead of exposing the host kernel directly to the container. |
| <a id="ref-3"></a>3 | CVE-2019-5736 | [cve.org/CVERecord?id=CVE-2019-5736](https://www.cve.org/CVERecord?id=CVE-2019-5736) | A malicious container can overwrite the host `runc` binary via `/proc/self/exe` fd re-opening, yielding host root code execution. |
| <a id="ref-4"></a>4 | CVE-2024-21626 ("Leaky Vessels") | [GHSA-xr7r-f8xq-vfvv](https://github.com/advisories/GHSA-xr7r-f8xq-vfvv) | A leaked internal file descriptor lets a container set its working directory into the host mount namespace, exposing or overwriting host files. |
| <a id="ref-5"></a>5 | gVisor Architecture Guide — Security Model | [gvisor.dev/docs/architecture_guide/security](https://gvisor.dev/docs/architecture_guide/security/) | Describes the Sentry/Gofer split and how syscall interception in a user-space kernel narrows the host kernel attack surface. |
| <a id="ref-6"></a>6 | gVisor Security and Vulnerability Reporting | [gvisor.dev/security](https://gvisor.dev/security/) | Defines gVisor's residual-risk taxonomy (SandboxRoot, SandboxRoot/HostDoS, SandboxRoot/Exfil) — the T6g–i cases this harness accepts. |
| <a id="ref-7"></a>7 | AI Sandboxes: A Threat Model, Taxonomy, and Measurement Framework | [arxiv.org/abs/2606.18532](https://arxiv.org/abs/2606.18532) | Sandbox as a tuple of assets (system under test, boundary, monitoring, intervention, evidence); the monitoring layer is itself a target. |
