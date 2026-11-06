# USS-002 — Create a gVisor agent container

## Participants

| Participant | Role |
|---|---|
| Developer | Operates `agent-host` (created by USS-001) from host Windows |
| Container creation tool | Go executable `create_agent_container`, run as root inside `agent-host`, that creates one agent container |
| `agent-host` | WSL2 distro from USS-001; hosts the Docker engine and the gVisor (`runsc`) runtime |
| Docker engine | Creates and runs the container with the `runsc` runtime |
| Agent container | New gVisor sandbox, one per `agentID` |
| Agent network | Shared bridge network the container attaches to (`agents-net`) |

## Goal

As a developer, I want to create an isolated gVisor-based agent container by running a single Go executable with an `agentID`, so that each agent runs in its own sandbox with its own workspace folder and an agent network attachment, without hand-assembling `docker run` options.

Isolation claim:

- The container runs under `runsc`: a user-space kernel (Sentry) mediates syscalls between the agent and the `agent-host` kernel, and provides an isolated network stack.
- Raw sockets are disabled in the default runtime.
- This is NOT a VM boundary: gVisor isolates the agent from the `agent-host` kernel, and `agent-host` still shares one WSL2 VM with other distros (see USS-001).

## Invocation

```
wsl.exe -d agent-host -u root -e create_agent_container <agentID>
```

The tool runs as root: the host-side network controls (bridge port isolation, locked port + FDB entry, the per-port nftables `netdev` table and `DOCKER-USER`/`INPUT` rules) need root, and so does access to the Docker socket. Running it with `-u root` from host Windows means `agent-operator` needs neither sudo nor `docker` group membership.

| Argument | Required | Meaning |
|---|---|---|
| `agentID` | yes | Identifier of the agent; names the container, the workspace folder and the network attachment |

`agentID` must match `^[a-z][a-z0-9-]{0,30}$` (lowercase letters, digits and `-`; starts with a letter; 1 to 31 characters). The container is named `agent-<agentID>`.

## Preconditions

- USS-001 is complete: `agent-host` exists and Docker and gVisor are installed, and `create_agent_container` is installed at `/usr/local/bin/create_agent_container` (`root:root`, mode 755).
- The Docker `daemon.json` on `agent-host` already registers the gVisor runtime `runsc`; USS-001 installs `runsc` and registers it (`runsc install`, then restarts dockerd). The tool does NOT create or modify `daemon.json`, install `runsc` or reload/restart the Docker daemon.
- A `Workspace-Agents` folder exists on `agent-host` (location: `/home/agent-operator/Workspace-Agents`). The developer creates it manually as `agent-operator` before the first run (a prerequisite, not created by any tool). The tool creates only the per-agent subfolder inside it.
- The shared agent network `agents-net` exists (created by USS-004).
- The agent network and its host-side controls are provided by USS-004; after each dockerd start `agent-restore.service` (`agent_lifecycle restore`, USS-001) rebuilds them and writes the ready marker `/run/agent-sandbox/ready` holding dockerd's MainPID. The tool waits up to 60 s for that marker before any docker call.
- The `Workspace-Agents` root is a directory (not a symbolic link) owned by `agent-operator`.
- The agent image `agent-image:latest` is available to Docker (created by USS-003).
- No container for the same `agentID` exists.

## Interaction

1. Developer runs `create_agent_container <agentID>` as root inside `agent-host`, from host Windows.
2. Tool validates the argument, takes the per-agent lock `/run/agent-sandbox/<agentID>.lock` (non-blocking `flock`, held for the whole run, so `agent_lifecycle guard` ignores the start, unpause and network events of this run and skips the container at its reconcile; a held lock, by another create or remove run or by the guard, is a failure, exit 1) and waits for the ready marker (see Preconditions; timeout `NET_NOT_READY`, exit 3, pointing at `systemctl status agent-restore.service`). It then checks the preconditions (including that the `runsc` runtime is registered with Docker, that no container of that name exists, and that `agents-net`, the image and the `Workspace-Agents` root are present). For `agents-net` only its existence is checked here; the gate is not checked before the start and is verified when the port is armed (step 5). It reports a mismatch and stops; it never repairs host configuration.
3. Tool creates the per-agent workspace folder `/home/agent-operator/Workspace-Agents/<agentID>` and its `.output` subfolder, and hands both to `agent-operator` (`chown`). This is the whole folder structure; the tool creates nothing else in the workspace. It also prepares the persistent Claude state `/var/lib/agent-sandbox/<agentID>/claude` (`1001:1001`, 0700; `/var/lib/agent-sandbox` and `/var/lib/agent-sandbox/<agentID>` `root:root` 0711, so other agents cannot list it; kept with its content if present) and the host DNS file `/etc/agent-sandbox/resolv.conf` (exactly `nameserver 1.1.1.1` and `nameserver 9.9.9.9`, `root:root`, 644). Last, before the container is requested, it removes the leftover network state of the agent name (`create_agent_net disarm agent-<agentID>`, name-keyed and idempotent) and the orphaned elements of the gate set `released` (`create_agent_net released`, `evict <mac>`). It never touches an existing container, because none exists for this name; a failing call stops the run before `docker create`, exit 2.
4. Tool creates the container `agent-<agentID>` from `agent-image:latest` through the Docker engine with the `runsc` runtime, `--network agents-net --restart no`, the limits listed under "Container capabilities", the workspace folder mounted read-write at `/workspace`, the Claude state mounted read-write at `/home/agent/.claude` with `CLAUDE_CONFIG_DIR=/home/agent/.claude` (login and conversation history survive recreating the container) and the host DNS file mounted read-only at `/etc/resolv.conf`, and starts it. The image's `tini` entrypoint runs as PID 1 (root) and starts the in-container `dockerd` as root; this is an idle init, no agent process runs at container start. The container is attached to `agents-net` from its creation but has no reachability: the agents-net gate (USS-004) drops every frame to and from its port until its MAC is released.
5. Tool pauses the container (`docker pause`) and verifies the pause (`State.Paused` reads `true`). If the pause is not verified, the tool stops: it neither arms nor unpauses. Then `create_agent_net arm` requires the `runsc` runtime and restart policy exactly `no`, starts from a clean slate (name-keyed disarm of every leftover of the name), loads the per-port table, applies the port flags and host-side rules, verifies them and releases the port (it attaches nothing); the tool reads the controls back with `create_agent_net check-gate`. Only then does the tool unpause the container (`docker unpause`).
The tool prints a progress line `[n/14] <step name>` on stdout before each of its fourteen steps (validate, lock, wait for ready, preconditions, workspace, state, clean leftovers, create and start, pause, arm, release, check `resolv.conf`, verify, report).

6. The agent is started only after the sequence pause, arm, check-gate, unpause has passed, through `docker exec -u 1001` (`enter_agent`); the tool's own first exec is the `resolv.conf` probe below, not agent work, and no agent work is started before success is reported. Right after the unpause the tool checks `/etc/resolv.conf` inside the container with an exec as uid 0 (`cat`; it must show `nameserver 1.1.1.1` and `nameserver 9.9.9.9` and not `127.0.0.11`; nothing is written in the container; `docker exec` does not work on a paused container, so this cannot precede the unpause). If the check fails, the tool re-pauses the container (CONTAINED) and exits 2. Tool verifies the result (see acceptance criteria) and reports success (a fixed message naming the container and the workspace) or failure.

## Container capabilities (summary)

The container is created with:

- the `runsc` runtime (not `runc`);
- a closed capability allowlist (`--cap-drop ALL`, then re-add only the 15 the in-container Docker daemon needs: `SYS_ADMIN`, `SYS_CHROOT`, `MKNOD`, `CHOWN`, `DAC_OVERRIDE`, `FOWNER`, `FSETID`, `SETUID`, `SETGID`, `SETPCAP`, `SETFCAP`, `KILL`, `AUDIT_WRITE`, `NET_BIND_SERVICE`, `SYS_RESOURCE`) and `no-new-privileges`;
- exactly three host mounts: the agent workspace at `/workspace` read-write, the per-agent Claude state at `/home/agent/.claude` read-write (`CLAUDE_CONFIG_DIR` set to it) and the host DNS file at `/etc/resolv.conf` read-only; no credential, signing-key or keyring mount;
- memory 4g, CPU 2, PID limit 512, and size-bounded `tmpfs` mounts `/tmp` (`rw,noexec,nosuid`, 512m) and `/var/lib/docker` (`rw,nosuid`, 2g), together strictly below the memory limit (by the fixed values; the tool does not compute the sum at run time);
- attachment to the agent network only (not host networking);
- `--restart no` (containers never restart on their own).

Agent user: every process of the agent workload is started with an explicit user, `docker exec -u 1001 agent-<agentID> ...` (e.g. `docker exec -it -u 1001 agent-<agentID> claude`); the uid alone is given because an explicit group (`-u 1001:1001`) makes Docker drop the supplementary group `docker`. The image sets no default `USER`, because its init and `dockerd` run as root (USS-003), so an exec without `-u` runs as uid 0. This applies to every launcher of agent work, not only to this tool (SSR-AGC-011). The agent user is a member of the in-container `docker` group: that is root-equivalent control of the in-container engine, although the agent process itself does not run as uid 0. The tool checks uid/gid 1001 and that `docker` is among the groups, not that the user is the group's only member, and it does not read the capability sets (effective, permitted, bounding) of any process; the capability limit is the creation options read back from the inspected container.

Constraint on the story: the container relies on the `tini` entrypoint of the agent image (USS-003) as its init and is created WITHOUT Docker `--init`.

Open item: the init and `dockerd` run as root by design. The capability allowlist is the 15 capabilities above (SSR-AGC-009); the PID limit (512) and the tmpfs sizes are design values (SSR-AGC-012), defined in USC-002 step 4.

## Network provisioning (summary)

Networking is provided by the tool, not by the agent:

- The container is created attached to the shared agent network (a later `network connect` is rejected for a `none` container, and gVisor does not see interfaces added after the start) and is closed by the agents-net gate; it is paused until the port is armed and verified, and the host-side controls (agent-to-agent isolation, fixed network identity, egress policy) are applied and verified before it is unpaused.
- `api.anthropic.com` (model API) and `platform.claude.com` (token exchange and refresh after the first web-based `claude /login`, USS-003) are reachable (TCP 443, the only egress hostnames) through the egress allowlist of USS-004. The token is created in the container and lives only there; the tool injects no credential.
- DNS: the container's `/etc/resolv.conf` (exactly `nameserver 1.1.1.1` and `nameserver 9.9.9.9`, `root:root` mode 644, not writable by the agent user; resolvers reachable on UDP and TCP port 53 only) is a read-only bind mount of the host file `/etc/agent-sandbox/resolv.conf`, which the tool prepares in step 6 and verifies inside the container after the unpause (SFR-AGC-024, SFR-NET-020); it is not written by the image, its init or an exec. Docker would otherwise regenerate its own file at every container start (embedded DNS `127.0.0.11`, which gVisor does not serve); the read-only mount keeps that from happening and the agent cannot change it.
- Open (confirm on a real run): whether `runsc` accepts the read-only bind mount over `/etc/resolv.conf` and the `.claude` mount, and whether owner and mode read back as expected. The verification reveals a mismatch.
- If any arming step fails, the tool first disarms the host-side controls (`create_agent_net disarm`, exit 0 required) and only then removes the container (unpausing it first for the removal, because `runsc` cannot kill a paused sandbox). If the disarm fails, nothing is removed: the container is left paused behind its controls and the tool exits 2.
- The controls are host-side only; the agent cannot see or clear them. They die with the veth: never `docker start` an old agent container, remove it (USS-006) and recreate it. After a dockerd restart (WSL restart, reboot or idle stop) `agent_lifecycle restore` has already removed the old agent containers and rebuilt the gate; `agent_lifecycle guard` follows container start, unpause, die and update and network connect and disconnect (it skips containers whose lock a tool holds): on `die` it disarms the name; on the other events it runs `check-gate` and removes the container on a clean failure; on an error it disarms first and removes only after a successful disarm (a failed disarm leaves the container in place, fail-closed). It also reconciles at start-up and every 60 s. A restarted container is removed, not re-armed. Known WSL limits: an idle stop of the distro (no `wsl.exe` session attached) still kills running containers, the next launch recreates them (login and history kept, the running claude process lost); per-port controls stay veth-bound (no nft bridge family), so a restarted container is replaced, never repaired.
- Arming is `create_agent_net arm <container>` (USS-004), called by this tool. Recovery is remove and recreate.



## Workspace

- Per-agent folder: `/home/agent-operator/Workspace-Agents/<agentID>` on `agent-host`, mounted read-write at `/workspace` inside the container. The `<agentID>` folder name is taken from the argument.
- Layout inside `/workspace`: this story creates only the folder and its `.output` subfolder. Any other content (for example `src`, `.npm-env`, `.venv`, `.conda-env`) depends on the application architecture and is not created, assigned or masked here; no git repository or worktree is assigned by this story.
- The folder and its `.output` subfolder are created by the tool (as root) and then both handed to `agent-operator` (`chown` of the two folders themselves, not recursive), whether created or reused.
- If the folder already exists, it is reused; this is not an error. A missing `.output` subfolder is created.

## Daemon / runtime dependency (summary)

The host Docker daemon registers the runtime `runsc` (USS-001; raw sockets off). The runtime is selected by the host at container creation; the agent cannot choose it. Runtime arguments are applied at container creation, so a daemon change does not affect already running containers.

## Acceptance criteria

- `create_agent_container <agentID>` exits successfully and the container exists and is running.
- `docker inspect` shows the container uses the `runsc` runtime.
- `/home/agent-operator/Workspace-Agents/<agentID>` and its `.output` subfolder exist on `agent-host`, owned by `agent-operator`, and the folder is mounted read-write at `/workspace` in the container.
- The container is attached to the agent network and the host-side network controls are verified as applied.
- The container was paused (and the pause verified) before arming, and unpaused only after the release was verified and read back.
- `docker inspect` shows the capability allowlist, `no-new-privileges`, memory 4g, CPU 2, PID limit 512, restart policy `no`, the two `tmpfs` mounts, `CLAUDE_CONFIG_DIR`, and exactly three bind mounts: the workspace (read-write), `/home/agent/.claude` (read-write, from `/var/lib/agent-sandbox/<agentID>/claude`) and `/etc/resolv.conf` (read-only, from `/etc/agent-sandbox/resolv.conf`).
- In the container, `/etc/resolv.conf` contains exactly `nameserver 1.1.1.1` and `nameserver 9.9.9.9`, is `root:root` mode 644 and is not writable by uid 1001.
- `docker inspect` shows no user set for the container (the init runs as root); `docker exec -u 1001 agent-<agentID> id` shows uid 1001 and gid 1001, and `docker exec -u 1001 agent-<agentID> id -nG` lists the in-container group `docker`.
- `daemon.json`, the Windows host and `dev-distro` are unchanged.
- The whole container is created by one run of the Go executable.

## Failure cases

| Case | Behaviour |
|---|---|
| Missing or invalid `agentID` | Usage error; nothing is created |
| Per-agent lock held (another create or remove of the same agent is running, or the guard is checking or removing a container of that name) | Report and stop; nothing is created, exit 1 |
| Ready marker missing or not holding dockerd's current MainPID after 60 s | `NET_NOT_READY`, exit 3; nothing is touched; check `systemctl status agent-restore.service` |
| `runsc` runtime not registered with Docker (daemon.json mismatch) | Report the mismatch and stop; do not modify `daemon.json` |
| Docker engine unavailable | Report and stop |
| Container for `agentID` already exists | Report and stop; the existing container is not reused and not modified |
| `Workspace-Agents` root missing, not a directory (or a symbolic link) or not owned by `agent-operator` | Report and stop; no side effect, exit 1 |
| Workspace folder or `.output` cannot be created or handed to `agent-operator` | Report and stop; nothing is started, exit 2 |
| Removal of leftover network state (`disarm`, `released`, `evict`) fails | Report and stop before `docker create`; no container is created, exit 2 |
| Container creation fails | `NOT_CREATED`; the tool does no cleanup and starts nothing; exit 2 |
| Container start fails | The container created by this run is removed (`REMOVED`; it is attached to `agents-net` but never released by the gate, so no disarm is needed; no agent work). If that removal fails, `CREATED_CLEANUP_FAILED` and the container is named for manual removal; exit 2 |
| Pause fails or `State.Paused` is not `true` | Tool stops; nothing is armed or unpaused; the container (attached to `agents-net` but never released by the gate, so removal without disarm is safe) is removed (`REMOVED`, or `REMOVE_FAILED` with the container named); exit 2 |
| Network arming or verification fails (including a network without the gate) | Tool runs `create_agent_net disarm` (exit 0 required), then removes the container (it never ran agent work; `REMOVED`, or `REMOVE_FAILED` with the container named); failure is reported, exit 2. If the disarm fails, nothing is removed, the container stays paused (`DISARM_FAILED`) and the tool exits 2 |
| Unpause fails | Container is left paused, armed and unreleased (CONTAINED), not removed; exit 2 |
| Check of `/etc/resolv.conf` after the unpause fails (non-zero exit of the root exec, or content mismatch) | Tool re-pauses the container and verifies the pause (CONTAINED); if that fails it reports UNSAFE (possibly running); never removed; exit 2 |
| Final verification fails (USC-002 step 9 checks) | Tool re-pauses the container and verifies the pause (CONTAINED); if that fails it reports UNSAFE (possibly running); never removed; exit 2 |
| Network `agents-net` does not exist | Report and stop; the tool does not create it |
| Agent image missing locally | Report and stop; the tool neither builds nor pulls it |
| Cleanup / rollback of partially created resources | The run removes only the container it created (after a failed start, pause or arming, never after a failed create, unpause, `resolv.conf` check or final verification); the workspace folder and any resource the run did not create are kept |
| Interrupt (Ctrl-C) | The running command is cancelled and the current step fails normally; cleanup, disarm and containment still run to completion, so no container is left behind or uncontained |

## Configuration

The tool looks for the `Workspace-Agents` root at `/home/agent-operator/Workspace-Agents` and for `create_agent_net` at `/usr/local/bin/create_agent_net`. The environment variables `AGENT_SANDBOX_WORKSPACE_ROOT` and `AGENT_SANDBOX_ARM_BIN` override those two locations (they change only where the tool looks); `AGENT_LOG_LEVEL` sets the console detail. Further paths and overrides: `AGENT_SANDBOX_RUNTIME_DIR` (`/run/agent-sandbox`, ready marker and locks), `AGENT_SANDBOX_STATE_ROOT` (`/var/lib/agent-sandbox`), `AGENT_SANDBOX_HOST_RESOLV` (`/etc/agent-sandbox/resolv.conf`) and `AGENT_SANDBOX_READY_TIMEOUT` (`60s`).


