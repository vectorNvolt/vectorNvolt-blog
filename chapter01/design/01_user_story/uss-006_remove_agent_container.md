# USS-006 — Remove a gVisor agent container

## Participants

| Participant | Role |
|---|---|
| Developer | Operates `agent-host` (created by USS-001) from host Windows |
| Container removal tool | Go executable `remove_agent_container`, run as root inside `agent-host`, that removes one agent container |
| `agent-host` | WSL2 distro from USS-001; hosts the Docker engine |
| Docker engine | Looks up and removes the container |
| Agent container | The sandbox `agent-<agentID>` created by USS-002 |
| Agent network control | Subcommand `create_agent_net disarm <container>` (USS-004) that removes the `released` elements, netdev table, iptables rules, neighbour entries and FDB entries of one container name |

## Goal

As a developer, I want to remove one agent container by running a single Go executable with an `agentID`, so that the container is disarmed and removed in the right order and its workspace and its Claude state (login and history) are kept unless I ask for them to be deleted.

## Invocation

```
wsl.exe -d agent-host -u root -e remove_agent_container <agentID> [--purge-workspace] [--purge]
```

The tool is meant to run as root: the disarm step needs `CAP_NET_ADMIN` in the host network namespace and access to the Docker socket. Running it with `-u root` from host Windows means `agent-operator` needs neither sudo nor `docker` group membership. This is a convention: the tool does not check its own user id, and a non-root run fails at the first engine or disarm call that needs the privilege.

| Argument | Required | Meaning |
|---|---|---|
| `agentID` | yes | Identifier of the agent; names the container and the workspace folder (same rule as USS-002) |
| `--purge-workspace` | no | Also delete the agent's own workspace folder |
| `--purge` | no | Also delete the agent state `/var/lib/agent-sandbox/<agentID>` (Claude login and conversation history); combinable with `--purge-workspace` |

`agentID` must match `^[a-z][a-z0-9-]{0,30}$`. The container is `agent-<agentID>`. The tool takes the `agentID` followed by each option at most once; any other argument, repetition or count, or a missing `agentID`, is a usage error that prints `usage: remove_agent_container <agentID> [--purge-workspace] [--purge]`.

## Build and delivery (Go)

- `remove_agent_container` is a static Go binary for `linux/amd64`, built in `dev-distro`.
- No Go toolchain is needed on `agent-host`.
- Delivery is owned by USS-001: `create_agent_host` installs the binary to `/usr/local/bin/remove_agent_container` (`root:root`, mode 755) and checks its SHA-256.

## Preconditions

- `agent-host` exists, Docker is installed and the Docker engine answers.
- `create_agent_net` is installed at `/usr/local/bin/create_agent_net` and offers `disarm <container>` (USS-004).
- `runsc` need not be registered: removing a container does not need the isolated runtime.
- The container need not exist: a missing container is not an error.

## Interaction

The tool has 10 numbered steps (progress lines `[n/10]`); the story keeps 8 interaction steps: the per-agent lock is part of step 1 and the agent state handling is part of step 7.

1. Developer runs `remove_agent_container <agentID> [--purge-workspace] [--purge]` as root inside `agent-host`, from host Windows. The tool validates the arguments and takes the per-agent lock `/run/agent-sandbox/<agentID>.lock` (non-blocking `flock`, held for the whole run; a held lock, i.e. a create or remove of the same agent still running, fails the run with exit 1, `NOT_REMOVED`).
2. Tool checks that the Docker engine answers.
3. Tool looks up `agent-<agentID>`. If it does not exist, steps 4 to 6 are skipped and the run continues at step 7, without a disarm: the network state left by a container that is gone is removed by the guard (on `die` and at its reconcile) or by `create_agent_container` before `docker create`.
4. Tool calls `create_agent_net disarm agent-<agentID>`, which deletes, name-keyed, the elements of the gate set `released` tagged with the container name (and its MACs), the port's `as_port_<id>` netdev table, its tagged iptables rules, its neighbour entries and its static FDB entries; it never touches the gate itself. The container is not removed before this call succeeded. If the guard already disarmed the port on the container's `die`, the disarm is repeated here (idempotent).
5. Tool removes the container (`docker rm -f agent-<agentID>`); a paused container is unpaused first, after the disarm, because `runsc` cannot kill a paused sandbox.
6. Tool checks again that `agent-<agentID>` no longer exists.
7. Tool handles the workspace folder `/home/agent-operator/Workspace-Agents/<agentID>`: it keeps it and reports its path, or, with `--purge-workspace`, deletes exactly that folder. It then handles the agent state `/var/lib/agent-sandbox/<agentID>`: it keeps it by default (so Claude login and history survive recreating the container), or, with `--purge`, deletes exactly that directory (`STATE_PURGE_FAILED` on failure).
8. Tool reports the outcome on stdout and exits 0.

The tool prints a progress line `[n/10] <step name>` on stdout before each step.

The same removal path is used by `agent_lifecycle` (USS-001 units): `restore` removes every `agent-*` container after a dockerd start with a tolerant disarm (a failed disarm is logged and the removal goes on, because the gate is rebuilt right after), `guard` uses the strict path: it removes a container only after a disarm that exited 0 and otherwise leaves it in place. The guard follows container start, unpause, die, update and network connect and disconnect: on `die` it disarms the name; on restart, unpause, connect and disconnect (and the other events) it runs `check-gate` and removes the container on a clean failure; on an error it disarms first and removes only after a successful disarm. It also reconciles at start-up and every 60 s. It holds the per-agent lock through check and removal. Neither ever purges.

## Workspace

- Default: the folder is kept and its path is reported.
- `--purge-workspace` deletes `/home/agent-operator/Workspace-Agents/<agentID>` recursively, and only if all of these hold:
  - it is a real directory (not a symlink or file);
  - the workspace root `Workspace-Agents` is not a symlink;
  - its resolved path is directly under the resolved root and is not the root itself.
- Otherwise nothing is deleted and the run fails (exit 2, `WORKSPACE_PURGE_FAILED`).
- An absent folder is noted, not an error.
- The purge runs even when no container exists, so it finishes a removal whose purge failed earlier.
- The purge runs only after the container is verified gone; a failure in steps 4 to 6 never touches the workspace.

## Agent state

- Default: `/var/lib/agent-sandbox/<agentID>` (Claude login and conversation history, mounted at `/home/agent/.claude`) is kept and its path is reported.
- `--purge` deletes exactly that directory under the same guards as the workspace purge (real directory, no symlink, directly under the real state root). Failure: exit 2, `STATE_PURGE_FAILED`. An absent directory is noted, not an error; the purge also runs when no container exists.

## Acceptance criteria

- `remove_agent_container <agentID>` exits 0 and the container `agent-<agentID>` does not exist.
- Stdout carries exit state `REMOVED` (or `NOT_PRESENT` when no container existed) and whether the workspace and the agent state were kept, purged or not present.
- `/var/lib/agent-sandbox/<agentID>` still exists unless `--purge` was given.
- The container's port was disarmed before the container was removed (not for an absent container, which is not disarmed here).
- `/home/agent-operator/Workspace-Agents/<agentID>` still exists unless `--purge-workspace` was given.
- No other container, network, image, workspace folder, `daemon.json` or Windows host resource is changed.
- Re-running after a failure is safe: disarm is idempotent and a missing container is not an error.

## Failure cases

| Case | Exit | Exit state | Behaviour |
|---|---|---|---|
| Step 1: missing or invalid `agentID`, or an option other than `--purge-workspace` and `--purge` | 1 | `NOT_REMOVED` | Usage error; nothing is changed |
| Step 1: per-agent lock held | 1 | `NOT_REMOVED` | Report and stop; nothing is changed |
| Step 2: Docker engine unavailable | 1 | `NOT_REMOVED` | Report and stop |
| Step 3: container lookup fails | 2 | `NOT_REMOVED` | Report and stop; nothing is changed |
| Step 4: `create_agent_net` cannot be run (binary missing) | 1 | `NOT_REMOVED` | Report and stop; nothing is removed |
| Step 4: `create_agent_net disarm` exits non-zero | 2 | `DISARM_FAILED` | Its stderr is reported; the container is not removed; its port stays locked and isolated; a re-run retries |
| Step 5: unpause of a paused container or `docker rm -f` fails | 2 | `REMOVE_FAILED` | Engine error reported; workspace not touched |
| Step 6: container still exists after removal | 2 | `REMOVE_FAILED` | Reported; workspace not touched |
| Step 6: absence cannot be verified (engine state unreadable) | 2 | `NOT_REMOVED` | Reported; workspace not touched |
| Step 7: `--purge-workspace` and the folder or root is a symlink, the path resolves outside the root, or deletion fails | 2 | `WORKSPACE_PURGE_FAILED` | Reported; nothing outside the agent's own folder is deleted; the container is already gone |
| Step 7: `--purge` and the state directory or its root is a symlink, the path resolves outside the state root, or deletion fails | 2 | `STATE_PURGE_FAILED` | Reported; nothing outside the agent's own state directory is deleted; the container is already gone |
| Rollback | n/a | n/a | None; every failure is safe to re-run |

The netdev tables and iptables rules are not kept across `wsl --shutdown`; `disarm` then finds nothing to disarm and exits 0. After a dockerd start `agent_lifecycle restore` has already removed the old agent containers, so a removal then usually reports `NOT_PRESENT`.
