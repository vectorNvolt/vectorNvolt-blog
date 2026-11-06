---
title: "Securing LLM Code Execution: Why I Chose gVisor Over Docker and VMs for My Agent Framework (#01)"
type: dev
code_tag: ch01_found/p01_p02_p03
code_package: agent-sandbox
code_url: https://github.com/vectorNvolt/vectorNvolt-blog/tree/ch01_found/p01_p02_p03/chapter01/packages/agent-sandbox
slug: foundation-isolation-runtime
status: draft
tags: [foundation, isolation, gvisor, sandboxing]
custom_excerpt: "My agent framework runs untrusted, LLM-generated scripts. I compare seven isolation options, from a bare process to a full VM, against my threat model and explain why I chose gVisor and which trade-offs I accept."
feature_image:
links:
  overview: /foundation-overview/
  howto-ch01: https://github.com/vectorNvolt/vectorNvolt-blog/blob/ch01_found/p01_p02_p03/chapter01/howto/README.md
---

<!--
OUTLINE — planning pass, not prose.
1. The problem: an untrusted-code boundary (more-problem). Threat model, constraints, scope.
2. Alternative 1, bare Linux process (more-bare). Island: isolation (mount island-isolation).
3. Alternative 2, Docker / runc (more-runc). Mount iso-runc.
4. Alternative 3, WebAssembly / WASI (more-wasm). Mount iso-wasm.
5. Alternative 4, gVisor / runsc (more-gvisor). Mount iso-gvisor.
6. Alternative 5, microVM / Firecracker (more-microvm). Mount iso-microvm.
7. Alternative 6, Kata Containers (more-kata). Mount iso-kata.
8. Alternative 7, standard VM / QEMU-KVM (more-vm). Mount iso-vm.
9. My decision (more-decision).
10. What would change my mind (more-change).
11. References (more-refs).
No requirement or threat-case IDs are cited, so there is no refs appendix.
Islands: one bundle, isolation (seven BlockDiagram mounts, data in islands/src/groups/isolation.js).
Styles: the single style card below. Every h2 gets a sec-more marker and its #more-<id> in the selector list.
-->

<!--kg-card-begin: html-->
<style>
/* ---- Inline helpers ------------------------------------------------------
   id-col-table: wrap a Markdown table in <div class="id-col-table"> to keep
   its first (ID) column on one line. grant-*: a capability's verdict,
   <strong class="grant grant-scoped">SCOPED</strong>. req-kind-*: the kind of
   a requirement in running text. */
.id-col-table th:first-child,
.id-col-table td:first-child {
  white-space: nowrap;
  width: 1%;
}
.grant {
  font-weight: 700;
}
.grant-granted { color: #16a34a; }
.grant-scoped { color: #f59e0b; }
.grant-denied { color: #dc2626; }
.req-kind { font-weight: 700; }
.req-kind-func { color: #1f6fa8; }
.req-kind-struct { color: #1e8449; }
/* rating-*: a scorecard rating in text (<strong class="rating rating-weak">WEAK</strong>) and as an accordion item's accent (class="req-item rated-weak"). */
.rating {
  font-weight: 700;
}
.rating-absent { color: #dc2626; }
.rating-weak { color: #ea580c; }
.rating-partial { color: #ca8a04; }
.rating-strong { color: #16a34a; }
.req-accordion details.req-item.rated-absent { --req-accent: #dc2626; }
.req-accordion details.req-item.rated-weak { --req-accent: #ea580c; }
.req-accordion details.req-item.rated-partial { --req-accent: #ca8a04; }
.req-accordion details.req-item.rated-strong { --req-accent: #16a34a; }

/* ---- Requirement / threat-case accordion (templates/collapsible-list) ---- */
.req-accordion {
  --req-accent: #9ca3af;
  margin: 1.4em 0;
}
.req-accordion details.req-item {
  border: 1px solid color-mix(in srgb, var(--req-accent) 35%, transparent);
  border-left: 4px solid var(--req-accent);
  border-radius: 6px;
  margin: 0.5em 0;
  padding: 0;
  overflow: hidden;
  background: color-mix(in srgb, var(--req-accent) 4%, transparent);
}
/* Functional = blue, structural = green; system-level items take the deeper
   shade, technical-level items (TFSecR / TSSecR) the lighter one. */
.req-accordion details.req-item.req-func { --req-accent: #1f6fa8; }
.req-accordion details.req-item.req-struct { --req-accent: #1e8449; }
.req-accordion details.req-item.req-func[id^="tfsecr"] { --req-accent: #5dade2; }
.req-accordion details.req-item.req-struct[id^="tssecr"] { --req-accent: #52be80; }
.req-accordion details.req-item summary {
  cursor: pointer;
  list-style: none;
  padding: 0.65em 1em;
  font-weight: 600;
  background: color-mix(in srgb, var(--req-accent) 12%, transparent);
  transition: background 0.15s ease;
}
.req-accordion details.req-item summary:hover {
  background: color-mix(in srgb, var(--req-accent) 20%, transparent);
}
.req-accordion details.req-item summary::-webkit-details-marker {
  display: none;
}
.req-accordion details.req-item summary::before {
  content: "▸";
  display: inline-block;
  margin-right: 0.6em;
  color: var(--req-accent);
  transition: transform 0.15s ease;
}
.req-accordion details.req-item[open] summary::before {
  transform: rotate(90deg);
}
.req-accordion details.req-item[open] summary {
  border-bottom: 1px solid color-mix(in srgb, var(--req-accent) 35%, transparent);
}
.req-accordion details.req-item .req-id {
  font-family: monospace;
  font-size: 0.85em;
  opacity: 0.75;
}
.req-accordion details.req-item .req-type {
  font-size: 0.75em;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.03em;
  color: var(--req-accent);
}
.req-accordion details.req-item > :not(summary) {
  padding-left: 1em;
  padding-right: 1em;
  font-size: 0.92em;
}
.req-accordion details.req-item > p:first-of-type {
  padding-top: 0.8em;
}
.req-accordion details.req-item > p:last-child,
.req-accordion details.req-item > ul:last-child {
  padding-bottom: 0.8em;
}
.req-accordion details.req-item ul {
  margin: 0.3em 0;
  padding-left: 2.2em;
}
.req-accordion details.req-item li {
  margin: 0.2em 0;
}
/* Technical requirements sit indented under each system requirement they
   implement; one that implements several appears under each of them. */
.req-accordion .req-children {
  margin: -0.2em 0 0.9em 1.6em;
  padding-left: 0.8em;
  border-left: 2px dashed color-mix(in srgb, #9ca3af 55%, transparent);
}
.req-accordion .req-children details.req-item {
  margin: 0.35em 0;
}
/* Threat cases: red accent, as in the overview's diagrams. */
.req-accordion details.req-item.req-threat { --req-accent: #c0392b; }
/* Implementation guides (ImGd, id="imgd-…"): light grey, under each technical requirement. */
.req-accordion details.req-item.req-impl { --req-accent: #9ca3af; }

/* ---- Collapsible sections (templates/collapsible-section) ---------------
   Heading + lead stay visible; a closed .sec-more hides every following
   sibling up to the next h2. Anchor paragraphs (<p><a id="…"></a></p>) stay
   in the layout so in-page links still land. */
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
#more-problem:not([open]) ~ :not(h2, p:has(> a[id]:empty:only-child), #more-problem ~ h2 ~ *),
#more-bare:not([open]) ~ :not(h2, p:has(> a[id]:empty:only-child), #more-bare ~ h2 ~ *),
#more-runc:not([open]) ~ :not(h2, p:has(> a[id]:empty:only-child), #more-runc ~ h2 ~ *),
#more-wasm:not([open]) ~ :not(h2, p:has(> a[id]:empty:only-child), #more-wasm ~ h2 ~ *),
#more-gvisor:not([open]) ~ :not(h2, p:has(> a[id]:empty:only-child), #more-gvisor ~ h2 ~ *),
#more-microvm:not([open]) ~ :not(h2, p:has(> a[id]:empty:only-child), #more-microvm ~ h2 ~ *),
#more-kata:not([open]) ~ :not(h2, p:has(> a[id]:empty:only-child), #more-kata ~ h2 ~ *),
#more-vm:not([open]) ~ :not(h2, p:has(> a[id]:empty:only-child), #more-vm ~ h2 ~ *),
#more-run:not([open]) ~ :not(h2, p:has(> a[id]:empty:only-child), #more-run ~ h2 ~ *),
#more-code:not([open]) ~ :not(h2, p:has(> a[id]:empty:only-child), #more-code ~ h2 ~ *),
#more-decision:not([open]) ~ :not(h2, p:has(> a[id]:empty:only-child), #more-decision ~ h2 ~ *),
#more-change:not([open]) ~ :not(h2, p:has(> a[id]:empty:only-child), #more-change ~ h2 ~ *),
#more-refs:not([open]) ~ :not(h2, p:has(> a[id]:empty:only-child), #more-refs ~ h2 ~ *) {
  display: none;
}

/* ---- Reference popups (templates/ref-popup) ---------------------------- */
/* In-text IDs made interactive by the refs island: a dotted underline, a
   popup with the definition on hover or focus, the appendix entry on click. */
a.ref-link {
  color: inherit;
  text-decoration: underline dotted;
  text-decoration-color: color-mix(in srgb, currentColor 55%, transparent);
  text-underline-offset: 0.2em;
  cursor: help;
}
a.ref-link:hover,
a.ref-link:focus-visible {
  text-decoration-style: solid;
}
.ref-pop {
  position: absolute;
  z-index: 1000;
  box-sizing: border-box;
  width: min(560px, calc(100vw - 32px));
  max-height: min(60vh, 520px);
  overflow-y: auto;
  overscroll-behavior: contain;
  -webkit-overflow-scrolling: touch;
  padding: 0.8em 1.1em 0.7em;
  border: 1px solid color-mix(in srgb, currentColor 22%, transparent);
  border-left: 4px solid var(--req-accent, #9ca3af);
  border-radius: 6px;
  box-shadow: 0 8px 28px rgba(0, 0, 0, 0.22);
  /* Overridden by the refs island with the post text's own size (the theme's
     root font size may be 10px, which makes rem units unusable here). */
  font-size: 16px;
  line-height: 1.55;
}
.ref-pop[hidden] { display: none; }
.ref-pop .ref-pop-title {
  margin: 0 0 0.4em;
  font-weight: 700;
}
.ref-pop .ref-pop-title .req-id {
  font-family: monospace;
  opacity: 0.75;
  margin-right: 0.4em;
}
.ref-pop p { margin: 0.35em 0; }
.ref-pop ul { margin: 0.25em 0; padding-left: 1.3em; }
.ref-pop li { margin: 0.15em 0; }
</style>
<!--kg-card-end: html-->


## The problem: an untrusted-code boundary

**Core Premise:** Autonomous agent frameworks that execute LLM-generated bash commands face a harsh reality: you are running untrusted, arbitrary code on your infrastructure.

A hallucinated command can wipe local volumes; a prompt-injected payload can probe internal metadata endpoints, steal environment tokens, or attempt container escapes. Defaulting to standard Docker (runc) leaves the host kernel directly exposed. Conversely, spinning up dedicated virtual machines for every ephemeral tool call destroys reasoning-loop latency and inflates cloud costs.

Below is an architectural evaluation of seven isolation runtimes for agent engines in Go, why gVisor (runsc) hits the operational sweet spot for single-tenant clusters, and the explicit performance trade-offs accepted.

<!--kg-card-begin: html-->
<details class="sec-more" id="more-problem"><summary><span class="sec-more-show">Show the full section</span><span class="sec-more-hide">Hide the section</span></summary></details>
<!--kg-card-end: html-->

**My threat model.** I assume the generated code is hostile or at least careless. When untrusted code makes a system call, which software intercepts it, and how much of the host OS is directly exposed?

**My constraints.** These shape the weighing more than any single feature does.

1. **Arbitrary Ecosystem Execution:** The agent must be able to run arbitrary bash commands, install tools (`apt-get`, `pip`, `npm`), execute compiled binaries, and debug scripts.
2. **Short-Lived, High-Frequency Tasks:** Sub-agents spawn for single tool calls (running a script, parsing a repo) and terminate immediately. Cold-start latency directly degrades end-to-end user experience.
3. **High Concurrency & Density:** Running dozens of parallel sub-agent tasks on a single node requires a minimal idle memory footprint per sandbox.
4. **No Guaranteed Hardware Virtualization (`/dev/kvm`):** In managed cloud Kubernetes (EKS, GKE, AKS) or cost-effective CI environments, nested virtualization is often unsupported, restricted, or requires expensive bare-metal instances.
5. **Standard Container Ergonomics:** The runtime must leverage standard OCI container images, public registries, and familiar layering workflows.

**What I mean by "boundary".** The question I ask of every option is simple. When the untrusted code makes a system call, which software handles it, and how much of the host does that software expose? My answers differ by option, and I think that is the most useful lens.

1. Bare Process:   [App] ────────────────────────────────────────────────────────────────────► [Host Kernel]
2. Docker (runc):  [App] ─────────────────────────────[Namespaces/cgroups]───────────────────► [Host Kernel] (Shared attack surface)
3. WASM / WASI:    [Wasm Module] ───────────► [Wasm Runtime (Wasmtime)] ─────────────────────► [Explicit Host Imports]
4. gVisor (runsc): [App] ───────────► [Sentry (User Kernel)] ────────────────────────────────► [Host Kernel] (Minimal safe subset)
5. microVM:        [App] ───────────► [Guest Kernel] ────────► [Firecracker + KVM] ──────────► [Host Kernel]
6. Kata Containers:[App] ───────────► [Guest Kernel] ────────► [Hypervisor + KVM] ───────────► [Host Kernel]
7. Standard VM:    [App] ───────────► [Full Guest OS] ───────► [QEMU + KVM] ─────────────────► [Host Kernel]

**How to read the diagrams.** Each option has an interactive block diagram. Red marks the untrusted code. Green marks the component that holds the boundary. Dark marks shared host infrastructure. Circles are interaction points, the diamond is the asset I protect, the square is a threat. Hover or click one for the detail.

## 1. Bare Linux process

**How the boundary works:** The framework spawns generated scripts directly via os/exec. The only boundary is standard Linux Discretionary Access Control (file permissions and user accounts).

**The Syscall Path:** System calls hit the host Linux kernel directly with zero mediation.

**Why it fails for AI agents:** Even when executed under an unprivileged user, an agent can read world-readable files, probe local loopback services, consume unlimited CPU/memory unless manually wrapped in cgroups, and access any host information exposed to that UID. 

**My reading:** this is a fine choice for code I wrote and trust, and the wrong choice for code an LLM wrote. For my use case I rule it out.

{{island:isolation}}


## 2. Docker with runc

**How the boundary works:** Standard Docker uses Linux namespaces (PID, Mount, Network, IPC, UTS) to create isolated views of system resources, alongside cgroups to throttle CPU and memory.

**The Syscall Path:** Syscalls route directly to the shared host Linux kernel. Capabilities can be dropped and seccomp profiles applied, but the underlying execution surface remains the host kernel.

**The Risk Factor:** The Linux kernel exposes approximately 350+ system calls. Even Docker’s default seccomp profile allows roughly 300 of them. Container escapes—whether through kernel exploits (e.g., Dirty COW, Dirty Pipe), /proc or /sys leakage, or eBPF subsystem bugs—have repeatedly allowed escapes, because containers share the host kernel and merely segregate views. 

**My reading:** a good boundary for workloads I trust, with a residual risk I am not comfortable carrying for untrusted code.

<!--kg-card-begin: html-->
<div id="iso-runc"></div>
<!--kg-card-end: html-->


## 3. WebAssembly with WASI runtimes (Wasmtime, Wasmer)

**How the boundary works:** Code is compiled to WebAssembly bytecode and executed inside a strict linear memory sandbox managed by a WASM runtime. WASI (WebAssembly System Interface) enforces capability-based security: the module has zero access to the outside world unless explicitly granted by the host via imported functions.
* **The Syscall Path:** **None.** WebAssembly modules cannot execute raw Linux syscalls. Every file read, network socket, or clock lookup is mapped to a capability provided by the host runtime.
* **Why it falls short for autonomous agents:** While WASM delivers sub-millisecond cold starts, negligible memory overhead, and runtimes written in a memory-safe language, it **fails on ecosystem compatibility**. Autonomous agents need to execute arbitrary bash scripts, run `curl`, install untrusted Python packages with C-extensions, and run pre-built binaries. You cannot easily `pip install pandas` or execute raw bash inside standard WASI without running a heavy userland/OS emulator inside the bytecode, which destroys performance and ergonomics.

**My reading:** an attractive, light capability model for code I can compile to Wasm myself, and a poor match for an agent that runs whatever a shell can run. For my use case I do not choose it.

<!--kg-card-begin: html-->
<div id="iso-wasm"></div>
<!--kg-card-end: html-->

## 4. gVisor with runsc

**How the boundary works:** Developed by Google, gVisor introduces an application kernel written in Go called the Sentry. The Sentry intercepts all guest system calls in user space, acting as an intermediate OS. File access is mediated by the Sentry, through a separate, isolated process called the Gofer or through directfs, depending on configuration.

**The Syscall Path:** The untrusted code makes a syscall. The Sentry handles and emulates the operation entirely in user space. To interact with the host, the Sentry makes only a restricted set of host system calls, which narrows what the untrusted code can reach in the host kernel.

**Why it fits:** 
- runsc is an OCI-compliant runtime that drops directly into Docker or Kubernetes configurations.
- Using the modern systrap platform, gVisor intercepts system calls through seccomp-triggered signals handled by stub processes, without requiring /dev/kvm.
- Startup latency is within the sub-second window required for interactive agent tool loops. **My reading:** it narrows what untrusted code reaches directly, and it keeps the container workflow.

<!--kg-card-begin: html-->
<div id="iso-gvisor"></div>
<!--kg-card-end: html-->

## 5. microVM with Firecracker

**How the boundary works:** Firecracker, built by AWS, is an open-source Virtual Machine Monitor (VMM) written in Rust. It utilizes the Linux Kernel-based Virtual Machine (/dev/kvm) to launch minimal Linux guest kernels inside lightweight virtual machines.

**The Syscall Path:** Syscalls land inside a dedicated guest kernel. The guest reaches the host only through Firecracker's stripped-down virtio device model (virtio-net, virtio-block, minimal serial console), which the VMM implements on top of hardware virtualization extensions (Intel VT-x / AMD-V). The VMM is itself a host process that uses KVM and the host kernel, so the host kernel stays exposed through KVM and the VMM's own syscalls.

**The Bottleneck:** While Firecracker boasts millisecond-level boot times (5–50 ms) and hardware-level isolation, it strictly requires access to /dev/kvm. Operating Firecracker at scale requires managing guest kernel versions, building uncompressed ext4 rootfs images, and provisioning bare-metal hardware or environments with nested virtualization enabled. 

**My reading:** by design this is among the strongest boundaries of the seven for untrusted code. My cost is the KVM requirement and the work of operating guest kernels and root filesystems.

<!--kg-card-begin: html-->
<div id="iso-microvm"></div>
<!--kg-card-end: html-->

## 6. Kata Containers

**How the boundary works:** Kata Containers is an OCI-compliant runtime that provisions a dedicated, lightweight virtual machine for each container or Kubernetes Pod. It uses hypervisors like QEMU, Cloud Hypervisor, or Firecracker under the hood, wrapped by `containerd-shim-kata-v2`.
**The Syscall Path:** Syscalls land inside a dedicated guest Linux kernel. The guest kernel talks to the hypervisor, which relies on hardware virtualization extensions (KVM) to isolate execution from the host.
**My reading:** it is in the same class of boundary as a microVM, with a heavier startup and footprint. A widely used choice for strong container isolation, but operationally heavier and blocked in environments lacking nested KVM.

<!--kg-card-begin: html-->
<div id="iso-kata"></div>
<!--kg-card-end: html-->


## 7. Standard VM with QEMU/KVM

**How the boundary works:** Complete hardware emulation running an off-the-shelf, general-purpose operating system on top of a hypervisor.
**The Syscall Path:** Emulated hardware devices handle I/O, and CPU instructions run via hardware virtualization.
**Why it fails for agents:** Provisioning full VMs introduces boot latencies ranging from 5 to 30 seconds and baseline memory footprints measured in gigabytes. For an autonomous agent dispatching multiple short-lived commands in a single reasoning loop, traditional VMs introduce unacceptable latency and infrastructure bloat. 
**My reading:** Unlike microVMs, traditional hypervisors carry more device emulation than short-lived agent execution needs

<!--kg-card-begin: html-->
<div id="iso-vm"></div>
<!--kg-card-end: html-->


## My decision

For an agent framework orchestrating ephemeral code execution, gVisor (runsc) gives the balance of security, operational simplicity, and deployment flexibility that fits my constraints best.
- It delivers an architectural boundary where it matters most: the LLM's system calls are handled by the Sentry first, and the host kernel sees only the Sentry's restricted set of calls.
- The Trade-Offs I Explicitly Accept
Selecting an engineering architecture means deliberately choosing which drawbacks you are willing to manage:
- The I/O Performance Tax: File access is mediated by the Sentry, through the Gofer file proxy or directfs depending on configuration. Workloads that generate thousands of small file operations (such as running npm install or unpacking large tarballs) experience measurable overhead compared to native runc.
- Compatibility Limitations: Because the Sentry implements Linux kernel interfaces in software, rarely used system call flags, specialized ioctl requests, or direct hardware hooks will fail. For typical Python, Node.js, and Bash agent workflows, compatibility is solid, but it must be verified against your specific toolchains.
- Software-Defined Boundary: gVisor trades the sprawling attack surface of the C-based Linux host kernel (~30 million lines of code) for the memory-safe, smaller surface of the Go-based Sentry (~400,000 lines). It is a software boundary, not a hardware-isolated boundary.


**What to take away.**

- I ask which software handles the untrusted code's system calls.
- Runc shares the host kernel, and that is the point I weigh most against it for LLM-generated code.
- VMs, microVMs and Kata Containers give a stronger-looking boundary, and they need KVM and more operational work.
- A Wasm runtime gives a capability model, and it does not run the arbitrary bash and tools my agents need.
- gVisor is my compromise for my threat model and constraints. 

## What would change my mind

A sound architectural decision is tied to its underlying assumptions. I would re-evaluate this choice under the following conditions:

- Affordable, Ubiquitous Nested Virtualization: If managed cloud providers make nested KVM universally available and cost-neutral across all standard Kubernetes node types, the operational argument against microVMs weakens significantly.
- Strict Multi-Tenant Isolation: If our framework evolves to execute untrusted code from multiple external customers side-by-side on the same compute node, a software boundary alone may not be enough. Multi-tenant zero-trust necessitates hardware-enforced boundaries (Firecracker or Kata Containers).
- Workloads Shift to Heavy Disk I/O: If agent workflows become predominantly I/O-bound rather than compute- or API-driven, the Gofer filesystem proxy overhead may force a migration to microVMs with native block storage virtualization.

## Run it yourself
The accompanying code, the `agent-sandbox` package, is a reference implementation of the gVisor lifecycle gate in Go. Its `create_agent_container` command creates one isolated gVisor (`runsc`) agent container from a single `agentID`.

*Target Environment:* The command runs as root inside the `agent-host` distro and calls `docker` and `/usr/local/bin/create_agent_net` directly. It never reads or modifies `daemon.json` or any other host configuration.

*Full runbook:* the [chapter 1 runbook]({{link:howto-ch01}}) takes a bare Windows + WSL2 machine to a running agent container, including building the binaries, creating `agent-host` with the `runsc` runtime, and creating the container (its sections 3 and 8 cover this post's runtime).

<!--kg-card-begin: html-->
<div style="border-left:4px solid #b91c1c;background:#fef2f2;color:#7f1d1d;padding:12px 16px;margin:1em 0;border-radius:4px;">

**Run this evaluation harness strictly within an isolated, disposable environment.** This repository is an architectural reference pattern designed to benchmark isolation boundaries, not an off-the-shelf production system. Hardening an untrusted-code sandbox for enterprise multi-tenancy—including custom seccomp profiles, network egress filtering, and tenant quotas—requires environment-specific design.

</div>
<!--kg-card-end: html-->



## Code references

These are the places in the code where the gVisor decision shows up. I describe what each one does.

<!--kg-card-begin: html-->
<details class="sec-more" id="more-code"><summary><span class="sec-more-show">Show the full section</span><span class="sec-more-hide">Hide the section</span></summary></details>
<!--kg-card-end: html-->

In this post's package, `agent-sandbox`:

- [config.go]({{link:code}}/internal/config/config.go) holds the runtime name, `runsc`, as one constant that the rest of the code reads.
- [s02_check_runtime.go]({{link:code}}/internal/steps/s02_check_runtime.go) holds `CheckRuntime`. I use it to refuse to go on when the engine does not register runsc. It reports the mismatch and does not repair it.
- [s20_check_preconditions.go]({{link:code}}/internal/steps/s20_check_preconditions.go) holds `CheckPreconditions`, which runs the runtime check first, before any other prerequisite.
- [docker.go]({{link:code}}/internal/services/docker.go) holds `RuntimeRegistered`, which asks the engine for its runtime list, and the container creation call that passes the runtime to Docker.
- [s06_create_start.go]({{link:code}}/internal/steps/s06_create_start.go) holds the create-and-start step, where I create the container with that runtime.
- [s08_verify.go]({{link:code}}/internal/steps/s08_verify.go) holds the step that reads the container back and compares its runtime with the configured one.
- [sequence_test.go]({{link:code}}/tests/sequence_test.go) holds the tests `TestRuntimeMissingExits1` and `TestRunscAbsentFromRuntimesExits1` for the missing-runtime cases.

The registration of runsc with Docker lives in a separate package, `agent-host`, in its "register runsc runtime and restart docker" step (`s10_register_runtime.go`). Its verification step checks that the engine lists runsc and can run a container with it. I give no link for it here, because that package has its own tag.


## References

The facts I state about gVisor, Firecracker, WebAssembly runtimes and Kata Containers come from their public project sites.

<!--kg-card-begin: html-->
<details class="sec-more" id="more-refs"><summary><span class="sec-more-show">Show the full section</span><span class="sec-more-hide">Hide the section</span></summary></details>
<!--kg-card-end: html-->

- gVisor project site and documentation: [gvisor.dev](https://gvisor.dev/) and [gvisor.dev/docs](https://gvisor.dev/docs/).
- Firecracker project site: [firecracker-microvm.github.io](https://firecracker-microvm.github.io/).
- Firecracker source repository and documentation: [github.com/firecracker-microvm/firecracker](https://github.com/firecracker-microvm/firecracker).
- Wasmtime project site: [wasmtime.dev](https://wasmtime.dev/).
- Wasmer project site: [wasmer.io](https://wasmer.io/).
- WebAssembly System Interface (WASI): [wasi.dev](https://wasi.dev/).
- Kata Containers project site: [katacontainers.io](https://katacontainers.io/).
