---
title: "Why I Granted SYS_ADMIN to Untrusted Agent Containers, and Why I Accept It in gVisor (#2)"
type: dev
code_tag: ch01_found/p01_p02_p03
code_package: agent-sandbox
code_url: https://github.com/vectorNvolt/vectorNvolt-blog/tree/ch01_found/p01_p02_p03/chapter01/packages/agent-sandbox
slug: foundation-gvisor-hardening
status: draft
tags: [foundation, gvisor, isolation]
custom_excerpt: "I grant SYS_ADMIN to untrusted agent containers because gVisor evaluates it inside the Sentry, not on the host. I walk through my runsc daemon.json flag by flag, compare runc with gVisor, and state the residual risk I accept."
feature_image:
links:
  isolation-runtime: https://vector-n-volt.ghost.io/foundation-isolation-runtime/
  howto-ch01: https://github.com/vectorNvolt/vectorNvolt-blog/blob/ch01_found/p01_p02_p03/chapter01/howto/README.md
---

<!--
OUTLINE (planning pass, not rendered). No requirement or threat-case IDs cited, so no refs block.

1. The runtime boundary and daemon.json (more-boundary)
   - Intro: why I register runsc with a fixed flag set; platform context WSL2, systrap, no /dev/kvm assumption.
   - Island bundle gvisor-hardening (placeholder in the lead, appears once). Mount div island-gvisor-hardening is emitted by the publisher.
   - daemon.json block, flag by flag, hedged wording.
   - Memory note: root:memory overlay uses host RAM; container root lost on exit.
2. Why I accept SYS_ADMIN in gVisor (more-sysadmin)
   - Capability set and why each group is there.
   - Alternative A: runc (mount div gvisor-capability-runc).
   - Alternative B: gVisor (mount div gvisor-capability-gvisor).
   - Residual risk: Sentry / Gofer / host kernel escape; Sentry is the new trust boundary.
3. What I have and have not verified (more-verify)
4. What this post leaves out (more-scope)
5. References (more-refs)

Links: isolation-runtime = post 01 of this chapter.

TODO verify against the gVisor docs before publishing (body wording is conservative until then):
 - --file-access=exclusive together with --overlay2=root:memory: exact semantics and interaction.
 - --file-access-mounts=shared: exact semantics for bind mounts.
 - --allow-flag-override=false: what it blocks (OCI annotations?) and whether the name is current.
 - --directfs=false: that it routes file access through the Gofer, and what the Gofer then does.
 - --directfs=false transport mechanism (9P vs internal IPC / how requests reach the Gofer) and the magnitude of the file I/O cost for file-heavy workloads such as node_modules installs (expected, not measured).
 - --net-raw=false: whether it is the right switch for raw sockets, and the default.
 - --network=sandbox: that it means the Netstack user-space stack instead of host network namespace use.
 - --overlay2=root:memory: that it is host-RAM backed, and how it is accounted against the container memory limit.
 - Whether the root:memory overlay counts against a per-container --memory limit (not verified), and whether swap bounds or a bounded tmpfs / mounted volume for large writes (torch-sized pip installs, big builds) actually contain host RAM use (not tested).
 - systrap: that it is the default or recommended platform when /dev/kvm is absent, and that it works on WSL2 as I run it.
 - Sentry host privileges: that it runs without host capabilities in this setup, and the exact seccomp posture.
 - Sentry makes a small seccomp-filtered set of host syscalls of its own; exact set and directfs interaction not verified.
 - Which of the listed capabilities (SYS_ADMIN, SYS_CHROOT, MKNOD, SETFCAP, ...) are honoured by the Sentry, and what each exposes.
 - Whether a hand-checked run of apt, pip, npm inside the sandbox succeeds with this exact flag set (not run for this post).
 - Run it yourself, none of steps 2-6 run: how `runsc install` treats an existing daemon.json; what `uname -r` and `dmesg` print under runc and runsc; whether the tmpfs mount with and without SYS_ADMIN gives the same visible result under both runtimes (and the effect of Docker's default seccomp/AppArmor under runc); the exit code and `docker stats` behaviour of the 512 MiB write under --memory 256m with root:memory.
-->

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
#more-boundary:not([open]) ~ :not(h2, p:has(> a[id]:empty:only-child), #more-boundary ~ h2 ~ *),
#more-sysadmin:not([open]) ~ :not(h2, p:has(> a[id]:empty:only-child), #more-sysadmin ~ h2 ~ *),
#more-run:not([open]) ~ :not(h2, p:has(> a[id]:empty:only-child), #more-run ~ h2 ~ *),
#more-code:not([open]) ~ :not(h2, p:has(> a[id]:empty:only-child), #more-code ~ h2 ~ *),
#more-verify:not([open]) ~ :not(h2, p:has(> a[id]:empty:only-child), #more-verify ~ h2 ~ *),
#more-scope:not([open]) ~ :not(h2, p:has(> a[id]:empty:only-child), #more-scope ~ h2 ~ *),
#more-refs:not([open]) ~ :not(h2, p:has(> a[id]:empty:only-child), #more-refs ~ h2 ~ *) {
  display: none;
}
</style>
<!--kg-card-end: html-->

## The runtime boundary and daemon.json

Running autonomous LLM agents means executing untrusted, machine-generated code inside your infrastructure. If an agent needs to debug code, install dependencies (apt-get, pip, npm), and run test suites, it requires a fully functional Linux environment.
Under standard container runtimes like runc, granting the permissions needed for build workflows is an unacceptable blast radius. A single escape or misconfiguration gives the agent access to the host kernel.
To solve this, I run gVisor's runsc as a dedicated Docker runtime. This post details the configuration I use in /etc/docker/daemon.json, the architectural defense model behind each flag, and the reasoning behind a choice that usually terrifies security teams: granting SYS_ADMIN to an untrusted agent container. I covered why I chose gVisor in the first place in [the isolation runtime post]({{link:isolation-runtime}}).

The diagram below shows the runtime boundary and where each flag sits. Click the boxes for details.

{{island:gvisor-hardening}}

<!--kg-card-begin: html-->
<details class="sec-more" id="more-boundary"><summary><span class="sec-more-show">Show the full section</span><span class="sec-more-hide">Hide the section</span></summary></details>
<!--kg-card-end: html-->

### The configuration

This is the `runtimes` entry in `/etc/docker/daemon.json`:

```json
{
  "runtimes": {
    "runsc": {
      "path": "/usr/local/bin/runsc",
      "runtimeArgs": [
        "--platform=systrap",
        "--network=sandbox",
        "--net-raw=false",
        "--allow-flag-override=false",
        "--overlay2=root:memory",
        "--file-access=exclusive",
        "--file-access-mounts=shared",
        "--directfs=false"
      ]
    }
  }
}
```

### Flag by flag
Each runtime argument explicitly shrinks the attack surface exposed to the agent while managing the performance penalties inherent to user-space syscall emulation.

- `--platform=systrap`: Interception platform selection dictates both performance and host requirements. While the KVM platform yields lower latency, it requires hardware virtualization access (/dev/kvm), which is often unavailable in nested virtualized cloud instances or standard WSL2 developer environments.
- `--network=sandbox`: Instead of bridging the agent directly to the host's network stack, this flag directs all network traffic through gVisor’s Netstack—a complete TCP/IP stack implemented entirely in Go.
Because the agent interacts only with Netstack, it never touches the host kernel's network drivers or raw socket primitives. An exploitation vector inside the Linux network stack does not compromise the host; an attacker is instead confined to the Go user-space network implementation.
- `--net-raw=false`: Autonomous agents should never need to craft arbitrary link-layer frames, execute ARP poisoning, or run raw socket port scanners inside internal VPC subnets. Setting --net-raw=false (paired with stripping CAP_NET_RAW from the container capability set) disables raw socket creation at the runtime boundary.
- `--allow-flag-override=false`: Prevents per-container runtime overrides via Docker CLI flags or compose annotations. I set it to keep per-container annotations or flags from loosening the runtime config.
- `--overlay2=root:memory`: The container’s root filesystem writes are redirected to an ephemeral, in-memory overlay. This enforces two operational properties:
    1. Total Ephemerality: All filesystem modifications, compiled artifacts, and downloaded payloads vanish completely when the container stops.
    2. RAM Exhaustion Hazard: Because writes consume host memory, a run of pip install torch or large compilation artifacts will rapidly fill the RAM overlay. I run a container with this flag under a Docker memory limit (--memory) to reduce the risk of host out-of-memory events. I have not checked how this overlay is counted against that limit. See "Code references" for what my code sets.

- `--file-access=exclusive` and `--file-access-mounts=shared`: This split balances sandbox caching against data integrity:
1. exclusive tells the sandbox that no host processes will modify the root filesystem out-of-band. The Sentry caches file metadata and directory trees aggressively without re-validating against the host filesystem on every syscall.
2. shared applies to explicitly mounted external volumes (e.g., source code repos bind-mounted from the host), forcing the sandbox to invalidate caches and respect external host file writes.
- `--directfs=false`: DirectFS allows the Sentry sandbox to execute direct filesystem operations against host file descriptors for performance. Disabling it (--directfs=false) forces all file I/O to be brokered through an isolated auxiliary process called the Gofer via a private IPC mechanism.

The trade-off is stark: file I/O operations (such as traversing heavy node_modules trees) take a substantial throughput hit. In exchange, the Sentry process never receives ambient host filesystem access tokens. With directfs off, I expect a compromised Sentry to have no direct host file descriptors to read from, so the Gofer becomes the next boundary I rely on. I have not verified this.

**My view:** None of these flags makes the sandbox a perfect wall. Each one removes a path I would otherwise have to defend, and each leaves a smaller piece of code that I trust instead.

## Why I accept SYS_ADMIN in gVisor

Standard container security orthodoxy dictates that CAP_SYS_ADMIN should never be granted to untrusted workloads. Under runc, SYS_ADMIN is functionally equivalent to host root—it exposes kernel namespaces, mount tables, eBPF, and device control.
However, complex development agents require elevated capabilities to function. Below is the capability set I grant to the agent container:

```json
["SYS_ADMIN","SYS_CHROOT","MKNOD","CHOWN","DAC_OVERRIDE","FOWNER","FSETID","SETUID","SETGID","SETPCAP","SETFCAP","KILL","AUDIT_WRITE","NET_BIND_SERVICE","SYS_RESOURCE"]
```

`SYS_ADMIN` is the one that needs a defence. On a normal Linux container runtime it is close to root on the host kernel. I grant it here because the agent needs a working build environment, and gVisor changes who evaluates the capability. The diagrams show the two cases. Click the boxes for details.

<!--kg-card-begin: html-->
<details class="sec-more" id="more-sysadmin"><summary><span class="sec-more-show">Show the full section</span><span class="sec-more-hide">Hide the section</span></summary></details>
<!--kg-card-end: html-->

### Scenario A based on runc

The container process communicates directly with the host Linux kernel. When a process requests a restricted operation (e.g., mount()), the host kernel checks if the process possesses CAP_SYS_ADMIN. If granted, the host kernel executes the operation directly within its own memory space. An unpatched bug anywhere in the kernel's mount, namespace, or device logic results in a full host escape.

The diagram shows where the capability is checked in this case. Click the boxes for details.

<!--kg-card-begin: html-->
<div id="gvisor-capability-runc"></div>
<!--kg-card-end: html-->

### Scenario B based on gVisor

The container process makes system calls that are intercepted before ever touching the host kernel. They terminate in the Sentry—gVisor’s user-space kernel written in memory-safe Go.
The Sentry evaluates SYS_ADMIN entirely within its emulated user namespace.
When the agent calls mount(), the Sentry updates its internal in-memory virtual mount table. No host mounts are performed.
The Sentry itself runs as an unprivileged host process restricted by an aggressive seccomp filter. The host kernel never sees or grants a SYS_ADMIN request.

The diagram shows where the capability is checked in this case. Click the boxes for details.

<!--kg-card-begin: html-->
<div id="gvisor-capability-gvisor"></div>
<!--kg-card-end: html-->

### Residual risk model

Granting SYS_ADMIN under gVisor does not eliminate risk; it relocates it to an engineered perimeter:
Expanded Sentry Surface: Enabling features like in-sandbox mounts forces the Sentry to run more of its own emulation code. More reachable Go code means a larger attack surface for Sentry logic bugs.
Gofer and IPC Exploits: The sandbox boundary relies on secure communication between the Sentry and the Gofer process.
Seccomp Escape: An attacker must first compromise the Sentry's memory space, locate an exploitable vulnerability in the reduced set of host syscalls permitted by the Sentry's seccomp filter, and then compromise the underlying host kernel.

I accept SYS_ADMIN under gVisor because defending an emulated, memory-safe user-space kernel behind a strict seccomp profile is significantly more defensible than exposing the bare host kernel to arbitrary LLM code.


## What this post leaves out

This post covers the runtime flags and the SYS_ADMIN decision. These topics are out of scope.

- Seccomp and AppArmor profile tuning.
- The KVM platform. I use `systrap` and do not assume `/dev/kvm`.
- Rootless `runsc`.
- Network egress design. The `agents-net` gate handles it, in a separate package and post, and I do not discuss it here.
- Multi-tenant quotas.
- Cgroup and disk-quota tuning.

## Run it yourself

This post argues three things: the `runsc` runtime is registered with one fixed set of flags, `SYS_ADMIN` is evaluated by the Sentry under gVisor and by the host kernel under runc, and the `root:memory` overlay uses host RAM. The steps below let me, and you, look at each of them with plain `docker` commands. They do not run the `agent-sandbox` lifecycle tool, which is a different topic.

*Target environment:* a disposable Linux host with Docker, where you can run `docker` as your own user and where a manual operator step (root) registers the runtime. Nothing here was run on a host by me yet.

*Full runbook:* the [chapter 1 runbook]({{link:howto-ch01}}) takes a bare Windows + WSL2 machine to a running agent container, including creating `agent-host` with the `runsc` runtime registered and creating the container with the capability set above (its sections 3 and 8 cover this post's runtime).
<!--kg-card-begin: html-->
<div style="border-left:4px solid #b91c1c;background:#fef2f2;color:#7f1d1d;padding:12px 16px;margin:1em 0;border-radius:4px;">

**Run this evaluation harness strictly within an isolated, disposable environment.** This repository is an architectural reference pattern designed to benchmark isolation boundaries, not an off-the-shelf production system. Hardening an untrusted-code sandbox for enterprise multi-tenancy—including custom seccomp profiles, network egress filtering, and tenant quotas—requires environment-specific design.

</div>
<!--kg-card-end: html-->


## Code references

I name functions and constants, not line numbers, because line numbers drift. Paths are relative to `packages/agent-sandbox/` and link to the code tag.

<!--kg-card-begin: html-->
<details class="sec-more" id="more-code"><summary><span class="sec-more-show">Show the full section</span><span class="sec-more-hide">Hide the section</span></summary></details>
<!--kg-card-end: html-->

| What | Where | Name |
|---|---|---|
| Container creation arguments | [`internal/services/docker.go`]({{link:code}}/internal/services/docker.go) | `CreateArgs` (called by `Docker.Create`) |
| The create-and-start step | [`internal/steps/s06_create_start.go`]({{link:code}}/internal/steps/s06_create_start.go) | `CreateAndStart` |
| Runtime, limits, tmpfs | [`internal/config/config.go`]({{link:code}}/internal/config/config.go) | `Runtime`, `Memory`, `CPUs`, `PidsLimit`, `TmpfsTmpPath`, `TmpfsTmpOptions`, `TmpfsDockerPath`, `TmpfsDockerOptions` |
| Capability allowlist | [`internal/config/config.go`]({{link:code}}/internal/config/config.go) | `Capabilities` |
| Verification after start | [`internal/steps/s08_verify.go`]({{link:code}}/internal/steps/s08_verify.go) | `Verify`, `inspectMismatches` |
| Entry point | [`cmd/create_agent_container/main.go`]({{link:code}}/cmd/create_agent_container/main.go) | `main` |
| runsc flags (other package) | `packages/agent-host/internal/config/config.go` | `RunscRuntimeArgs`, `DaemonJSON` |
| Runtime registration (other package) | `packages/agent-host/internal/steps/s10_register_runtime.go` | `RegisterRuntime`, `RunscInstallCmd` |

What the code passes, read from `CreateArgs` and `config.go` (code-read, not run):

- `--runtime=runsc`, `--cap-drop ALL`, then one `--cap-add` per entry of `Capabilities`, and `--security-opt no-new-privileges`. The 15 capabilities are the same ones, in the same order, as the list in this post. `NET_RAW` is not among them, which matches the `--net-raw=false` section.
- `--memory 4g`, `--cpus 2`, `--pids-limit 512`. These are constants. The environment overrides in `config.Load` cover only the log level, paths and timeouts, so memory, CPUs and pids are fixed design values.
- `--memory-swap` is not passed. Docker's default swap allowance therefore applies. I have not checked how that behaves under `runsc`.
- `--tmpfs /tmp:rw,noexec,nosuid,size=512m` and `--tmpfs /var/lib/docker:rw,nosuid,size=2g`. These bound only those two paths. I have not checked how the `root:memory` overlay counts against the 4g limit.
- `--restart no`, and the network `agents-net`.

What the registration does, read from `packages/agent-host` (code-read, not run):

- `RunscRuntimeArgs` holds eight arguments. They are the same eight, in the same order, as the `runtimeArgs` in the configuration above. I found no mismatch.
- `RegisterRuntime` does not write the JSON by hand. It copies `runsc` to `/usr/local/bin/runsc` and runs `runsc install --runtime=runsc --` followed by the eight arguments. I have not compared the resulting `daemon.json` with the block above.

What the verification checks: `Verify` reads `docker container inspect` and compares the runtime, `CapDrop`, the exact `CapAdd` set, `no-new-privileges`, memory, CPUs, pids limit, restart policy, tmpfs, mounts and network. It does not check swap. On any mismatch it re-pauses the container and reports `CONTAINED` or `UNSAFE`. It never removes it.

## References

- gVisor documentation: security model, platforms, networking, filesystem and Gofer, and `runsc` configuration flags. URLs are added once I have checked the flag wording above.

<!--kg-card-begin: html-->
<details class="sec-more" id="more-refs"><summary><span class="sec-more-show">Show the full section</span><span class="sec-more-hide">Hide the section</span></summary></details>
<!--kg-card-end: html-->

- Docker documentation: alternative runtimes and the `runtimes` entry in `daemon.json`.
- Linux `capabilities(7)` manual page, for the meaning of each capability on a normal kernel.
