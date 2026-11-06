# agent-net (`create_agent_net`)

Go CLI with four forms (plus a second binary, `remove_agent_net`), all run as root inside the `agent-host` distro (docker, `ip`, `bridge`, `nft` and `iptables` are called directly, no `wsl.exe` prefix):

- `create_agent_net` (no arguments) creates the shared bridge network `agents-net` (`172.30.0.0/24`, not `--internal`), installs its gate (below) and verifies both.
- `create_agent_net arm <container>` is called by `create_agent_container` (`/usr/local/bin/create_agent_net arm <container>`) for a container that was created with `--network agents-net --restart no`, started and paused. It attaches nothing: it requires the gate, loads the container's closed port table, applies the host-side controls, verifies them and only then releases the port (agent MAC into the gate set `released` plus the accept rules), with one atomic `nft -f`.
- `create_agent_net disarm <container>` is the inverse of `arm` for one container (also called by `remove_agent_container`): it removes the agent MAC from `released`, deletes the container's port table, then its tagged iptables rules, then its static FDB entry.
- `create_agent_net check-gate <container>` is a read-only report of the container's port controls and of the gate. It changes nothing.
- `create_agent_net released` (read only) prints one line per element of the gate set `released`: `<mac> <comment>` (the comment is the container name; empty for an untagged element, so the line is `<mac> `). Used by the guard of `agent-sandbox`.
- `create_agent_net evict <mac>` removes one element of `released` by MAC (an untagged or orphaned element). Nothing else of the gate is touched; an absent element is not an error. Pipelines: `releasedPipeline`, `evictPipeline` (steps `s41`, `s42`; privilege checks as the other forms). Exit codes as below: 0 ok, 1 usage or precondition (`NOT_CHECKED` / `NOT_EVICTED`), 2 failure (`EVICT_INCOMPLETE` for evict).
- `remove_agent_net` (second binary, no arguments) removes every agent container on `agents-net` (including created-but-never-armed ones), any leftover `as_port_*` table, the `agents_gate` table, any leftover `agent-net:` rule and the network.

Any other invocation prints usage on stderr and exits 1. The tool only reads `daemon.json` (to hash it), never writes it, and never reloads or restarts docker. Spec: `design/02_use_case/usc-004_create_agent_net.yml`, `usc-007_remove_agent_net.yml`, `design/03_sys_req/agent-net/`, `design/01_user_story/uss-004_create_agent_net.md`.

## The agents-net gate

Containers are created on `agents-net` (`--network agents-net`), not on `none` and connected later: on the real host (2026-10-31) Docker rejects `network connect` while `none` is attached, and after `disconnect none` + `connect` gVisor/`runsc` still shows only `lo` (it never sees interfaces added after the start). "No reachability until armed" is therefore enforced network-wide by a gate installed when `agents-net` is created (SFR-NET-025, SSR-NET-005):

- nft table `netdev agents_gate` bound to the bridge device `br-<id>`: ingress and egress chains, policy drop, accepting only frames whose source (ingress) or destination (egress) MAC is in the set `released`. Covers container to host, DNS, routed egress and host to container.
- iptables rule in `DOCKER-USER`, `-i <br> -o <br> -m physdev --physdev-is-bridged -j DROP`, comment `agent-net:gate`: blocks port-to-port traffic between unarmed containers.
- `bridge-nf-call-iptables` must be 1. Create and arm read it and require it; they never set it (host configuration is not changed).
- The nftables bridge family is not supported by the agent-host kernel and `ebtables` is not used.
- Release adds the agent MAC to `released` in the same nft transaction that opens the per-port table, as `{ <mac> comment "<container>" }`; a failure after the release removes it again. Disarm removes it by comment and by MAC. Element comments need nft >= 0.9.1 (the host has 1.0.9) and kernel set element userdata. The set is read as JSON (`nft -j list set netdev agents_gate released`) to see the comments. `check-gate`: `closed` = MAC not in `released`, `released` = MAC in `released`, anything else (a missing gate table included) = `inconsistent`.
- Port mark (`config.PortMark`, bit 28, `0x10000000`): both accept rules of the port table's ingress chain also run `meta mark set meta mark or 0x10000000` (other mark bits kept). The gate's ingress rule is `ether saddr @released meta mark and 0x10000000 == 0x10000000 meta mark set meta mark and 0xefffffff accept`: it requires the bit and clears it again so it never reaches host routing or iptables. The gate egress rule (`ether daddr @released accept`), the closed port table and the port egress chain are unchanged. Why: a restarted container gets a new veth with no port table, so its frames carry no mark and the gate drops them even if its MAC is still in `released`. The bit is far from the low bits used by Docker, kube-proxy (`0x4000`/`0x8000`), firewalld and Cilium defaults. A gate (or port rule) without the mark is a mismatch: `arm` refuses such a network, `check-gate` reports `agents-net gate` / `accept rules` as `mismatch`; recreate the network (`remove_agent_net`, `create_agent_net`). Probed on 6.6.87.2-microsoft-standard-WSL2 with nft 1.0.9 (a mark set at the veth's netdev ingress is still present at the bridge's netdev ingress); not yet verified on a live agent-host. The read-back matches tokens (`meta mark set`, `0x10000000`, `0xefffffff`), not the exact `nft list` text.
- An `agents-net` created before the gate existed must be recreated once: `remove_agent_net`, then `create_agent_net`. `arm` refuses such a network (`GATED_UNARMED`, exit 1).

## Steps

Keep in sync with `internal/steps/sNN_*.go`. The first failing step stops the run (exception: remove steps 7-10, see below); nothing success-like is printed on stdout after a failure. There is no rollback in any flow. File numbers identify a step; the arm flow runs `s12` (identify the veth) before `s10` (load the port table), because the table can only be loaded once the veth exists.

Create flow (`sequence.createPipeline`, also selected for an invalid invocation so that step 1 prints the usage):

| Step | File | Requirement | Exit code on failure | Purpose |
|---|---|---|---|---|
| 1 | `s01_validate_invocation.go` | SFR-NET-001 | 1 | Accept exactly no arguments, `arm <container>`, `disarm <container>` or `check-gate <container>`; anything else prints usage on stderr with the not-done state of the first argument's mode (`GATED_UNARMED` / `NOT_DISARMED` / `NOT_CHECKED`, else `NOT_CREATED`). The container name `gate` is rejected: its rule tag `agent-net:gate` would collide with the tag of the network gate rule. |
| 2 | `s02_check_privilege_engine.go` | SFR-NET-002 | 1 | euid 0, `CAP_NET_ADMIN` (bit 12 of `CapEff`), the network namespace is the same as pid 1's (`/proc/self/ns/net` vs `/proc/1/ns/net`), access to the docker socket, `docker version` answers; each missing item is named. The docker CLI runs with `-H unix://<checked socket>`. |
| 3 | `s03_refuse_existing.go` | SFR-NET-003 | 1 | `docker network inspect agents-net` must not find the network; an existing network is named and not touched. Absent counts only on a not-found answer; any other docker error stops the run. |
| 4 | `s04_check_range_clash.go` | SFR-NET-004 | 1 | `172.30.0.0/24` must not overlap any docker network subnet, host address (`ip -j addr`) or host route (`ip -j route show table all`); default routes are ignored; every clash is named. |
| 5 | `s05_record_engine_state.go` | SSR-NET-001, SFR-NET-006 | 1 | Record the SHA-256 of `daemon.json` (absent is a state) and the engine start identity. |
| 6 | `s06_create_network.go` | SFR-NET-005 | 2 | `docker network create --driver bridge --subnet 172.30.0.0/24 agents-net` (no `--internal`: the network keeps its NAT route out). No resolver file is written (`create_agent_container` mounts the host file `/etc/agent-sandbox/resolv.conf` read-only at `/etc/resolv.conf` and verifies it). The error is reported (`CREATE_FAILED`). |
| 7 | `s36_install_gate.go` | SFR-NET-025 | 2 | Install the gate on the existing network: `bridge-nf-call-iptables` must be 1 (read only, checked here because the network and its bridge exist now; if not, the network stays and the run fails with `CREATED_UNVERIFIED`, control `network gate`), then table `agents_gate` on the bridge device and the `agent-net:gate` rule in `DOCKER-USER`. |
| 8 | `s07_verify_network.go` | SFR-NET-006, SFR-NET-025, SSR-NET-001 | 2 | Re-inspect (exists, driver `bridge`, `Internal` false, subnet), read the gate back (table, both chains bound to the bridge device with policy drop, empty set `released`, tagged rule), compare `daemon.json` hash and engine identity; failed checks are listed, the network stays (`CREATED_UNVERIFIED`). |
| 9 | `s08_report.go` | SFR-NET-007 | none (exit 0) | Success report on stdout; says that `arm`, `disarm` and `check-gate` are available. |

Arm flow (`sequence.armPipeline`, 13 steps in five phases: preconditions 1-5, clean slate 6, identify and close 7-8, controls and read-back 9-10, release 11-13; steps 1 and 2 (validate, privilege) are shared and not listed in the table). `RecordEngineState` (step 3) and `VerifyEngineUnchanged` (step 12) are in every pipeline (SSR-NET-001): a changed `daemon.json` hash or engine start identity fails the run with exit 2 (arm: failed control `engine state`).

| Step | File | Requirement | Exit code on failure | Purpose |
|---|---|---|---|---|
| 3 | `s05_record_engine_state.go` | SSR-NET-001 | 1 | Record the `daemon.json` hash and engine start identity. |
| 4 | `s09_require_network.go` | SFR-NET-008 | 1 | `agents-net` must exist; bridge device taken from `com.docker.network.bridge.name` or `br-<first 12 of id>`. The gate (`agents_gate` table) must be present and `bridge-nf-call-iptables` must be 1 (read, never set); otherwise refuse with `GATED_UNARMED` and say the network predates the gate and must be recreated. |
| 5 | `s34_require_paused.go` (`RequirePaused`) | SFR-NET-008 | 1 | The container must exist, be attached to `agents-net` and to no other network, have runtime `runsc` (`config.SandboxRuntime`), have restart policy exactly `no` (an empty policy is refused: the sandbox passes `--restart no`), and be running and paused. Reads only. |
| 6 | `s40_clean_slate.go` (`CleanSlate`) | SFR-NET-008 | 1 or 2 | Arm builds from empty state: runs the name-keyed disarm (steps `s19`..`s23`, `s37`, see Disarm) in-process before anything is identified or closed. A leftover that cannot be removed fails arm (control `clean slate`, `GATED_UNARMED`); a port table bound to the live veth of another container is refused (exit 1). |
| 7 | `s12_identify_port.go` | SFR-NET-008, SSR-NET-002 | 2 | PID and IP/MAC from `docker container inspect`; the interface with that MAC and IP is found with the host's `ip` through `nsenter -t <pid> -n`; its `link_index` is mapped to the host veth via `ip -j link`, whose master must be the bridge. |
| 8 | `s10_close_port.go` | SFR-NET-008 | 2 | First remove the agent MAC from `released` (an earlier run may have left it there; a re-arm must not inherit an open gate). Then load `table netdev as_port_<id>` in one atomic `nft -f -`: ingress and egress chains bound to the veth, both `policy drop`, no accept rule; an older table of that name is replaced. Read back. If `as_port_<id>` is bound to another live veth, arm refuses (exit 1) and loads nothing; a table bound to a veth that no longer exists is replaced. |
| 9 | `s13_apply_controls.go` | SFR-NET-009, SSR-NET-002 | 2 | `isolated on`, `learning off`, `locked on`, static FDB entry, permanent neighbour entry, `DOCKER-USER` egress allow-list and default-deny, `INPUT` default-deny. Before any control: a PERMANENT neighbour entry for the port IP on the bridge with another `lladdr` than the port MAC fails arm (control `neighbour entry`) when a live `agents-net` endpoint owns that `lladdr`, otherwise it is deleted and arm continues. The source identity accept rules are not added here. Re-checks running and paused afterwards. |
| 10 | `s14_verify_controls.go` | SFR-NET-010 | 2 | Read each applied state back (`bridge -j link/fdb show`, `nft list table`, `iptables -S`). The agent MAC must still be absent from `released` before the release (control `gate release set`). |
| 11 | `s15_release_port.go` | SFR-NET-010, SFR-NET-024 | 2 | Re-check running and paused, add the agent MAC to `released` of `agents_gate` and the accept rules (below) in ONE atomic `nft -f -` (the only operation that opens a port, always last), read them back and verify again. If the release or its read-back fails, the MAC is removed from `released` and the port is loaded closed again (`closeAgain`). |
| 12 | `s35_verify_engine_unchanged.go` | SSR-NET-001 | 2 | Compare `daemon.json` hash and engine identity with step 3. |
| 13 | `s16_report_armed.go` | SFR-NET-010 | none (exit 0) | Success on stdout; the caller then unpauses the container. |

SFR-NET-011: any arm failure prints `failed control: <name>: <detail>` as the first stderr line (names: `port identification`, `port table`, `container paused`, `port isolation`, `mac learning off`, `port lock`, `static FDB entry`, `neighbour entry`, `DNS egress rule`, `endpoint egress rule`, `egress default-deny`, `host-input default-deny`, `gate release set`, `port release`, `engine state`; the verification names the rule's own control, the DNS rule and the endpoint rule are distinct controls), reports exit state `GATED_UNARMED`, exits 2 and leaves the already applied controls in place; there is no rollback. The container's table never has accept rules, and its MAC is not in `released`, after a failure: if the release read-back fails the MAC is removed and the port is reloaded closed (`closeAgain` in `s15`). If identifying the host interface fails (step 7), no table exists yet and the container stays attached (it was created on `agents-net`), paused and closed by the gate. The caller removes the still paused container. A precondition failure (steps 1-5, a missing gate, or a port table bound to another live veth) exits 1.

Check-gate flow (`sequence.checkGatePipeline`, 7 steps, SFR-NET-021): steps 1-4 as in arm (validate, privilege, record engine state, require network), then

| Step | File | Exit code on failure | Purpose |
|---|---|---|---|
| 5 | `s17_check_gate.go` (`CheckInspect`) | 1 (`NOT_CHECKED`) | The container must exist. |
| 6 | `s17_check_gate.go` (`CheckPortControls`) | 2 (`GATE_INCONSISTENT`) | Find the live veth as in arm (if there is none, port flags, FDB entry and neighbour entry report `missing`, and the container IP is taken from the tagged rules), read everything, print the report. A missing iptables chain counts as empty. |
| 7 | `s35_verify_engine_unchanged.go` | 2 (`GATE_INCONSISTENT`) | Compare `daemon.json` hash and engine identity with step 3. |

Report on stdout, one line per control (the labels of SFR-NET-021; `<id>` is the container's id, e.g. `table as_port_web-1`), every control checked even after a failure, then the summary line. stdout carries nothing else: the `[n/m]` step lines and the `DEBUG` lines of this form go to stderr.

```
table as_port_<id>: ok|missing|mismatch
chain policies: ok|missing|mismatch
accept rules: ok|missing|mismatch
agents-net gate: ok|missing|mismatch
gate release: ok|missing|mismatch
port flags: ok|missing|mismatch
static FDB entry: ok|missing|mismatch
neighbour entry: ok|missing|mismatch
DNS egress rule: ok|missing|mismatch
endpoint egress rule: ok|missing|mismatch
other tagged iptables rules: ok|missing|mismatch
container confinement: ok|missing|mismatch
closed|released|inconsistent
```

`container confinement` is `ok` when the container has restart policy exactly `no`, is attached to `agents-net` only, and is running. It is not required to be unpaused (`create_agent_container` runs check-gate while the container is still paused).

`accept rules` is `ok` when the table has no accept rule, or has exactly the accept rules of a released port (anything else is `mismatch`). `agents-net gate` covers the `agents_gate` table (set, chains bound to the bridge device, policy drop), the `agent-net:gate` rule and `bridge-nf-call-iptables`; it is `missing` when the table is absent. `gate release` compares the agent MAC with the set `released` and the port table: `ok` when the table has no accept rule and the MAC is not in `released`, or when it has accept rules and the MAC is in `released`; `mismatch` for the other combinations (or an unknown MAC with accept rules); `missing` when the gate table is not available. The summary is `closed` (all controls ok, no accept rules), `released` (all controls ok, verified accept rules) or `inconsistent` (any control not ok, a missing `agents_gate` table included). The three iptables controls use the same rule plan as `arm` (`iptPlan` in `s13_apply_controls.go`: destination, protocol, port, `-s <ip>`, `ESTABLISHED,RELATED` with the source, default-deny with the source, comment `agent-net:<container>`). `DNS egress rule`: udp+tcp 53 to `1.1.1.1` and `9.9.9.9`, and no other DNS accept. `endpoint egress rule`: tcp 443 to the addresses `api.anthropic.com` and `platform.claude.com` resolve to now (resolved again on the host as in arm; an address that changed since arm shows as a mismatch), and no other 443 accept. `other tagged iptables rules`: the `DOCKER-USER` default-deny and the `INPUT` replies accept and default-deny. A control is `missing` when no rule of its kind is there and `mismatch` when a rule of its kind exists with another destination, source or port (or a wanted rule is absent). The resolver mount is not part of check-gate. Exit 0: every control `ok`. Exit 1, `NOT_CHECKED`: invalid invocation, unknown container, no privilege, no engine, no `agents-net`. Exit 2, `GATE_INCONSISTENT`: a control is `missing` or `mismatch`, or a read-back failed; the failed controls are repeated on stderr. Strictly read-only (no `nft -f`, no `delete`, no `iptables -I/-D`, no `bridge ... set`).

## Controls (arm)

| Control | Applied with | Verified by |
|---|---|---|
| Port table | `nft -f -`: `table netdev as_port_<id>`, `chain ingress { type filter hook ingress device "<veth>" ...; policy drop; }`, `chain egress { ... hook egress ...; policy drop; }` | `nft list table netdev as_port_<id>`: both chains on their hook, bound to the veth, `policy drop`, no rule |
| Port isolation | `bridge link set dev <veth> isolated on` | `bridge -j link show dev <veth>`: `isolated` true, master is the bridge |
| Learning off | `bridge link set dev <veth> learning off` | same output: `learning` false |
| Kernel MAC lock | `bridge link set dev <veth> locked on` (never unlocked) | same output: `locked` true |
| Fixed identity | `bridge fdb replace <mac> dev <veth> master static` (`<mac>` = MAC docker assigned on agents-net) | `bridge -j fdb show dev <veth>`: entry with `<mac>`, state `static`/`permanent` |
| Neighbour entry (SFR-NET-009/010/024) | `ip neigh replace <ip> lladdr <mac> dev <bridge> nud permanent` | `ip -j -4 neigh show <ip> dev <bridge>`: `lladdr` is `<mac>`, state `PERMANENT` |
| DNS egress rule | `iptables -I DOCKER-USER`: accept `-s <ip>` to the fixed resolvers `1.1.1.1` and `9.9.9.9` (udp+tcp 53) | `iptables -S DOCKER-USER` |
| Endpoint egress rule | `iptables -I DOCKER-USER`: accept `-s <ip>` to the resolved addresses of `api.anthropic.com` and `platform.claude.com` (tcp 443; names resolved host-side through the fixed resolvers, not the host's `/etc/resolv.conf`) | `iptables -S DOCKER-USER` |
| Egress default-deny | `iptables -I DOCKER-USER`: drop `-s <ip> ! -o <bridge>` (below the two accepts above) | `iptables -S DOCKER-USER` |
| Host-input default-deny | `iptables -I INPUT`: accept `-s <ip>` ESTABLISHED,RELATED, then drop `-s <ip>`; no DNS accept (the resolvers are external) | `iptables -S INPUT` |

Release (the last operation, one atomic `nft -f -` that first flushes both chains, so a repeat leaves exactly these rules). `<bridge MAC>` is the address of the `agents-net` bridge interface in `ip -j link`, `<bridge IP>` its IPAM gateway:

| Chain | Rule | Layer |
|---|---|---|
| ingress | `ether saddr <mac> ether daddr <bridge MAC> ip saddr <ip> accept` | 2 + 3 |
| ingress | `ether saddr <mac> ether daddr ff:ff:ff:ff:ff:ff arp operation request arp saddr ether <mac> arp saddr ip <ip> arp daddr ip <bridge IP> accept` | 2 + 3 |
| egress | `ether saddr <bridge MAC> ether type ip accept` | 3 |
| egress | `ether saddr <bridge MAC> ether daddr <mac> arp operation reply accept` | 2 + 3 |

Everything else, including IPv6 and any other frame type, falls to the policy drop of the two chains. Every iptables rule carries the comment `agent-net:<container>`; nft tables are owned by the name `as_port_<id>` (`<id>` = container name without the `agent-` prefix). Existing iptables rules are detected and not added twice. Every iptables call uses `-w` (wait for the xtables lock). Published ports bound to `127.0.0.1` is the concern of the container creation tool; this unit does not check it.

## Per-port table (design decision, 2026-10-30)

The WSL2 kernel (6.6.87.2-microsoft-standard-WSL2, shared by all distros) has no nftables bridge family, but the `netdev` family with both hooks works, so each port gets its own netdev table. Requirements: SFR-NET-008..011 in `design/03_sys_req/agent-net/`.

- A new bridge port starts open (`learning on`, `isolated off`, `locked off`), and a netdev chain can bind only to a veth that exists. The window between "veth appears" and "table loaded" is closed by the `agents_gate` table on the bridge device (the MAC of the new container is not in `released`) and by the pause: the new container is paused for the whole arm run, and already armed agents may send only to the bridge MAC. `arm` checks that the container is running and paused (`RequirePaused`, step 5) and re-checks after the controls and before the release.
- Each port has its own table. Arm and disarm never touch another port's table or rules; there is no shared gate to rebuild.
- The tables and iptables rules do not survive `wsl --shutdown` or a reboot (`agents-net` does). `create_agent_container` creates agents with `--restart no`; operator rule: never `docker start` an old agent container, remove it and recreate it. After a docker start, `agent_lifecycle restore` (`agent-restore.service`, `packages/agent-sandbox`) removes the old agent containers, runs `remove_agent_net` then `create_agent_net` and writes the ready marker; `agent_lifecycle guard` removes an agent container started outside the tools. Per-port controls stay bound to the veth (no nft bridge family in this kernel), so a restarted container is replaced, never repaired.

## Disarm (`create_agent_net disarm <container>`)

Spec: `design/02_use_case/usc-004_create_agent_net.yml` (alt-2), SFR-NET-012, SSR-NET-003. `sequence.disarmPipeline`, 12 steps; steps 1-3 (validate, privilege, record engine state) are shared with the other forms. Steps 5-10 (`DisarmInspect`..`DisarmVerify`) run unchanged per container inside `remove_agent_net`.

| Step | File | Exit code on failure | Purpose |
|---|---|---|---|
| 4 | `s18_disarm_require_network.go` | 1 (`NOT_DISARMED`) | Without `agents-net` there is nothing to disarm: report and exit 0. Otherwise read the bridge device. |
| 5 | `s19_disarm_inspect.go` | 1 (`NOT_DISARMED`) | PID, IP and MAC from docker. A container that does not exist is not a failure (name-keyed cleanup continues); inside `remove_agent_net` it is skipped as already gone. A docker error other than not-found stops the run. |
| 6 | `s20_disarm_find_ports.go` | 2 (`DISARM_INCOMPLETE`) | Find the port: the live veth (a failure to identify it is not fatal) and the veth the table `as_port_<id>` is bound to; the IPs of the tagged iptables rules add to the identity. The container's MAC is recovered from the accept rules of the port table (docker reports none for a stopped container). Changes nothing. |
| 7 | `s37_disarm_close_gate.go` | 2 (`DISARM_INCOMPLETE`) | Remove the agent MAC from `released` of `agents_gate` (the gate closes the port first; an absent gate table or entry is not an error). |
| 8 | `s21_disarm_delete_table.go` | 2 (`DISARM_INCOMPLETE`) | `nft delete table netdev as_port_<id>` (order: released entry, table, iptables rules, FDB entry). |
| 9 | `s22_disarm_remove_rules.go` | 2 (`DISARM_INCOMPLETE`) | Delete the rules in `DOCKER-USER` and `INPUT` whose comment is exactly `agent-net:<container>`, then the neighbour entry of each known container IP on the bridge (`ip neigh del`, only when `ip neigh show` finds it and its `lladdr` is the container's MAC, or it is PERMANENT and its `lladdr` belongs to no live `agents-net` endpoint, i.e. left by an uncleanly disarmed container; an entry owned by another live endpoint stays; live endpoint MACs = `MacAddress` of the containers on `agents-net` in `docker network inspect`, the container being removed excluded), then the static FDB entry of a veth that still exists. `isolated on`, `locked on` and `learning off` stay: the port stays closed. |
| 10 | `s23_disarm_verify.go` | 2 (`DISARM_INCOMPLETE`) | Read back: no table, tagged rule, neighbour entry or FDB entry of the container is left; leftovers are listed. A neighbour entry counts as left when its `lladdr` is the container's MAC, or it is PERMANENT and owned by no live endpoint (`neighbour entry of <ip> (lladdr <mac>) on <bridge> is still there`). |
| 11 | `s35_verify_engine_unchanged.go` | 2 (`DISARM_INCOMPLETE`) | Compare `daemon.json` hash and engine identity with step 3. |
| 12 | `s24_report_disarmed.go` | none (exit 0) | `disarmed (N tables, rules and entries removed)` or `nothing to disarm`. |

Disarm is name-keyed and idempotent; it works for a running, a stopped and a gone container (a container that does not exist is no longer an error: `Missing`, the later steps still run). It collects the container's MACs from docker, from the accept rules of its port table and from the gate set elements whose comment is the container name; the IPs from docker and from its tagged rules. It then removes every `released` element with that comment or one of those MACs (set elements only: the `agents_gate` table, its chains and the `agent-net:gate` rule are never touched), deletes `as_port_<id>`, deletes every iptables rule tagged `agent-net:<container>` whatever its IP, deletes the neighbour entries of the container's IPs (never one owned by another live endpoint) and the static FDB entries of its MACs on every port of the bridge. Step 10 verifies: no element with the comment or a collected MAC, no table, no tagged rule, no neighbour or FDB entry. A port table bound to the live veth of a different running container (`agent-web` vs `web`) is refused (exit 1, `NOT_DISARMED`) before anything is changed.

Only the named container's table and rules are touched. Absent tables, rules and entries are not an error, so the run is safe to repeat. Exit codes: 0 disarmed or nothing to disarm, 1 usage or precondition, 2 runtime (run it again).

## Remove (`remove_agent_net`)

Spec: `design/01_user_story/uss-007_remove_agent_net.md`, `design/02_use_case/usc-007_remove_agent_net.yml`, SFR-NET-013..019, 022, 023, SSR-NET-003. Takes no arguments; any argument prints usage and exits 1. `sequence.removePipeline`, 13 steps. There is no rollback.

| Step | File | Exit code on failure | Purpose |
|---|---|---|---|
| 1 | `s25_validate_remove.go` | 1 (`NOT_REMOVED`) | No arguments. |
| 2 | `s02_check_privilege_engine.go` | 1 (`NOT_REMOVED`) | Shared with the other forms. |
| 3 | `s05_record_engine_state.go` | 1 (`NOT_REMOVED`) | Record the `daemon.json` hash and engine start identity. |
| 4 | `s26_survey_removal.go` | 1 (`NOT_REMOVED`) | Read the network, the `as_port_*` tables and the `agents_gate` table (`nft list tables`) and the tagged iptables rules (a missing chain counts as empty); an unreadable part stops the run with nothing changed. If none exists: report `nothing to remove` and exit 0. |
| 5 | `s27_refuse_foreign.go` | 1 (`NOT_REMOVED`) | List the containers attached to `agents-net`, stopped and created-but-never-armed ones included (containers are attached at creation). If any is not named `agent-<id>`, refuse, list them and change nothing. |
| 6 | `s28_remove_agents.go` | 2 (`REMOVE_INCOMPLETE`) | Per agent container: disarm in-process, then `docker rm -f`. The first failure stops the run and names the container and those still to remove. Workspaces are never touched. |
| 7 | `s29_remove_port_tables.go` | recorded | Delete every leftover `as_port_*` table by name; other nft tables are not touched. |
| 8 | `s30_remove_leftover_rules.go` | recorded | Delete any iptables rule with a comment starting `agent-net:` (the `agent-net:gate` rule included) left in `DOCKER-USER` and `INPUT`. |
| 9 | `s38_remove_gate.go` | recorded | Delete the `agents_gate` table (SFR-NET-026, with its set `released`), when it exists. |
| 10 | `s31_remove_network.go` | recorded | `docker network rm agents-net`, when it exists. |
| 11 | `s32_verify_removed.go` | 2 (`REMOVE_INCOMPLETE`) | Read back: network absent, no `as_port_*` table, no `agent-net:` rule; leftovers and the failures recorded by steps 7-10 are listed. |
| 12 | `s35_verify_engine_unchanged.go` | 2 (`REMOVE_INCOMPLETE`) | Compare `daemon.json` hash and engine identity with step 3. |
| 13 | `s33_report_removed.go` | none (exit 0) | Success on stdout (number of removed containers, workspaces kept). |

Steps 7-10 record a failure and let the later steps run; step 11 reports it (SFR-NET-015/016). Order guarantee (SSR-NET-003): all agent containers are disarmed and removed (step 6) before any leftover table is deleted (step 7); if step 6 fails the tables and rules of the containers still there stay. A port is never unlocked or de-isolated.

Re-running after a partial removal finishes it: every step tolerates what is already absent and removes only what is there.

Exit states: `NOT_REMOVED` (exit 1, nothing changed), `REMOVE_INCOMPLETE` (exit 2, part of the setup is left; run `remove_agent_net` again). Exit 0 covers both a full removal and `nothing to remove`.

## Open points and proposed defaults

- Verified on the real host (2026-10-31): `docker network connect` is rejected while `none` is attached, and gVisor never sees interfaces added after the start, hence the attachment at creation and the gate. Netdev hooks on the bridge device, `br_netfilter` and the iptables `physdev` match work; the nftables bridge family does not. Unverified on the real host: `docker pause` with `runsc`; the real `nft list table` text (rule matching is by tokens, not exact text); `iptables -S` comment quoting; `bridge link set ... locked on` needs a recent iproute2.
- Engine start identity (SSR-NET-001): pid from `/var/run/docker.pid` plus the start time (field 22) of `/proc/<pid>/stat`. It detects a restart; a SIGHUP reload keeps the same process and is only detected through the `daemon.json` hash.
- Allowed destinations: DNS resolvers = the fixed `1.1.1.1` and `9.9.9.9` (udp+tcp 53); egress targets = the fixed `api.anthropic.com` and `platform.claude.com` (no override; each resolved host-side at arm time to IPv4 addresses, tcp 443). Host-side resolution (`arm`, `check-gate`, `refresh_endpoints`) does not use the host's `/etc/resolv.conf`: it queries `1.1.1.1` then `9.9.9.9` directly on port 53, 5 s timeout per resolver, failover to the second; if both fail, the error names each resolver and says port 53 to them must be reachable from the agent host. Other host tools use the host's own DNS, which `create_agent_host` points at the same resolvers through a static `/etc/resolv.conf`. The resolved IPs are fixed at arm time and are not refreshed.
- Egress and input rules are IPv4 only. There is no ingress ARP reply rule: the host's neighbour entry for the agent is permanent (`ip neigh replace <ip> lladdr <mac> dev <bridge> nud permanent`, control `neighbour entry`), so the host never sends an ARP request to the agent, and the egress ARP rule only lets the bridge answer the agent's own request (`arp operation reply`, to the agent MAC). A stale permanent entry for the IP with another `lladdr` is deleted by `arm` first, unless a live endpoint owns that `lladdr` (then arm fails). The entry is deleted by `disarm` (after the iptables rules, before the FDB entry). Entries of removed containers disappear with the bridge in `remove_agent_net`, so the leftover steps (7-10) do not enumerate them.
- The agent-sandbox gives the container `SYS_ADMIN` but not `NET_ADMIN` by default (it may be added later). The controls are on the host side of the veth in any case, so they hold if `NET_ADMIN` is added; but the veth is identified through the peer index read in the container namespace (as the design allows), so a hostile container could try to influence that value; the step cross-checks MAC, IP and the bridge master.
- `arm` exit codes: 0 port released, 1 usage or precondition (steps 1-5, or a port table bound to another live veth), 2 apply or verify failure.
- The first failing control stops the run; later controls are neither applied nor verified.

## How to build / run

```
GOOS=linux GOARCH=amd64 go build -o create_agent_net ./cmd/create_agent_net
```

```
GOOS=linux GOARCH=amd64 go build -o remove_agent_net ./cmd/remove_agent_net
```

```
wsl.exe -d agent-host -u root -e create_agent_net
create_agent_net arm <container>
create_agent_net disarm <container>
create_agent_net check-gate <container>
wsl.exe -d agent-host -u root -e remove_agent_net
```

| Exit code | Meaning |
|---|---|
| 0 | Network created and verified / container armed and port released / container disarmed or nothing to disarm / every control ok (`check-gate`) / setup removed or nothing to remove (`remove_agent_net`) |
| 1 | Usage or precondition not met; nothing created or changed (`check-gate`: nothing checked, `NOT_CHECKED`) |
| 2 | Runtime failure (create, verify, apply, release, disarm, remove); `check-gate`: a control is missing or mismatched, `GATE_INCONSISTENT` |

## Refresh endpoints (`refresh_endpoints`)

Developer-run tool on the agent-host. Endpoint IPs exist only as `DOCKER-USER` accept rules, resolved when a container is armed. `refresh_endpoints` re-resolves the endpoints and updates those rules for the armed containers without re-arming. Per container it inserts the missing rules first, then deletes the stale ones by exact rule spec. It touches only the endpoint accepts (tcp 443, tag `agent-net:<container>`, not `agent-net:gate`), never the DNS rules, default-deny or INPUT rules. Running it twice changes nothing the second time.

Endpoints file, JSON, default `/etc/agent-net/endpoints.json`: it must be owned by root and not group/world-writable.

```
{"endpoints": ["api.anthropic.com", "platform.claude.com"]}
```

Entries are host names or IPv4 literals, at least one, no duplicates. `create_agent_net` (arm, check-gate) reads the same file when it exists (path override `AGENT_NET_ENDPOINTS_FILE`, which must then exist) and falls back to the two built-in endpoints otherwise. A file that exists but is unusable fails arm and check-gate; the defaults are not used then.

```
GOOS=linux GOARCH=amd64 go build -o refresh_endpoints ./cmd/refresh_endpoints
```

```
refresh_endpoints [--dry-run]
```

The endpoints config is never passed by path: the tool always reads the endpoints JSON from stdin (at most 64 KiB; empty or larger is exit 1). It validates the content (the rules above; stdin bytes have no owner or mode to check), checks privilege, resolves all names (through the same fixed resolvers), installs the bytes as the canonical file `/etc/agent-net/endpoints.json` (parent directory created 0755 if missing; temp file in the same directory, mode 0644, fsync, atomic rename; owner root:root because the tool runs as root; skipped and logged when the installed file already has identical bytes), then updates the rules. Any positional argument or unknown flag prints the usage and exits 1. The canonical file is changed only by root and by this tool; a write failure exits 2 before any rule changes. `create_agent_net` still reads the canonical file as described above.

`--dry-run` prints the plan (including the file that would be installed) and changes and writes nothing. All names are resolved before the first rule changes; a name without an IPv4 address (IPv6-only) is a failure. With no armed container the file is installed and the tool exits 0.

| Exit code | Meaning |
|---|---|
| 0 | Rules up to date (or nothing armed) |
| 1 | Bad invocation, missing, oversize or invalid stdin, or missing privilege; nothing changed |
| 2 | Lookup or endpoints file write failed (nothing changed) or a rule change failed (`REFRESH_INCOMPLETE`, run again to converge) |

Only containers that are already armed are updated; a container armed later picks up the file at arm time.

### Windows side (`refresh_endpoints.exe`)

The default config is `endpoints.json` in this package (`{"endpoints": ["api.anthropic.com", "platform.claude.com"]}`); copy it to `dist/` next to the exes and edit it there. `refresh_endpoints.exe` reads `endpoints.json` from the current working directory (run it from that folder), validates it with the same parser, and runs `wsl.exe -d agent-host -u root -e /usr/local/bin/refresh_endpoints [--dry-run]` with the file bytes on stdin (no `/mnt/c`, no `wslpath`, no staging file). Output is streamed; the exit code is the remote one.

```
GOOS=windows GOARCH=amd64 go build -buildvcs=false -o ../../dist/refresh_endpoints.exe ./cmd/refresh_endpoints_win
```

```
refresh_endpoints.exe [--dry-run]
```

| Exit code | Meaning |
|---|---|
| 0, 1, 2 (from the distro) | Exit code of `refresh_endpoints` as in the table above |
| 1 | Bad invocation, `endpoints.json` missing, oversize (64 KiB) or invalid, or `wsl.exe` not found; wsl.exe not called |
| 2 | `wsl.exe` could not run |

## Configuration

| Variable | Default |
|---|---|
| `AGENT_LOG_LEVEL` | `INFO` (`DEBUG` also traces each command) |
| `AGENT_NET_ENDPOINTS_FILE` | `/etc/agent-net/endpoints.json` (optional unless set) |
| `AGENT_NET_DOCKER_SOCKET` | `/var/run/docker.sock` |
| `AGENT_NET_DAEMON_JSON` | `/etc/docker/daemon.json` (read-only, hashed) |
| `AGENT_NET_DOCKER_PID_FILE` | `/var/run/docker.pid` |

## Tests

```
go test ./...
```

## Layout

| Here | Role |
|---|---|
| `cmd/create_agent_net/`, `cmd/remove_agent_net/`, `cmd/refresh_endpoints/`, `cmd/refresh_endpoints_win/` | entry points (the last one is the Windows `refresh_endpoints.exe`; its logic is in `internal/winrefresh/`) |
| `internal/config/` | constants (network, port table, tags, fixed DNS resolvers and endpoints) and environment overrides |
| `internal/services/` (`docker`, `net`, `nft`, `host`, `render`) | process runner, docker adapter, ip/bridge/iptables adapter, per-port nft adapter, host reads, console output |
| `internal/steps/` | one file per step |
| `internal/sequence/` | numbered step lists (create, arm, check-gate, disarm, remove), first failure sets the exit code |
| `tests/` | sequence tests with a fake runner (stateful ip/bridge/nft/iptables) and a fake host |
