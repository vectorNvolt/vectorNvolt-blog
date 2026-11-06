# USS-004 — Create the shared agent network

High-level concept only. Detailed design (bridge flags, nftables `netdev` and `DOCKER-USER` rules, rationale tables, verification commands) is intentionally not repeated here; it is specified in the SFR-NET-* requirements derived from USS-004.

USS-004 is a prerequisite of USS-002: `create_agent_container` consumes the network and the Agent network control provided here.

## Participants

| Participant | Role |
|---|---|
| Developer | Operates `agent-host` (created by USS-001) from host Windows; acting as root (`-u root`), runs `create_agent_net` |
| Network creation tool | Go executable `create_agent_net`, run as root inside `agent-host`, that creates the shared agent network and provides the Agent network control |
| `agent-host` | WSL2 distro from USS-001; hosts the Docker engine |
| Docker engine | Creates the bridge network |
| Agent network | Shared bridge network (`agents-net`) that agent containers attach to (USS-002) |
| Agent network control | Subcommands `create_agent_net arm <container>` and `disarm <container>`, plus the read-only `check-gate <container>`, that apply, verify, remove and check the host-side network controls for one agent container, and `released` and `evict <mac>`, that list and remove elements of the gate set |

## Goal

As a developer, acting as root inside `agent-host`, I want to create the shared agent network by running a single Go executable, so that agent containers (USS-002) have a segmented, identity-pinned and egress-controlled network to attach to, with the host-side controls defined once and reused wherever a container is created or relaunched.

Isolation claim (precise):

- All controls are applied on the host side (the bridge, or the agent's veth as the host sees it). They are not addressable, observable or clearable from inside the sandbox.
- Agents cannot reach each other; traffic leaving the bridge is subject to a host-side egress policy.
- Nothing prevents an agent from mutating its own primary network attachment; the controls only bound what such a mutation can reach.
- This is NOT a VM or kernel boundary, and `agent-host` still shares one WSL2 VM with other distros (see USS-001).

## Invocation

```
wsl.exe -d agent-host -u root -e create_agent_net
```

The tool accepts exactly six forms:

- no arguments: create the network (there is exactly one agent group, hence one network);
- `create_agent_net arm <container>`: called by `create_agent_container` (USS-002);
- `create_agent_net disarm <container>`: called by USS-006 `remove_agent_container`, USS-007 `remove_agent_net`, `create_agent_container` (before `docker create` and after an arming failure) and `agent_lifecycle guard`; `arm` runs it as its clean slate;
- `create_agent_net check-gate <container>`: read-only per-port check, called by `create_agent_container` and `agent_lifecycle guard`;
- `create_agent_net released`: read-only listing of the elements of the gate set, one `<mac> <comment>` line each, called by `agent_lifecycle guard`;
- `create_agent_net evict <mac>`: removes one element of the gate set by MAC, called by `agent_lifecycle guard`.

Any other argument is a usage error. The subcommands are part of the binary, so they are available once USS-001 installs `create_agent_net`; network creation does nothing extra to provide them. Host-side commands (`nft`, `iptables`, `ip`) run as root in `agent-host`.

The Developer runs the tool as root (`-u root`) inside `agent-host`: creating the network, the bridge port controls, the per-port nftables table and `DOCKER-USER`/`INPUT` rules need `CAP_NET_ADMIN` in the host network namespace of `agent-host`, and access to the Docker socket needs root. Running it with `-u root` from host Windows means `agent-operator` needs neither sudo nor `docker` group membership.

Network name: `agents-net`. Subnet: one fixed private /24, `172.30.0.0/24`. It must not overlap any subnet already in use in `agent-host`: the subnets of existing Docker networks and the addresses and routes of `agent-host` (including the WSL NAT interface); the tool checks this. Docker's default address pools are not a clash criterion: the subnet is given explicitly, and those pools (172.17.0.0/16–172.31.0.0/16) always contain it.

## Build and delivery (Go)

- `create_agent_net` is a static Go binary for `linux/amd64`, built in `dev-distro`.
- No Go toolchain is needed on `agent-host`; Go is therefore not an `agent-host` dependency.
- Delivery is owned by USS-001: `create_agent_host` installs the binary to `/usr/local/bin/create_agent_net` (`root:root`, mode 755) and checks its SHA-256.

## Preconditions

- USS-001 is complete: `agent-host` exists and Docker is installed and running.
- The tool does NOT create or modify the Docker `daemon.json`, and does not reload or restart the Docker daemon; USS-001 owns the Docker runtime registration.
- The agent network does not exist yet. If it already exists: report and stop.
- The subnet `172.30.0.0/24` does not overlap a subnet in use in `agent-host` (existing Docker networks, host addresses and routes).

## Interaction

1. Developer, acting as root, runs `create_agent_net` inside `agent-host`, from host Windows.
2. Tool validates the invocation (no arguments) and checks the preconditions (including that the Docker engine is reachable). It reports a mismatch and stops; it never repairs host configuration.
3. Tool creates the shared bridge network `agents-net` (subnet `172.30.0.0/24`) through the Docker engine and installs its gate (see "The agents-net gate"). The container's DNS resolver file is not provided here: `/etc/resolv.conf` is a read-only mount of the host file `/etc/agent-sandbox/resolv.conf` set up by `create_agent_container` (exactly `nameserver 1.1.1.1` and `nameserver 9.9.9.9`) and verified inside the container by USS-002 after the unpause (SFR-AGC-024, SFR-NET-020).
4. Tool verifies the result, the gate included (see acceptance criteria), and reports success or failure.

## Network creation (summary)

Segmentation uses one shared bridge, not one bridge per agent:

- Every agent port is marked isolated. This gives no agent-to-agent forwarding, enforced at L2 inside the bridge.
- Docker's inter-container communication switch is not used for segmentation.
- `agents-net` is created by `create_agent_net` as a Docker bridge network with a NAT route out (not `Internal`) and the fixed subnet `172.30.0.0/24`. Outbound traffic is bounded by the host-side egress rules (layer 6 below), not by the network type.

## Host-side network controls (summary)

Applied per agent container by the Agent network control:

Six layers on each agent port (the WSL2 kernel has no nftables bridge family, so the `netdev` family is used):

1. Port policy: a per-port nftables `netdev` table with ingress and egress chains bound to the agent's veth, policy drop; accept rules are added only at release.
2. Source identity: an explicit MAC and IP per agent; ingress accepts only frames whose source MAC, IPv4 source address, and ARP sender MAC and IP match the declared ones.
3. Destination limit: the agent may send only to the bridge MAC, and receives only from the bridge MAC. IPv6 and all other frame types are dropped.
4. Kernel MAC lock: port `locked on`, learning off, and a static forwarding-table (FDB) entry pinning the agent MAC to its port.
5. Port isolation: bridge port isolation on each agent veth, as above.
6. Host side: iptables rules in `DOCKER-USER` and `INPUT`, tagged `agent-net:<container>`. Egress policy is default-deny with two explicit allow rules: (a) DNS, UDP and TCP port 53 to the resolvers `1.1.1.1` (primary) and `9.9.9.9` (secondary) only; (b) TCP port 443 to `api.anthropic.com` (model API) and `platform.claude.com` (OAuth token exchange, refresh and revocation for `claude /login`) only. Everything else is dropped; IPv6 is dropped at the port (layer 3). The check-gate report lists rule (a) and rule (b) as distinct controls. The `/login` sign-in pages (`claude.ai`, `claude.com`) are opened in the developer's browser on Windows, not from the container. Endpoint hostnames are resolved host-side at arm time. Per-profile allowlists are out of scope.

Additionally:

- Published-port bypass: host-addressed traffic from agent ports is default-deny in `INPUT` (only replies to flows the host started are accepted). Segmentation at the bridge does not cover this path. That published ports bind to `127.0.0.1` on the host is the concern of the container creation tool; this tool does not check it.

Everything here is host-side; none of it can be set in the container manifest, and it dies with the veth (see the non-persistence bullet under Agent network control).

## The agents-net gate

Why attached at creation, why a gate (verified on the real host, 2026-10-31): Docker rejects `docker network connect` while the `none` network is attached ("container cannot be connected to multiple networks with one of the networks in private (none) mode"). After `network disconnect none` and `connect` the commands succeed, but gVisor (`runsc`) only shows `lo` in the container: it never sees interfaces added after the start. So agent containers are created on `agents-net` and "no reachability until armed" is enforced network-wide instead. The gate is installed and verified by the creation of `agents-net`:

- nft table `netdev agents_gate`, bound to the bridge device of `agents-net` (`br-<id>`): an ingress chain and an egress chain, policy drop, accepting only frames whose source MAC (ingress) or destination MAC (egress) is in the set `released` (created empty; the verification does not read its elements; `released` lists them). Ingress additionally requires the port mark (bit `0x10000000`), which the accept rules of the released per-port table set and the gate clears when it accepts; a frame without the mark is dropped, and a gate built without the mark is a mismatch (fail closed). This covers container to host, DNS, routed egress and host to container.
- iptables rule in `DOCKER-USER`, `-i <br> -o <br> -m physdev --physdev-is-bridged -j DROP`, comment `agent-net:gate`: blocks port-to-port traffic between unarmed containers. It needs `bridge-nf-call-iptables` = 1, which the tool reads and requires and never sets.
- `arm` adds the agent MAC to `released`, as an element with the container name as comment, in the same nft transaction that opens the per-port table; a failure after the release removes it again. `disarm` removes it. `check-gate` reports `closed` (MAC not in `released`), `released` (MAC in `released`) or `inconsistent`; a missing gate table is `inconsistent`.
- Verified on `agent-host`: netdev hooks on the bridge device work; the nftables bridge family is not supported by the kernel; `br_netfilter` is on; the iptables `physdev` match works. `ebtables` is deliberately not used (an extra dependency, likely nft-bridge-backed).
- A network created before the gate existed must be recreated (`remove_agent_net`, then `create_agent_net`); `arm` refuses it. This is done automatically at every dockerd start by `agent_lifecycle restore` (`agent-restore.service`, USS-001), which also removes the old agent containers first; by hand it is only a troubleshooting fallback. The host configuration is not changed.

## Agent network control

Arming is one callable unit, not inline code in the creation tool, because every control dies with the veth:

- It consists of subcommands of `create_agent_net` (`arm`, `disarm`, `check-gate`, `released`, `evict`), not a fourth binary. `arm` is called by container creation (USS-002); `disarm` by USS-006, USS-007, container creation and the guard (its behaviour is specified in USS-007); `released` and `evict` by the guard. Any path that does not call `arm` leaves the agent closed behind the gate, never reachable.
- Sequence: the caller creates the container attached to `agents-net` (`--network agents-net`, runtime `runsc`, `--restart no`), starts it and pauses it; no agent process runs at container start (PID 1 is `tini` running `dockerd`; the agent runs only through `docker exec -u 1001` after the release, the check-gate and the unpause). `arm` takes 13 steps (progress lines `[n/13]`): it first runs a clean slate, the name-keyed disarm of every leftover of the container name (so a re-arm cannot inherit an open gate or stale state; a leftover that cannot be removed fails the arm, and a port table bound to the live veth of another container refuses it), loads the per-port table with policy drop and no accept rules, sets isolated, locked, learning off, the static FDB entry and the iptables rules, verifies, releases by adding the agent MAC to the gate set `released` and the accept rules (which set the port mark) in one atomic `nft -f`, and verifies again. The caller then runs `check-gate` while the container is still paused and unpauses it only when the check succeeds. `arm` attaches nothing.
- Fail-closed: if any apply or verify step fails, the failure is reported to the caller, which disarms the container and then removes it (unpausing it just before `rm -f`, behind the gate). Before loading anything, `arm` verifies with `docker inspect` that the container exists, is running, is paused, is attached to `agents-net` only, has runtime `runsc` and has restart policy exactly `no` (an empty policy is refused), and that the gate is present; otherwise it changes nothing and reports `GATED_UNARMED`, exit 1.
- `check-gate` reports 12 controls, one line each, including `container confinement` (restart policy `no`, attached to `agents-net` only, running; not required to be unpaused) and the port mark on the accept rules of a released port; every control is checked even after a failure.
- Per-port table: each agent port has its own table and its own entry in `released`; a port that is not released accepts nothing.
- Startup window: the container is attached from its creation, so its port exists before any per-port table does. The network-wide gate closes that window (next section): until the agent MAC is in `released`, no frame to or from the port passes, whatever the container does.
- Not persistent: tables and rules do not survive a WSL restart or reboot; the `agents-net` Docker network does. Containers do not restart on their own (`--restart no`), so no port exists without its table. The gate is rebuilt automatically at dockerd start by `agent_lifecycle restore` (`agent-restore.service`), which writes the ready marker `/run/agent-sandbox/ready` that `create_agent_container` waits for; `agent_lifecycle guard` follows container start, unpause, die and update and network connect and disconnect: on `die` it runs `disarm`, on the other events `check-gate`, removing the container on a clean failure; on an error it disarms first and removes only after a successful disarm. It also reconciles at start-up and every 60 s (`released`, `evict`, `disarm`). Recovery is remove and recreate, never `docker start` of an old agent container: the per-port controls stay bound to the veth (no nft bridge family in the WSL2 kernel), so a restarted container is removed, not re-armed; a frame of a restarted container, whose new veth has no port table and so sets no mark, is dropped by the gate even when its MAC is still in `released`. An idle stop of the distro by WSL still kills running containers.
- `arm` and `disarm` also handle a stale PERMANENT neighbour entry for the container IP: `arm` deletes one owned by no live `agents-net` endpoint (and fails when a live endpoint owns it), `disarm` deletes one owned by no live endpoint.
- The host-side veth is looked up from the host (never from inside the sandbox): `docker inspect` gives the container PID, the peer interface index is read in the container's network namespace, and it is matched to the veth on the host.
- It needs `CAP_NET_ADMIN` in the host network namespace, which the root tool already holds; no new privilege is introduced, and arming can never be delegated to anything running inside an agent.

## Open items

- Nothing here is measured; the validation list (for example isolation applying cleanly to a Docker-managed port, controls being lost on reconnect, which published-port turnaround mechanism the host uses, veth derivation under `runsc`) is unconfirmed.
- A known gap remains: an agent can still mutate its own primary attachment, and no denial or audit record results. Whether gVisor's netstack could reject this is unverified.
- Unverified assumption: `docker pause` works with the `runsc` runtime and freezes gVisor's network stack.
- Verified (replaces an earlier assumption): `docker network connect` is rejected while `none` is attached, and gVisor never sees an interface added after the start. Hence the attachment at creation and the gate above.
- Endpoint IPs are resolved on the host at arm time (A records only) and stay pinned until the next arm or a manual `refresh_endpoints`, which resolves all names first and updates the rules of every armed container. The endpoint names come from the root-owned `/etc/agent-net/endpoints.json` (absent: `api.anthropic.com` and `platform.claude.com`). Both default hosts are CDN-backed (currently A `160.79.104.10`, AAAA `2607:6bc0::10` for both), so a long-running agent can lose the model API or token refresh when their IPs change until `refresh_endpoints` is run; an automatic periodic refresh or a hostname-filtering proxy on `agent-host` is not provided. The DNS resolvers themselves (`1.1.1.1`, `9.9.9.9`) are fixed and not deferred.
- Open: which resolver the host uses for the arm-time lookup is not defined, and the container resolves through `1.1.1.1` / `9.9.9.9`, so the two lookups may return different CDN IPs; the design does not say how this is reconciled.
- Open: IPv6 egress is dropped at the port by the layer 3 policy drop (IPv6 frames never accepted), so the AAAA records are unusable; whether this is intended to stay (no AAAA rule) or whether the container must be prevented from preferring AAAA is not decided.
- Note: the container's `/etc/resolv.conf` is a read-only bind mount of the host file `/etc/agent-sandbox/resolv.conf`, prepared by `create_agent_container` and verified inside the container after the unpause (SFR-AGC-024), not by the image and not by this tool. That `runsc` accepts this mount is still to be confirmed on a real run (open item in USS-002 / USC-002).
- Unverified assumption: `claude /login` with the pasted-code flow needs no direct container access to `claude.ai` or `claude.com`.

## Acceptance criteria

- `create_agent_net` exits successfully and the Docker network `agents-net` (`172.30.0.0/24`) exists.
- The network is a bridge network (not `Internal`, NAT route out) usable by USS-002 containers; outbound traffic is bounded by the host-side egress allowlist (layer 6).
- `create_agent_net arm`, `disarm` and `check-gate` (each with `<container>`), `released` and `evict <mac>` are available.
- `daemon.json`, the Windows host and `dev-distro` are unchanged, and the Docker engine was not restarted.
- The gate (`agents_gate` table with its `released` set loaded empty, and the `agent-net:gate` rule in `DOCKER-USER`) is installed and verified; `bridge-nf-call-iptables` is 1.
- The whole network is created by one run of the Go executable.
- Running it again when the network already exists: report and stop.

## Failure cases

| Case | Behaviour |
|---|---|
| Any argument other than none, `arm <container>`, `disarm <container>`, `check-gate <container>`, `released` or `evict <mac>` (or an `evict` argument that is not a MAC address) | Usage error; nothing is created |
| Docker engine unavailable | Report and stop |
| Not running as root / missing `CAP_NET_ADMIN` in the host netns | Report and stop; nothing is created |
| Network already exists | Report and stop |
| Subnet `172.30.0.0/24` overlaps a subnet in use in `agent-host` (Docker network, host address or route) | Report and stop; nothing is created |
| `bridge-nf-call-iptables` is not 1 | Read after the network is created (the bridge must exist): report and stop, no gate is installed, the network is left in place (`CREATED_UNVERIFIED`, exit 2); the tool never sets it |
| `arm`, `disarm` or `check-gate` with the container name `gate` | Usage error; the rule tag `agent-net:gate` would collide with the tag of the network gate rule |
| Network creation fails | Report and stop |
| Gate install or verification fails | Report and stop; the network is left in place (no rollback) |
| Arming or verification fails for a container | Failure is reported to the caller (USS-002), which removes the container while it is paused |
| Cleanup / rollback of partially created resources | None; report and stop |

