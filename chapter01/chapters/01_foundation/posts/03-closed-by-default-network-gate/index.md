---
title: "Atomic Sandboxing: Building a Closed-by-Default Network Gate for Agent Code Execution in Go (#03)"
type: dev
code_tag: ch01_found/p01_p02_p03
code_package: agent-net
slug: foundation-closed-by-default-network-gate
status: draft
tags: [foundation, network, gvisor, nftables, go]
custom_excerpt: "I build a closed-by-default network gate in Go for gVisor agent containers. They attach at creation, stay closed, and open with one atomic nftables release after the host-side controls verify. End-to-end runs and several host checks are still pending."
feature_image:
links:
  gvisor-hardening:
  howto-ch01:
---

<!--
OUTLINE (planning pass, not rendered). No requirement or threat-case IDs are cited in the text, so there is no refs block and no gen_refs.py.

1. Why I attach at creation and close by default (more-problem)
   - runsc does not see interfaces added after start; docker network connect is rejected while none is attached (package README, recorded 2026-10-31).
   - Gate idea: agents-net created with a network-wide gate; MAC in set `released` AND the port mark set by the container's own port table opens a port (a restarted container has a new veth, so no mark).
2. The three layers (more-layers)
   - Island net-arch (lead). Layers: per-port netdev table on the veth (sets the mark), bridge port flags + static FDB on the veth, agents_gate netdev table + neighbour entry on the bridge device, iptables DOCKER-USER / INPUT by container IP.
   - Kernel without nftables bridge family; netdev family used.
3. The arming sequence (more-arming)
   - Island net-arming (lead). 13 steps (sequence.go armPipeline), five phases: preconditions (1-5), clean slate (6), identify + close (7-8), controls + read-back (9-10), atomic release (11-13).
   - Create, start, pause and unpause are done by the caller (create_agent_container), not by arm.
4. The Go implementation (more-go)
   - ExecRunner, argv only. Pipelines of numbered steps. Exit codes 0 / 1 / 2. No rollback except the release.
5. Edge cases and caveats (more-caveats)
6. What I have and have not verified (more-verify)
   - Evidence words: Observed, Unit-tested, Code-read, Pending.
7. What this post leaves out (more-scope)
8. References (more-refs)

Links: gvisor-hardening = post 02 of this chapter, empty until live. The built-in code link = this post's code tag (not cut yet).
Islands: net-arch, net-arming (one bundle each, built by another author).

Package facts checked against:
 - packages/agent-net: README.md, internal/sequence/sequence.go, internal/config/config.go, internal/steps/s13_apply_controls.go, s15_release_port.go, s17_check_gate.go, s22_disarm_remove_rules.go, s28_remove_agents.go, s34_require_paused.go, s37_disarm_close_gate.go, s40_clean_slate.go, internal/services/docker.go, gate.go, nft.go.
 - packages/agent-sandbox: README.md, internal/sequence/guard.go, internal/services/reconcile.go.
 - packages/agent-image/Dockerfile.agent (PID 1 is tini, CMD dockerd).
TODO before publishing:
 - Cut the code tag and fill code_tag.
 - Fill links: gvisor-hardening once post 02 is live.
 - Add a real end-to-end run (create, arm, check-gate, disarm, remove) as Observed evidence.
 - Run refresh_endpoints.exe on a live host (stdin install of endpoints.json, plus the seed at host creation) and record as Observed.
 - Run a live docker restart probe on agent-host: event sequence, whether MAC and IP are kept, new veth name, and what the guard does.
 - Run the port mark live on agent-host (arm, check-gate, a restarted container's unmarked frames dropped) and record as Observed. Today it is Observed only in an isolated network namespace.
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
#more-problem:not([open]) ~ :not(h2, p:has(> a[id]:empty:only-child), #more-problem ~ h2 ~ *),
#more-layers:not([open]) ~ :not(h2, p:has(> a[id]:empty:only-child), #more-layers ~ h2 ~ *),
#more-arming:not([open]) ~ :not(h2, p:has(> a[id]:empty:only-child), #more-arming ~ h2 ~ *),
#more-go:not([open]) ~ :not(h2, p:has(> a[id]:empty:only-child), #more-go ~ h2 ~ *),
#more-caveats:not([open]) ~ :not(h2, p:has(> a[id]:empty:only-child), #more-caveats ~ h2 ~ *),
#more-verify:not([open]) ~ :not(h2, p:has(> a[id]:empty:only-child), #more-verify ~ h2 ~ *),
#more-scope:not([open]) ~ :not(h2, p:has(> a[id]:empty:only-child), #more-scope ~ h2 ~ *),
#more-run:not([open]) ~ :not(h2, p:has(> a[id]:empty:only-child), #more-run ~ h2 ~ *),
#more-code:not([open]) ~ :not(h2, p:has(> a[id]:empty:only-child), #more-code ~ h2 ~ *),
#more-refs:not([open]) ~ :not(h2, p:has(> a[id]:empty:only-child), #more-refs ~ h2 ~ *) {
  display: none;
}
</style>
<!--kg-card-end: html-->


## Why I attach at creation and close by default

Running untrusted LLM-driven agents safely requires two distinct boundaries: compute isolation and deterministic network control. While gVisor runtime flags effectively isolate system calls from the host kernel, an agent executing arbitrary code or querying APIs still needs outbound connectivity. Without strict gating, that outbound access exposes internal subnets, local cloud metadata endpoints (such as 169.254.169.254), and peer containers to prompt-injection or SSRF attacks.

This post covers the architecture and Go implementation of a deterministic egress gate I built for gVisor sandboxes. The design principle is simple: attach the network interface at container creation, keep every packet dropped by default, and unlock access through an atomic verification pipeline. It builds on my previous [gVisor hardening post]({{link:gvisor-hardening}}), which focused strictly on runtime flags.



**Core premise.** The standard recipe for sandboxing network traffic is straightforward: create the container with --network none, configure the host interfaces, and run docker network connect once the environment is locked down.

In testing with gVisor's OCI runtime (runsc), this conventional approach broke down:
- Docker rejects network connect while --network none is still attached.
- If you execute disconnect none followed by connect, runsc fails to recognize the newly attached interface; inside the sandbox, only the loopback interface (lo) remains visible.
- gVisor initializes its network stack (netstack) during sandbox boot; dynamic interface hot-plugging is not reliably exposed to the guest.

To resolve this, I flipped the order:
1. The container is created on a dedicated bridge network, agents-net (172.30.0.0/24), ensuring the virtual interface exists at container start.
2. A low-overhead gate installed directly on the bridge drops all egress traffic before the guest process ever executes.
3. Outbound connectivity is unlocked only after the container is paused, the host-side firewall rules are applied, and the resulting state is verified via read-back.

## The three layers

No single packet filter is treated as an absolute boundary. The sandbox enforces three independent layers between the agent and external infrastructure:

**Per-port nftables table on the host veth:** Each container receives a dedicated table netdev as_port_<id> with ingress and egress chains bound to its host veth interface. Both chains enforce a strict drop policy until explicitly released. At release, its ingress accept rules also set the port mark.

**Bridge port controls and the bridge gate:** On the veth: isolated on, learning off, locked on, along with a static forwarding database (FDB) entry. On the bridge: a permanent neighbor table entry, and the agents_gate table, which passes a frame from the container only if its source MAC is in @released and the port mark is set.

**Layer 3/4 filtering in DOCKER-USER and INPUT:** Strict iptables rules, matched by container IP, limiting destination IPs and ports leaving agents-net or targeting the host.

The diagram shows how the three layers sit between the container and the outside. Click the boxes for details.

{{island:net-arch}}

<!--kg-card-begin: html-->
<details class="sec-more" id="more-layers"><summary><span class="sec-more-show">Show the full section</span><span class="sec-more-hide">Hide the section</span></summary></details>
<!--kg-card-end: html-->


### Layer 2: port controls

These are separate `bridge`/`ip` calls, applied in this order:

```
bridge link set dev <veth> isolated on
bridge link set dev <veth> learning off
bridge link set dev <veth> locked on
bridge fdb replace <mac> dev <veth> master static
ip neigh replace <ip> lladdr <mac> dev <bridge> nud permanent
```

- **isolated on:** Prevents the port from forwarding frames to other isolated ports on the same bridge, eliminating lateral container-to-container movement.
- **learning off:** Disables MAC learning; the bridge will not update its forwarding database based on incoming source MACs.
- **locked on:** Instructs the kernel to drop any frame from a source MAC that lacks an explicit static forwarding database entry.

The static FDB entry binds the container's MAC to that specific veth. The permanent neighbor entry on the bridge binds the container's IP to that MAC. Together they mitigate ARP poisoning or MAC spoofing attempts within the bridge domain.

The tool checks that /proc/sys/net/bridge/bridge-nf-call-iptables is set to 1 so bridged frames are evaluated by iptables rules. The tool validates this configuration but does not alter host-wide sysctl settings at runtime.

### Layer 3 and 4: iptables

Rules are injected into DOCKER-USER and INPUT. Each rule is tagged with the comment agent-net:<container> and uses -w to safely acquire the xtables lock:

- DNS: Restricted to UDP/TCP port 53 targeting explicit upstream resolvers (1.1.1.1, 9.9.9.9). (Note: In environments with high data-exfiltration sensitivity, point this instead to an internal inspecting DNS proxy to prevent DNS tunneling).
- Model API Endpoints: Restricted to TCP port 443 targeting resolved IPv4 addresses for approved services (e.g., api.anthropic.com, platform.claude.com). The host resolves these addresses during arming; the container has no control over destination routing.
- Default Deny: A final DROP rule for any traffic originating from the container IP that targets external or local networks.
- Host Protection (INPUT): Drops all container-initiated traffic directed at the host itself, allowing only established/related connection replies.
- Bridge Egress Control: An explicit rule tagged agent-net:gate drops bridged inter-port traffic between containers that have not completed the arming sequence.

The egress and input rules are IPv4 only. IPv6 frames fall to the policy drop of the nft chains. The endpoint names come from `/etc/agent-net/endpoints.json`, a root-owned installed copy that only root and the dedicated `refresh_endpoints` tool may write. I keep the source of the allowlist, `endpoints.json` on the Windows side next to the setup exes, off the host that runs the code it constrains: agent-host holds only the installed copy, for arming. When the installed file is absent, two built-in names apply (see the first caveat below).

## The arming sequence

create_agent_net arm <container> executes a linear, 13-step pipeline organized into five phases:

Click the boxes for details.

{{island:net-arming}}

1. Preconditions (Steps 1–5): Verify process capabilities (CAP_NET_ADMIN, PID 1 network namespace). Confirm Docker daemon connectivity. Record a cryptographic hash (SHA-256) of /etc/docker/daemon.json and the Docker engine start identity. Ensure agents-net and its gate exist, and that the container uses the runsc runtime, has restart policy exactly no, is connected solely to agents-net, and is running and paused.
2. Clean Slate (Step 6): Run the same name-keyed cleanup as disarm, so arm always builds from empty state. Whatever an earlier container or run of that name left (gate elements, port table, tagged iptables rules, neighbour and FDB entries) is removed first. A leftover that cannot be removed fails the arm.
3. Port Identification & Default-Deny (Steps 7–8): Extract container PID, IP, and MAC from Docker inspect data. Identify the host-side veth from host state: the host kernel's view of the container's network namespace, read as root through nsenter. Nothing reported by code inside the container is used. Cross-check the veth against the agent MAC, the container IP and the agents-net bridge master; any mismatch fails the arm. Evict the MAC from the gate's @released set and load the initial default-drop nftables port table using an atomic nft -f - transaction.
4. Host-Side Controls & Verification (Steps 9–10): Apply the bridge port flags, static FDB entries, and iptables rules. Immediately execute read-backs (bridge -j link show, nft list table, iptables -S) to confirm the kernel state matches the specification.
5. Atomic Release (Steps 11–13): Re-verify the container is still running and paused. In a single atomic nft -f - transaction, append the allow rules to the container's port table (the ingress ones set the port mark) and add its MAC to the bridge gate's @released set, with the container name as the element comment. Re-verify the daemon identity and configuration hash. If successful, signal completion so the orchestrator can unpause the container.

### Why I release last, and in one transaction

Opening the gate is an all-or-nothing operation. If any control fails or verification does not match during Steps 1–10, the container remains completely isolated.
If step 11 fails, the utility triggers a fallback routine (closeAgain) to ensure the MAC is stripped from @released, resets the port table to drop, and exits with code 2.
No rollback is attempted for steps 1–10: leaving an unreleased container behind closed, default-drop firewall rules is a safe failure state. The caller can safely destroy the paused container without traffic leaking.


## Edge cases and caveats

These are the limits I know about. I list them as my current view, not as a closed set.

- **Pinned model addresses.** Design choice, fails closed: the container never picks destinations, so a CDN rotation drops new connections until refreshed.
  - **Workaround:** `refresh_endpoints.exe` re-resolves and updates rules; `check-gate` reports drift.
- **No survival of a host restart.** Rules vanish on reboot or `wsl --shutdown`; a restarted container is dropped. I detect and remove, not prevent. 
  - **Workaround:** `--restart no`, recreate never restart; `agent_lifecycle guard` cleans up.
- **Tool versions.** Port locking needs recent iproute2 and kernel; minimums not pinned. 
- **Teardown order.** Disarm runs before container removal; a failed removal stops the run. 
  - **Workaround:** run it again.
- **Capabilities.** No `NET_ADMIN` by default; controls are host-side, so should hold if added.

**What to take away.**

- Attaching at creation and closing by default worked around what I observed with `runsc` and `docker network connect`.
- One atomic transaction opens a port, and only after read-back checks.
- The other controls stay in place after a failed arm. A closed port is the state I fail into.


## What this post leaves out

I name these so they are not read as covered:

- The gVisor runtime flags (my [hardening post]({{link:gvisor-hardening}})).
- IPv6 policy beyond the default drop.
- Automatic or scheduled refreshing of resolved endpoint addresses during a long run. Only the manual `refresh_endpoints.exe` command exists.
- Which model endpoints an agent should be allowed to reach. The built-in default is two names, the `endpoints.json` I supply at setup can change them, and I do not argue for either here.
- Published ports bound to `127.0.0.1`. That belongs to the container creation tool.
- Credentials and how the agent logs in to a model.

## Run it yourself
The accompanying code, the `agent-net` package, is a reference implementation of the closed-by-default gate in Go. Its `create_agent_net` command creates the `agents-net` network and its gate, and arms, checks and disarms one container on it. A second binary, `remove_agent_net`, takes the setup down. A third, `refresh_endpoints`, updates the pinned model addresses.

*Target Environment:* The commands run as root inside the `agent-host` distro and call `docker`, `ip`, `bridge`, `nft`, `iptables` and `nsenter` directly. They only read `daemon.json` (to hash it) and never write it, and they never reload or restart Docker. `bridge-nf-call-iptables` is read and required to be 1, never set.

*Full runbook:* the [chapter 1 runbook]({{link:howto-ch01}}) takes a bare Windows + WSL2 machine to a running agent container, including building the binaries, creating `agent-host`, and updating the model endpoints (its sections 6, 8 and 10 cover this post's network gate).
<!-- TODO(author): fill links.howto-ch01 (URL of howto/README_CH01.md at the code tag) before publish -->


<!--kg-card-begin: html-->
<div style="border-left:4px solid #b91c1c;background:#fef2f2;color:#7f1d1d;padding:12px 16px;margin:1em 0;border-radius:4px;">

**Run this evaluation harness strictly within an isolated, disposable environment.** This repository is an architectural reference pattern designed to benchmark isolation boundaries, not an off-the-shelf production system. Hardening an untrusted-code sandbox for enterprise multi-tenancy—including custom seccomp profiles, network egress filtering, and tenant quotas—requires environment-specific design.

</div>
<!--kg-card-end: html-->


## Code references

These are the places in the code where the closed-by-default decision shows up. I describe what each one does.

<!--kg-card-begin: html-->
<details class="sec-more" id="more-code"><summary><span class="sec-more-show">Show the full section</span><span class="sec-more-hide">Hide the section</span></summary></details>
<!--kg-card-end: html-->

In this post's package, `agent-net`:

- [sequence.go]({{link:code}}/internal/sequence/sequence.go) holds the numbered step lists: `createPipeline`, `armPipeline` (13 steps), `checkGatePipeline`, `disarmPipeline` and `removePipeline`. The first failing step sets the exit code, and there is no rollback.
- [config.go]({{link:code}}/internal/config/config.go) holds the fixed names as constants: the network and its subnet, the gate table `agents_gate` and its set `released`, the per-port table prefix `as_port_`, and the port mark bit `PortMark`.
- [gate.go]({{link:code}}/internal/services/gate.go) holds `LoadGate`, which installs the gate on the bridge device in one atomic `nft -f -` with policy drop on both chains. It also holds `GateRuleText`, the one rule of each gate chain, and `GateElementText`, the set element that carries the container name as its comment.
- [s36_install_gate.go]({{link:code}}/internal/steps/s36_install_gate.go) holds `InstallGate`, the create step that installs the gate right after the network exists, before the create run verifies it.
- [s09_require_network.go]({{link:code}}/internal/steps/s09_require_network.go) holds `RequireNetwork`. Arm uses it to refuse a network whose gate is missing or whose `bridge-nf-call-iptables` is not 1, with exit 1 (`GATED_UNARMED`). Check-gate runs it too, but reports a missing gate as `inconsistent` instead.
- [s34_require_paused.go]({{link:code}}/internal/steps/s34_require_paused.go) holds `RequirePaused`, which reads the container and requires `agents-net` only, the `runsc` runtime, restart policy `no`, and running and paused.
- [s40_clean_slate.go]({{link:code}}/internal/steps/s40_clean_slate.go) holds `CleanSlate`, which runs the name-keyed disarm steps in-process so that arm starts from empty state.
- [s10_close_port.go]({{link:code}}/internal/steps/s10_close_port.go) holds `ClosePort`. It takes the MAC out of `released`, loads the closed port table, reads it back and refuses a table that is bound to another container's live veth.
- [nft.go]({{link:code}}/internal/services/nft.go) holds `LoadClosedPort` and `ReleasePort`. `ReleasePort` flushes the two chains, adds the accept rules and adds the MAC to `released` in one transaction. `AcceptRules` builds those rules, and the ingress ones set the port mark.
- [s15_release_port.go]({{link:code}}/internal/steps/s15_release_port.go) holds `ReleasePort`, the only step that opens a port. It re-checks that the container is paused, reads everything back, and calls `closeAgain` on any failure after the release.
- [s17_check_gate.go]({{link:code}}/internal/steps/s17_check_gate.go) holds `CheckInspect` and `CheckPortControls`, the read-only report that ends in `closed`, `released` or `inconsistent`.
- [s37_disarm_close_gate.go]({{link:code}}/internal/steps/s37_disarm_close_gate.go) holds `DisarmCloseGate`, the first deletion of a disarm: the gate closes the port before anything else of it is removed.
- [sequence_test.go]({{link:code}}/tests/sequence_test.go) holds `TestCreateInstallsAndVerifiesTheGate`, `TestArmSuccessReleasesPortLast`, `TestArmReleaseAddsTheMACToTheGateInTheSameTransaction`, `TestArmRefusesWhenTheGateIsMissing` and the `TestCheckGate...` tests.
- [disarm_remove_test.go]({{link:code}}/tests/disarm_remove_test.go) holds `TestDisarmRemovesTheMACFromTheGateBeforeTheTable` and `TestDisarmVerifiesTheGateSet`.

According to the package README, `create_agent_container` and `remove_agent_container` of the `agent-sandbox` package call `arm` and `disarm`, and its guard uses `released`. I give no link for that package, because it has its own tag.


## References

- Source of the facts in this post: the package README and code of `agent-net` in [the code tag]({{link:code}}).
- nftables documentation: the `netdev` family, ingress and egress hooks, and sets.
- Linux `bridge(8)` manual page: port flags `isolated`, `learning` and `locked`, and forwarding database entries.
- Docker documentation: the `DOCKER-USER` chain and user-defined bridge networks.

<!--kg-card-begin: html-->
<details class="sec-more" id="more-refs"><summary><span class="sec-more-show">Show the full section</span><span class="sec-more-hide">Hide the section</span></summary></details>
<!--kg-card-end: html-->

To reproduce, clone the tag once it is cut. The tag and the 40-hex commit sha are added at release.

```
git clone --branch <code_tag> https://github.com/vectorNvolt/vectorNvolt-blog
```
