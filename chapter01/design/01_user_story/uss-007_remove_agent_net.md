# USS-007 — Remove the shared agent network

High-level concept only. Rule-level detail (per-port netdev tables, `DOCKER-USER` and `INPUT` rules) is defined in USS-004 and not repeated here.

USS-007 is the inverse of USS-004. It also specifies the behaviour of `disarm`, a subcommand of the root-owned `create_agent_net` binary (USS-004) that USS-006 calls and the cascade reuses.

## Participants

| Participant | Role |
|---|---|
| Developer | Operates `agent-host` (created by USS-001) from host Windows; acting as root (`-u root`), runs `remove_agent_net` |
| Network removal tool | Go executable `remove_agent_net`, run as root inside `agent-host`, that removes the agent containers, the per-port netdev tables (`as_port_*`) and the network |
| `create_agent_net disarm <container>` | Subcommand (USS-004) that removes the `released` elements, netdev table, iptables rules, neighbour entries and FDB entries of one container name; the same code runs in-process for each container of the cascade |
| `agent-host` | WSL2 distro from USS-001; hosts the Docker engine |
| Docker engine | Lists the attached containers, removes them and the network |
| Agent network | Shared bridge network `agents-net` |
| Agent containers | `agent-<id>` containers (USS-002) attached to `agents-net` |

## Goal

As a developer, acting as root inside `agent-host`, I want to remove the whole agent network setup by running a single Go executable, so that no agent container, `as_port_*` netdev table, `agent-net:` iptables rule or `agents-net` is left, without any agent port being left open while it is removed.

## Invocation

```
wsl.exe -d agent-host -u root -e remove_agent_net
create_agent_net disarm <container>     # inside agent-host; called by USS-006
```

`remove_agent_net` takes no arguments; any argument is a usage error. `create_agent_net disarm <container>` takes a docker container name. The tool runs as root: the table and rule removal need `CAP_NET_ADMIN` in the host network namespace of `agent-host`, and access to the Docker socket needs root.

## Build and delivery (Go)

- `remove_agent_net` is a static Go binary for `linux/amd64`, built in `dev-distro`; no Go toolchain is needed on `agent-host`.
- Delivery is owned by USS-001: `create_agent_host` installs the binary to `/usr/local/bin/remove_agent_net` (`root:root`, mode 755) and checks its SHA-256. `disarm` is part of the root-owned `create_agent_net` binary (`/usr/local/bin/create_agent_net`, `root:root`, mode 755, also installed by USS-001).

## Preconditions

- `agent-host` exists and `remove_agent_net` is installed.
- The tool runs as root with `CAP_NET_ADMIN` and Docker socket access; the engine answers.
- Nothing else is required to exist: parts of the setup may already be missing (the netdev tables and iptables rules do not survive a WSL restart).

## Interaction

The 11 steps below are use-case steps (USC-007). They follow the order of the 13 steps of the `remove_agent_net` code pipeline, which additionally records the engine state (after the privilege check) and compares it at the end (before the report).

1. Developer runs `remove_agent_net` as root inside `agent-host`, from host Windows. The tool validates the invocation (no arguments).
2. Tool checks root, `CAP_NET_ADMIN`, Docker socket access and a reachable engine.
3. Tool surveys what exists: the network `agents-net`, the `as_port_*` netdev tables, the `agents_gate` table, and iptables rules whose comment starts `agent-net:`. When none exists, it reports `nothing to remove` and exits 0.
4. Tool lists the containers attached to `agents-net` (stopped ones included; containers are attached at creation, so a created-but-never-armed container is listed and removed too). If any is not named `agent-<id>`, it refuses and changes nothing.
5. For each agent container, tool disarms its port (see Order guarantee and Disarm), verifies, unpauses it if paused, and removes it with `docker rm -f`. Workspaces are never touched; each container is reported on stdout as soon as it is removed, with its workspace kept.
6. Tool deletes any `as_port_*` netdev table left over. Other nftables tables are not touched.
7. Tool deletes any iptables rule whose comment starts `agent-net:` (the `agent-net:gate` rule included) left in `DOCKER-USER` and `INPUT`. Other rules are not touched.
8. Tool deletes the `agents_gate` netdev table. Other tables are not touched.
9. Tool removes the network `agents-net` when it exists.
10. Tool reads back: the network is absent, no `as_port_*` or `agents_gate` table is left and no `agent-net:` rule is left; it lists every failure of steps 6 to 9 too.
11. Tool reports the number of removed containers (workspaces kept) and the removal on stdout and exits 0.

## Order guarantee

- Containers are disarmed and removed before leftover tables and rules, the gate and the network are dropped, in this order: leftover port tables, tagged rules (the gate rule included), the gate table, the network.
- Per container, the port stays locked and isolated (`locked on`, `learning off`, `isolated on`; never unlocked) throughout, so no port ever accepts traffic it should not. Disarm first removes the elements of the gate set `released` tagged with the container name (and its MACs), then deletes its `as_port_<id>` table, its tagged iptables rules in `DOCKER-USER` and `INPUT`, its neighbour entries and its static FDB entries; it never touches the `agents_gate` table, its chains or the `agent-net:gate` rule. The port goes with the veth when the container is removed.
- The first failure stops the run; containers not yet removed keep their tables and rules. There is no rollback.

## Disarm (`create_agent_net disarm <container>`)

Same code as the per-container part of step 5; 12 steps: validate invocation, check privilege and engine, record engine state, require `agents-net`, inspect container, find ports, remove the container's elements from the gate set, delete the port's netdev table, remove its tagged iptables rules, neighbour entries and static FDB entries, verify the container's state is gone, verify engine state, report disarmed.

- Name-keyed: it works for a running, a stopped and a removed container. It is also called by the guard (on `die`, on an error path and for names whose container is gone), by `create_agent_container` (before `docker create` and after an arming failure) and as the clean slate of `arm`.
- Without `agents-net` there is nothing to disarm: exit 0.
- A container that does not exist is not a failure: the cleanup continues from what the name still holds; inside `remove_agent_net` it is simply skipped.
- The identity of the name is collected from docker, from the accept rules of the port table, from the `released` elements whose comment is the container name and from the tagged iptables rules. The port is found from the live veth; for a stopped or removed container, by the table name `as_port_<id>` and the `agent-net:<container>` iptables tag.
- Only the elements, table, rules and entries of the named container are touched; never the `agents_gate` table, its chains or the `agent-net:gate` rule (the table is deleted only by `remove_agent_net`, step 8). A port table bound to the live veth of a different running container is refused before anything is changed. Isolation, `locked` and learning-off stay on the veth. Safe to repeat; with nothing left it reports `nothing to disarm` and exits 0.
- Exit 0 disarmed or nothing to disarm; 1 usage or precondition (`NOT_DISARMED`); 2 runtime (`DISARM_INCOMPLETE`, elements, table or rules may be left; run it again).

Accepted window (owner decision, SFR-NET-012, SSR-NET-003): during each disarm, between the deletion of the netdev table and the deletion of the neighbour entry and the static forwarding entry, the port may not yet be fully closed at the level of the per-port controls (the gate has dropped its frames since its `released` elements were deleted); "without any agent port being left open" does not cover that interval. It is accepted because the container is being removed; the disarm order is unchanged.

Open item: the effect of a leftover `as_port_*` table whose veth is already gone is not yet confirmed; `remove_agent_net` step 6 deletes any such table, a WSL restart clears them too. To be confirmed on the real host.

## Acceptance criteria

- `remove_agent_net` exits 0; no agent container is attached to `agents-net`; the network, all `as_port_*` netdev tables, the `agents_gate` table and all `agent-net:` iptables rules are gone.
- The workspace folders of the removed agents, other containers, other nftables tables and iptables rules, `daemon.json`, the Windows host and `dev-distro` are unchanged.
- Re-running when nothing exists reports `nothing to remove` and exits 0.
- Re-running after a failure finishes the removal.
- `create_agent_net` followed by `create_agent_container` can set the setup up again.

## Failure cases

| Case | Exit | Exit state | Behaviour |
|---|---|---|---|
| Step 1: any argument | 1 | `NOT_REMOVED` | Usage error; nothing is changed |
| Step 2: not root, `CAP_NET_ADMIN` missing, no Docker socket access, or engine unavailable | 1 | `NOT_REMOVED` | Report and stop |
| Step 3: network, nftables tables or iptables chains cannot be read | 1 | `NOT_REMOVED` | Report and stop; nothing is changed |
| Step 4: a container on `agents-net` is not named `agent-<id>` (or the list cannot be read) | 1 | `NOT_REMOVED` | The foreign containers are listed; nothing is changed, no agent container is removed |
| Step 5: disarming or removing an agent container fails | 2 | `REMOVE_INCOMPLETE` | Stops at once; the container and those still to remove are named; their tables and rules stay; re-run finishes |
| Steps 6 to 9: table, rule or network removal fails | 2 | `REMOVE_INCOMPLETE` | Recorded, the later steps still run, reported by the read-back; no rollback; re-run finishes |
| Step 10: read-back finds something left (or the engine configuration changed) | 2 | `REMOVE_INCOMPLETE` | What is left is listed; re-run finishes |
| Part of the setup already missing | 0 | n/a | Each removal tolerates what is absent; the run is idempotent |
