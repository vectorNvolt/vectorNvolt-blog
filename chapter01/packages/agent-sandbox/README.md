# agent-sandbox (`create_agent_container`)

Go CLI that creates one isolated gVisor (`runsc`) agent container from a single `agentID`. It runs as root inside the `agent-host` distro, so it calls `docker` and `/usr/local/bin/create_agent_net` directly (no `wsl.exe` prefix). It never reads or modifies `daemon.json` or any other host configuration. Spec: `design/01_user_story/uss-002_create_agent_container.md`, `design/02_use_case/usc-002_create_agent_container.yml`.

## Steps

Keep in sync with `internal/steps/sNN_*.go`. The first failing step stops the run; nothing success-like is printed on stdout after a failure.

| Step | File | Requirement | Exit code on failure | Purpose |
|---|---|---|---|---|
| 1 | `s01_validate_id.go` | SFR-AGC-001 | 1 | `agentID` must match `^[a-z][a-z0-9-]{0,30}$`, checked before any engine call. |
| 2 | `s23_acquire_lock.go` | (lifecycle) | 1 (`NOT_CREATED`) | Non-blocking `flock` on `/run/agent-sandbox/<id>.lock`, held until the run ends (including start, pause and unpause), so `agent_lifecycle guard` can tell tool-driven starts from outside ones. A second create/remove of the same agent fails fast. |
| 3 | `s24_wait_ready.go` | (lifecycle) | 3 (`NET_NOT_READY`) | Wait up to 60 s (`AGENT_SANDBOX_READY_TIMEOUT`) until `/run/agent-sandbox/ready` exists and holds dockerd's current MainPID (`systemctl show -p MainPID --value docker.service`). Timeout: check `systemctl status agent-restore.service`. No docker call is made before this. |
| 4 | `s20_check_preconditions.go` (runs `s02`, `s03`, `s04`) | SFR-RTC-001, SFR-RTC-002, SFR-AGC-002, SFR-AGC-003, SFR-AGC-023 | 1 | In order, first failure stops, no side effect: `runsc` listed in `docker info --format '{{json .Runtimes}}'` (engine down or runtime missing is reported, never repaired); no `agent-<id>` in any state (left untouched); `agents-net`, `agent-image:latest` exist and the Workspace-Agents root exists, is a directory, is not a symlink and is owned by uid 1001. Every missing prerequisite is named; nothing is created or pulled. |
| 5 | `s05_prepare_workspace.go` | SFR-AGC-004, SFR-AGC-005 | 2 (`NOT_CREATED`) | `mkdir` `<root>/<id>` and `<root>/<id>/.output` (reused if real directories; the root is never created), then `chown` both, not recursively, to 1001:1001. |
| 6 | `s25_prepare_state.go` | (lifecycle) | 2 (`NOT_CREATED`) | Persistent Claude state: `/var/lib/agent-sandbox` and `/var/lib/agent-sandbox/<id>` root:root 0711, `/var/lib/agent-sandbox/<id>/claude` 1001:1001 0700 (created if missing, kept with its content if present; symlinks refused). Host DNS file `/etc/agent-sandbox/resolv.conf` with `nameserver 1.1.1.1` and `nameserver 9.9.9.9` (root:root 0644, rewritten if the content differs). |
| 6a | `s28_clean_leftovers.go` | SFR-AGC-016 | 2 (`NOT_CREATED`) | New step 7 of 14 (the rows below keep their old numbers; the pipeline is now 14 steps). Before the container exists: `create_agent_net disarm agent-<id>` (name-keyed, idempotent) and the gate set part of the guard reconcile (`create_agent_net released`: untagged element -> `evict <mac>`, element of a container that does not exist -> `disarm <name>`), so no leftover element exists when the container is created. Either failing stops the run before `docker create`. |
| 7 | `s06_create_start.go` | SFR-AGC-006, SSR-AGC-012 | 2 (`NOT_CREATED`, `REMOVED`, `CREATED_CLEANUP_FAILED`) | `docker create` (runsc, `--network agents-net` (closed by the agents-net gate until armed), `--pids-limit 512`, tmpfs `/tmp` and `/var/lib/docker`, memory 4g, 2 cpus, drop ALL + 15-capability allowlist, no-new-privileges, `--restart no`, binds `<ws>:/workspace:rw`, `/var/lib/agent-sandbox/<id>/claude:/home/agent/.claude:rw` with `CLAUDE_CONFIG_DIR=/home/agent/.claude`, and `/etc/agent-sandbox/resolv.conf:/etc/resolv.conf:ro`; no credential mounts) then `docker start`; on start failure remove only `agent-<id>`. |
| 8 | `s21_pause_container.go` | SFR-AGC-012, SSR-AGC-002, SFR-AGC-022 | 2 (`REMOVED`, `REMOVE_FAILED`) | `docker pause`, then inspect must show `State.Paused` true before the arming; otherwise remove only `agent-<id>` (it is not armed: its MAC is not released at the gate). |
| 9 | `s07_arm_network.go` | SFR-AGC-007, SFR-AGC-008, SSR-AGC-016, SFR-AGC-022 | 2 (`REMOVED`, `REMOVE_FAILED`, `DISARM_FAILED`) | `create_agent_net arm agent-<id>` on the paused container (it attaches nothing; it refuses a network without the gate), then `create_agent_net check-gate agent-<id>` must exit 0. On failure `disarm` runs first; only a disarm with exit 0 allows removing `agent-<id>`. If the disarm fails nothing is removed and the container stays paused behind its controls (`DISARM_FAILED`). The failed control and arm stderr are reported. No prompt. |
| 10 | `s22_release_container.go` | SFR-AGC-021, SSR-AGC-002 | 2 (`CONTAINED` / `UNSAFE`) | `docker unpause`, only after the gate check passed. A failed unpause never removes the armed container: still paused is `CONTAINED`, otherwise containment as in step 12. |
| 11 | `s18_write_resolv_conf.go` (func `CheckResolvConf`; the file name is from the former write step) | SFR-AGC-024, SFR-NET-020 | 2 (`CONTAINED` / `UNSAFE`) | Nothing is written in the container any more. `docker exec -u 0:0 <c> cat /etc/resolv.conf` must show both `nameserver 1.1.1.1` and `nameserver 9.9.9.9` and must not contain `127.0.0.11` (Docker's embedded resolver, which gVisor does not serve). Failure triggers containment. |
| 12 | `s08_verify.go` (with `s19_verify_resolv_conf.go`) | SFR-AGC-009, SFR-NET-020, SSR-AGC-011 | 2 (`CONTAINED` / `UNSAFE`) | Read the container state and compare every field: running and not paused, runtime, `Config.User` empty, `CLAUDE_CONFIG_DIR` in `Config.Env`, cap drop/add, no-new-privileges, memory, cpus, pids limit, restart policy, tmpfs, exactly three mounts (`/workspace` rw, `/home/agent/.claude` rw with the right sources, `/etc/resolv.conf` ro from `/etc/agent-sandbox/resolv.conf`), networks exactly `agents-net`; workspace and `.output` exist and are owned by 1001; `id` as `-u 1001` (uid only, so Docker keeps the supplementary group `docker`) and `id -nG` contains `docker`; `/etc/resolv.conf` read back as `-u 1001` (exactly the two lines, `root:root`, 644, not writable by uid 1001). All mismatches are listed, then containment. |
| 13 | `s09_report.go` | SFR-AGC-010 | none (exit 0) | Print the success message naming container, workspace and Claude state. |

PID 1 and the agent process: the image runs `tini` (`ENTRYPOINT ["/usr/bin/tini","--"]`) which starts `dockerd` (in-container infrastructure, `CMD ["dockerd"]`); no agent process starts at container start. The agent runs only through `docker exec -u 1001` (`enter_agent`), after pause -> arm -> check-gate -> unpause. The create path's own execs are `cat`/`stat`/`id` probes. The image is unchanged.

Containment (`containFailure`): re-pause the container and verify `State.Paused`. Paused and verified is `CONTAINED`; anything else is `UNSAFE` (possibly running). The container is never removed in either case.

SFR-AGC-011 (distinct failure status) is covered by the exit codes and the stderr failure block.

## remove_agent_container

Second binary (`cmd/remove_agent_container`), the removal counterpart. It disarms and removes one agent container and keeps the workspace unless asked. Spec: `design/01_user_story/uss-006_remove_agent_container.md`, `design/02_use_case/usc-006_remove_agent_container.yml`. Sequence: `internal/sequence/remove.go` (`removePipeline`, 10 steps). `runsc` need not be registered.

```
wsl.exe -d agent-host -u root -e remove_agent_container <agentID> [--purge-workspace] [--purge]
```

`--purge-workspace` deletes the workspace folder; `--purge` deletes the agent state `/var/lib/agent-sandbox/<id>` (Claude login and conversation history). Both are optional, each at most once, and come after `agentID`; anything else prints usage and exits 1. By default both the workspace and the agent state are kept. `agentID` is validated against the pattern before any path is built, so `--purge` can only ever delete `<state root>/<id>`.

| Step | File | Requirement | Exit code on failure | Purpose |
|---|---|---|---|---|
| 1 | `s10_validate_remove_args.go` | SFR-AGC-013 | 1 (`NOT_REMOVED`) | `agentID` pattern as for create, optional `--purge-workspace` and `--purge`, checked before any engine call. |
| 2 | `s23_acquire_lock.go` | (lifecycle) | 1 (`NOT_REMOVED`) | The same per-agent `flock` as create, held for the whole run. |
| 3 | `s11_check_engine.go` | SFR-AGC-014 | 1 (`NOT_REMOVED`) | The engine must answer. |
| 4 | `s12_locate_container.go` | SFR-AGC-015 | 2 (`NOT_REMOVED`) | Look up `agent-<id>`. If absent, steps 5-7 are skipped and the run continues at step 8. |
| 5 | `s13_disarm_network.go` | SFR-AGC-016, SSR-AGC-016 | 1 (`NOT_REMOVED`) if `create_agent_net` cannot be run, 2 (`DISARM_FAILED`) if `disarm` exits non-zero | `create_agent_net disarm agent-<id>` before anything is removed; on failure the container stays behind the gate and a re-run retries. `agent_lifecycle restore` runs this path in a tolerant mode (a failed disarm is logged and the removal goes on, because the gate is rebuilt right after). |
| 6 | `s14_remove_container.go` | SFR-AGC-017 | 2 (`REMOVE_FAILED`) | `docker rm -f agent-<id>`; nothing else is removed. |
| 7 | `s15_verify_removed.go` | SFR-AGC-018 | 2 (`REMOVE_FAILED` if still there, `UNVERIFIED` if unreadable) | Read the engine again; the container must be gone. |
| 8 | `s16_handle_workspace.go` | SFR-AGC-019, SSR-AGC-015 | 2 (`WORKSPACE_PURGE_FAILED`) | Keep `<root>/<id>`, or with `--purge-workspace` delete exactly it. |
| 9 | `s26_handle_state.go` | (lifecycle) | 2 (`STATE_PURGE_FAILED`) | Keep `/var/lib/agent-sandbox/<id>`, or with `--purge` delete exactly it (same guards as the workspace purge: real directory, no symlink, directly under the real state root). |
| 10 | `s17_report_removed.go` | SFR-AGC-020 | none (exit 0) | Print the exit state and the workspace and state results on stdout. |

Workspace rules:

- Default: the folder is kept and its path is reported.
- `--purge-workspace` deletes `<root>/<id>` recursively only if it is a real directory (not a symlink or file), the root is not a symlink and the resolved folder is directly under the resolved root (not the root itself). Otherwise nothing is deleted and the run fails with `WORKSPACE_PURGE_FAILED`.
- An absent folder is noted (`workspace not present, nothing to purge`), not an error.
- The purge also runs when no container exists, so it finishes a removal whose purge failed earlier. It never runs after a failure in steps 4-6.

| Exit code | Exit state | Meaning |
|---|---|---|
| 0 | `REMOVED` | Container disarmed, removed and verified gone |
| 0 | `NOT_PRESENT` | No container `agent-<id>` existed |
| 1 | `NOT_REMOVED` | Usage, engine unavailable, or `create_agent_net` not runnable; nothing changed |
| 2 | `NOT_REMOVED` | Container lookup failed (step 3); nothing changed |
| 2 | `DISARM_FAILED` | Port not closed; container left in place behind the gate |
| 2 | `REMOVE_FAILED` / `UNVERIFIED` | Removal failed, or the container is still there / could not be checked; workspace untouched |
| 2 | `WORKSPACE_PURGE_FAILED` | Container is gone, purge refused or failed |
| 2 | `STATE_PURGE_FAILED` | Container is gone, `--purge` of the agent state refused or failed |

Every failure is safe to re-run. Uses the same `AGENT_LOG_LEVEL`, `AGENT_SANDBOX_WORKSPACE_ROOT` and `AGENT_SANDBOX_ARM_BIN` variables as create. Build:

```
GOOS=linux GOARCH=amd64 go build -o remove_agent_container ./cmd/remove_agent_container
```

## enter_agent

Fourth binary (`cmd/enter_agent`, runs as root in `agent-host`). Opens a session in `agent-<id>`, creating or recreating the container first when needed. Sequence: `internal/sequence/enter.go` (`RunEnter`). It replaces the runbook one-liner around `docker exec -it -u 1001 agent-<id> claude`.

```
wsl.exe -d agent-host -u root -e enter_agent <agentID> [-- <cmd> [args...]]
```

Anything else (no id, invalid id, extra arguments, `--` without a command) prints usage and exits 1. Not root: exit 1.

Behaviour, by the state of `agent-<id>` (`docker container inspect --format {{.State.Status}}`):

| State | Action |
|---|---|
| `running` | `create_agent_net check-gate agent-<id>`. On failure the report lines and a hint (`remove_agent_container <id>`, then retry) go to stderr and the exit code of `check-gate` is returned. A running container is never removed. |
| absent | Create sequence (same code as `create_agent_container`, its progress is printed as usual). |
| anything else (`exited`, `paused`, `created`, `dead`, `restarting`) | Remove sequence (as `remove_agent_container <id>`, no purge: workspace and agent state are kept), then the create sequence. |

After that the process is replaced (`syscall.Exec`) by `docker exec -it -u 1001 agent-<id> <cmd>` (`-i` instead of `-it` when stdin is not a terminal). One line `entering agent-<id> (<cmd>)` goes to stderr first. `<cmd>` is what follows `--` if given. Otherwise it is `claude --continue` when a previous conversation exists (any `*.jsonl` in `<state root>/<id>/claude/projects/*/`), else `claude`.

| Exit code | Exit state | Meaning |
|---|---|---|
| (exec) | none | The process became `docker exec`; its exit code is the command's |
| 1 | `NOT_ENTERED` | Usage or not root |
| 1 or 2 | `NOT_ENTERED` | Running container failed `check-gate`; the exit code of `check-gate` is returned |
| 2 | `NOT_ENTERED` | State or gate check could not be run, docker not found, or exec failed |
| any | as create / remove | A remove or create step failed; its exit code and failure report are returned and nothing is exec'd |

Uses the same variables as create (`AGENT_SANDBOX_STATE_ROOT` for the conversation lookup). Build: `GOOS=linux GOARCH=amd64 go build -o enter_agent ./cmd/enter_agent`.

## agent_lifecycle

Third binary (`cmd/agent_lifecycle`, runs as root in `agent-host`, run by two systemd units: `agent-restore.service` and `agent-guard.service`). Build: `GOOS=linux GOARCH=amd64 go build -o agent_lifecycle ./cmd/agent_lifecycle`. Sequences: `internal/sequence/restore.go`, steps `internal/steps/s27_restore.go`. The unit files are not in this package: `create_agent_host.exe` (`packages/agent-host`, step 11, `internal/steps/lifecycle_units.go`) writes, enables and starts them.

| Command | Purpose |
|---|---|
| `agent_lifecycle restore` | After each docker start (the nft/iptables gate is gone and all containers are dead). Steps: must be root (exit 1 `NOT_RESTORED`); remove `/run/agent-sandbox/ready` first; wait up to 60 s (`AGENT_SANDBOX_DOCKER_WAIT`) for the docker daemon and note dockerd's MainPID; remove every container named `agent-*` (any state) through the remove path (tolerant disarm; the agent state directories are never purged); run `remove_agent_net` (a failure, e.g. network absent, is only a warning) then `create_agent_net` (must exit 0); require network `agents-net` to exist; write the marker `/run/agent-sandbox/ready` (dir 0755, file 0644) containing dockerd's MainPID, unless dockerd restarted meanwhile. Any failure exits 2 (`RESTORE_FAILED`, or `NOT_RESTORED` when the daemon never answered) and leaves no marker. Success prints `exit_state: RESTORED`. |
| `agent_lifecycle guard` | Long-running. Follows `docker events` for container `start`, `unpause`, `die`, `update` and network `connect`, `disconnect` (a network event carries the container ID in `Actor.Attributes.container`; it is resolved to a name, and any connect/disconnect of an `agent-*` container counts, whatever the network). For `agent-<id>`: try the per-agent lock without blocking; held means a create/remove is running, ignore. If free, the guard keeps the lock until it is done (the remove path is run with `LockHeld`, it takes no lock of its own). `die`: run `create_agent_net disarm <name>` (the container stays). Other events: skip a container that no longer exists or whose `create_agent_net check-gate` passes; on a clean gate failure (exit 2) run the remove path for `<id>` (never `--purge`) and log one line to stderr (the journal). Errors (the existence check fails, or check-gate cannot run or exits 1) fail closed: `disarm <name>` first, then `docker rm` only when the disarm exited 0; if the disarm failed the container is left in place, paused if possible; both outcomes are logged. Reconcile: once at start and every `AGENT_SANDBOX_GUARD_RECONCILE` (default 60 s) every `agent-*` container whose lock is free gets the same check, then `create_agent_net released` is read: an element without comment -> `evict <mac>`, an element whose container does not exist -> `disarm <name>`. If `docker events` ends the guard exits 1 so systemd restarts it. |

Paths: ready marker `/run/agent-sandbox/ready`, per-agent locks `/run/agent-sandbox/<id>.lock`, agent state `/var/lib/agent-sandbox/<id>/claude` (mounted at `/home/agent/.claude`, the home of user `agent` in `packages/agent-image/Dockerfile.agent`), host DNS file `/etc/agent-sandbox/resolv.conf`.

## How to build / run

```
GOOS=linux GOARCH=amd64 go build -o create_agent_container ./cmd/create_agent_container
```

```
wsl.exe -d agent-host -u root -e create_agent_container <agentID>
```

A missing or extra argument prints usage and exits 1.

| Exit code | Meaning |
|---|---|
| 0 | Container created, armed, released and verified |
| 1 | Usage, lock held, or prerequisite not met (steps 1, 2, 4); nothing created (`NOT_CREATED`) |
| 2 | Runtime failure (steps 5-12) |
| 3 | `NET_NOT_READY` (step 3): `/run/agent-sandbox/ready` missing or not holding dockerd's current MainPID after 60 s; nothing was touched. Check `systemctl status agent-restore.service`. |

Exit states on failure: `NET_NOT_READY`, `NOT_CREATED`, `REMOVED`, `REMOVE_FAILED`, `CREATED_CLEANUP_FAILED`, `DISARM_FAILED` (container left paused behind its controls), `CONTAINED` (re-paused, not removed), `UNSAFE` (could not be paused and verified, possibly running).

## Configuration

| Variable | Default |
|---|---|
| `AGENT_LOG_LEVEL` | `INFO` (`DEBUG` also traces each command) |
| `AGENT_SANDBOX_WORKSPACE_ROOT` | `/home/agent-operator/Workspace-Agents` |
| `AGENT_SANDBOX_ARM_BIN` | `/usr/local/bin/create_agent_net` |
| `AGENT_SANDBOX_REMOVE_NET_BIN` | `/usr/local/bin/remove_agent_net` (restore only) |
| `AGENT_SANDBOX_RUNTIME_DIR` | `/run/agent-sandbox` (ready marker, locks) |
| `AGENT_SANDBOX_STATE_ROOT` | `/var/lib/agent-sandbox` |
| `AGENT_SANDBOX_HOST_RESOLV` | `/etc/agent-sandbox/resolv.conf` |
| `AGENT_SANDBOX_READY_TIMEOUT` | `60s` (create's wait for the ready marker; Go duration) |
| `AGENT_SANDBOX_DOCKER_WAIT` | `60s` (restore's wait for the docker daemon; Go duration) |

## Tests

```
go test ./...
```

## Layout

| Here | Role |
|---|---|
| `cmd/create_agent_container/`, `cmd/remove_agent_container/`, `cmd/agent_lifecycle/`, `cmd/enter_agent/` | entry points |
| `internal/config/` | constants (runtime, caps, limits) and environment overrides |
| `internal/services/` (`docker`, `arm`, `fs`, `render`) | process runner, docker/arm adapters, filesystem interface, console output |
| `internal/steps/` | one file per step |
| `internal/sequence/` | numbered step list, first failure sets the exit code |
| `tests/` | sequence tests with a fake runner and a temp-dir filesystem |

## Open points

- `create_agent_net arm <container>` (`packages/agent-net`) is implemented. It attaches nothing (the container is created on `agents-net`), applies the host-side controls (`nft`, `iptables`, `bridge`, `nsenter`), verifies them. Its exit codes are 0 (armed), 1 (usage or precondition) and 2 (apply or verify failure); on failure the first stderr line is `failed control: <name>: <detail>`. Step 6 does not distinguish 1 from 2: any non-zero exit or failure to run the binary (`AGENT_SANDBOX_ARM_BIN`, default `/usr/local/bin/create_agent_net`), or a failing `check-gate`, counts as an arming failure; it runs `disarm` first and removes only `agent-<id>` after a successful disarm. The arm tool has no rollback of its own.
- The readiness gate is the per-port nft table `as_port_<id>` plus tagged iptables rules (see `packages/agent-net/README.md`). They do not survive `wsl --shutdown` or a reboot, while `agents-net` does (`create_agent_net check-gate <container>` reports the state of one port). Containers are created with `--restart no` (verified in step 9), so Docker never revives them. WSL also stops the distro when no `wsl.exe` session is attached, which kills the container the same way as a restart; launch via `howto/README.md` section 9 so the attached session keeps it up. After a restart `agent_lifecycle restore` removes the old containers and rebuilds the gate, and create waits for its ready marker (`NET_NOT_READY` otherwise); a container started outside the tools (`docker start`/`restart`) is removed by `agent_lifecycle guard`. Do not `docker start` an old agent container by hand. Step 4 creates the container on `agents-net` and starts it, step 5 pauses it, and `arm` (step 6) only applies the controls and releases the port. The gate (`agents_gate` table, installed by `create_agent_net` when it creates `agents-net`) keeps the port closed until `arm` adds the agent MAC to its set `released`; `arm` does not recreate a missing gate, it refuses the network (recreate it once: `remove_agent_net`, then `create_agent_net`). The container is unpaused only in step 7, after `check-gate` passed.
- Exactly three mounts, no credential mounts: the workspace (rw), the per-agent Claude state (rw, so login and conversation history survive recreating the container) and the host resolv.conf (ro); step 12 requires exactly these. The state directory is owned by 1001:1001 (the container user `agent`); its parents are root:root 0711 so other agents cannot list it.
- Verified on the real host (2026-10-31): `docker network connect` is rejected while `none` is attached, and gVisor never sees an interface added after the start. Hence the container is created with `--network agents-net` and held closed by the agents-net gate (`packages/agent-net/README.md`); `arm` does not attach.
- The git worktree `/workspace/src` is not created by any step.
- `SYS_ADMIN` is granted per decision; `NET_ADMIN` is not. It can be added later if the ephemeral (nested Docker daemon) container needs it; the suspected need is loopback bring-up, unmeasured. Without `NET_ADMIN` the agent cannot alter its own network attachment by that route.
- Open runtime item (SFR-AGC-024): `/etc/resolv.conf` is now a read-only bind mount of a host file (Docker's generated file reverted to `127.0.0.11`, which gVisor does not serve, on every start). That this bind mount works under `runsc`, and the checks of steps 11 and 12 against it, are still unverified on the real host; so are the `.claude` bind mount and `CLAUDE_CONFIG_DIR` handling.
- After a failure in steps 10-12 the container is never removed; it is left paused (`CONTAINED`) or reported `UNSAFE`. Only failures in steps 7-9 (start, pause, arm) trigger removal of `agent-<id>`, and for arming only after a successful disarm.
