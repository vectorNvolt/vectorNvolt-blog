# USS-001 — Configure the `agent-host` foundation VM

Revision 2026-11-22 (`design/local-vm-migration-plan.md`): the WSL2 distro is replaced by a standard Ubuntu 26.04 foundation VM. The Go tool now runs inside the VM as root and customizes it. WSL, `wsl.exe`, the rootfs import, `wsl.conf`, the isolation units and the keep-alive are removed.

## Participants

| Participant | Role |
|---|---|
| Developer | Owns the machine (macOS, Linux or Windows) that runs the VM; works in the VM over a shell, SSH or VS Code Remote-SSH |
| VM tool | Lima (macOS / Linux, `vm/agent-host.lima.yaml`) or Multipass (Windows, `vm/agent-host.cloud-init.yaml`); a cloud provider can use the same cloud-init file |
| Provisioning tool | Go executable `create_agent_host`, native linux build, run as root inside the VM, that configures it |
| `agent-host` | The foundation VM itself (hostname `agent-host`), dedicated to agent operation |

## Goal

As a developer, I want to turn a standard foundation VM into the `agent-host` by running a single Go executable inside it, so that agents run in a separate environment from my own machine, with all required dependencies already installed.

Isolation claim (precise):

- `agent-host` is a VM: its own kernel, its own filesystem and its own network stack, separate from the reader's machine.
- It has no host mounts (Lima `mounts: []`, no Multipass mounts) and no host interop; files move only by explicit transfer (`limactl copy`, `multipass transfer`, `scp`, `rsync`, `git`).
- The only way in is SSH with a key (password and keyboard-interactive login off, root login off, agent forwarding off; hardened by `vm/foundation.sh`) or the VM tool's own shell.
- Containers on `agents-net` cannot reach the VM's own sshd: `create_agent_net arm` installs an INPUT drop per container, verified exactly by `check-gate` (USS-004).
- Not verified: Lima on macOS (Apple Virtualization) is untested.

## Foundation VM (precondition)

The VM is created from the aligned config files in `vm/` (single source of truth `vm/foundation.sh`, generated into both files by `vm/generate.sh`):

| Reader's machine | Tool | Config file |
|---|---|---|
| macOS / Linux | Lima | `vm/agent-host.lima.yaml` |
| Windows | Multipass (Hyper-V) | `vm/agent-host.cloud-init.yaml` |
| Cloud (later) | provider | `vm/agent-host.cloud-init.yaml` |

Every foundation VM guarantees (`vm/check-foundation.sh` asserts it, `FOUNDATION_VERSION=1`): Ubuntu 26.04 LTS stock image and kernel (amd64 or arm64); systemd running and cloud-init done; a login user with passwordless `sudo` and SSH key login; `git`, `ca-certificates`, `curl`, `build-essential`; the pinned Go toolchain in `/usr/local/go` (sha256-verified per architecture); the marker `/etc/agent-foundation` containing `FOUNDATION_VERSION=1`; nothing else (no Docker, no containerd, no host mounts). `create_agent_host` checks the marker first and refuses to run on an unknown foundation.

## Build in the VM (precondition)

All Go executables are built natively inside the VM with the foundation's Go toolchain (`CGO_ENABLED=0`); nothing is built on the reader's machine, nothing is cross-compiled and no Windows binaries exist. There are four Go modules, all written to one folder `dist/` at the repo root (the repository checkout is in the VM). In bash, from the repo root:

```bash
mkdir -p dist
(cd packages/agent-host && CGO_ENABLED=0 go build -o ../../dist/create_agent_host ./cmd/create_agent_host)
(cd packages/agent-image && CGO_ENABLED=0 go build -o ../../dist/create_agent_image ./cmd/create_agent_image)
(cd packages/agent-net && CGO_ENABLED=0 go build -o ../../dist/create_agent_net ./cmd/create_agent_net)
(cd packages/agent-net && CGO_ENABLED=0 go build -o ../../dist/remove_agent_net ./cmd/remove_agent_net)
(cd packages/agent-net && CGO_ENABLED=0 go build -o ../../dist/refresh_endpoints ./cmd/refresh_endpoints)
(cd packages/agent-sandbox && CGO_ENABLED=0 go build -o ../../dist/create_agent_container ./cmd/create_agent_container)
(cd packages/agent-sandbox && CGO_ENABLED=0 go build -o ../../dist/remove_agent_container ./cmd/remove_agent_container)
(cd packages/agent-sandbox && CGO_ENABLED=0 go build -o ../../dist/agent_lifecycle ./cmd/agent_lifecycle)
(cd packages/agent-sandbox && CGO_ENABLED=0 go build -o ../../dist/enter_agent ./cmd/enter_agent)
```

| Module | `cmd/` package | Binary |
|---|---|---|
| `packages/agent-host` | `create_agent_host` | `create_agent_host` |
| `packages/agent-image` | `create_agent_image` | `create_agent_image` |
| `packages/agent-net` | `create_agent_net`, `remove_agent_net`, `refresh_endpoints` | same names |
| `packages/agent-sandbox` | `create_agent_container`, `remove_agent_container`, `agent_lifecycle`, `enter_agent` | same names |

`create_agent_host` is run from the folder holding the seven tool builds (`create_agent_container`, `remove_agent_container`, `create_agent_net`, `remove_agent_net`, `agent_lifecycle`, `enter_agent`, `refresh_endpoints`) and `endpoints.json` (default: the folder of the executable; `AGENT_HOST_TOOLS_DIR` overrides). `Dockerfile.agent` and `requirements.lock` (USS-003) are used by `create_agent_image`, not by `create_agent_host`. `endpoints.json` is the model endpoint list, `{"endpoints":["api.anthropic.com","platform.claude.com"]}` by default; it is a hand-kept file, not a build output.

`dist/` is untracked (repo `.gitignore`, `/dist/`); binaries are not committed.

## Interaction

1. Developer runs `sudo create_agent_host` in a shell inside the VM (`limactl shell agent-host`, `multipass shell agent-host`, or SSH). It takes no arguments. The tool first checks the foundation: it runs on Linux, as root, and `/etc/agent-foundation` contains `FOUNDATION_VERSION=1`; otherwise it stops with exit 1 and names `vm/check-foundation.sh`. It also refuses with exit 1 (nothing changed) when the marker lacks `UBUNTU=26.04`, when `/proc/mounts` lists a `virtiofs`, `9p`, `sshfs`, `fuse.sshfs` or `fuse.reverse-sshfs` mount, when `/etc/agent-host` exists, when `agent-operator` exists, or when UID or GID 1001 is taken. A rerun is refused, not repaired.
2. Tool reads the seven tool builds and `endpoints.json` from its folder and computes the SHA-256 of each build (reference hashes), before anything is changed.
3. Tool creates the non-root user `agent-operator` (no sudo, not in the `docker` group) with an explicit pinned numeric UID and GID, not the next free ones. The value is 1001, one shared value, identical to the UID of the non-root user in the agent image (USS-003). The foundation's login user is not removed or changed; the creation fails if UID or GID 1001 is already taken.
4. Tool sets the hostname `agent-host` (`hostnamectl`), writes the cloud-init drop-in `/etc/cloud/cloud.cfg.d/99-agent-host-hostname.cfg` with `preserve_hostname: true` (`root:root`, 644) so the name survives a reboot, and adds `127.0.1.1 agent-host` to `/etc/hosts` when no entry names it. (Replaces the former `wsl.conf` step.)
5. Tool waits for systemd to finish starting, installs the dependencies (see below), including gVisor `runsc` from the gVisor apt repository, whose signing key must match a pinned fingerprint before the repository is trusted. It then registers `runsc` as a Docker runtime with `runsc install` (writes the runtime into `/etc/docker/daemon.json`) and restarts `dockerd` (`systemctl restart docker`). USS-001 is the only owner of this registration; USS-002, USS-003 and USS-004 never touch `daemon.json`. Before the restart it persists and loads `br_netfilter` with `net.bridge.bridge-nf-call-iptables` and `-ip6tables` set to 1 (`/etc/modules-load.d/agent-host.conf`, `/etc/sysctl.d/90-agent-host-bridge.conf`, `modprobe`, `sysctl -p`), since agent-net only reads that value. It also writes `/etc/apt/apt.conf.d/51agent-host-no-docker-upgrade` (`root:root`, 644), which blacklists `docker.io`, `docker-buildx`, `containerd`, `runc` and `runsc` from unattended-upgrades.
6. Tool installs the seven Linux tools `create_agent_container` and `remove_agent_container` (USS-002, USS-006), `create_agent_net` and `remove_agent_net` (USS-004, USS-007), `agent_lifecycle` (restore and guard, see step 6a), `enter_agent` and `refresh_endpoints` (model endpoint list, see step 6b) from its folder into `/usr/local/bin/` (owner `root:root`, mode 755; they run as root, so only root may replace them), then compares the SHA-256 of the installed file with the reference hash from step 2, for each binary. Each file is written by a shell command that takes the bytes on stdin (`/bin/sh -c 't="$1.tmp" && cat > "$t" && chown root:root "$t" && chmod "$2" "$t" && mv -f "$t" "$1"' sh /usr/local/bin/<name> 755`; `install /dev/stdin` is not used because Ubuntu 26.04 ships uutils coreutils, whose `install` fails to read `/dev/stdin`).
   6a. In the same step the tool writes two systemd units (`root:root`, mode 644): `/etc/systemd/system/agent-restore.service` (oneshot, `RemainAfterExit`, `Requires`/`After`/`PartOf` `docker.service`, runs `agent_lifecycle restore`, `ExecStopPost` removes `/run/agent-sandbox/ready`, `WantedBy` `docker.service` and `multi-user.target`) and `/etc/systemd/system/agent-guard.service` (simple, `Restart=always`, runs `agent_lifecycle guard`). It runs `systemctl daemon-reload`, enables `docker.service` and both units and starts them. After every dockerd start `agent-restore.service` removes the old `agent-*` containers, rebuilds the agents-net gate and writes the ready marker that `create_agent_container` waits for (USS-002, USS-004); `agent-guard.service` follows container start, unpause, die and update and network connect and disconnect: on `die` it disarms the name, on the other events it runs `check-gate` and removes the container on a clean failure; on an error it disarms first and removes only after a successful disarm; it also reconciles at start-up and every 60 s (check-gate of every agent container whose lock is free, eviction of untagged `released` elements, disarm of names whose container is gone). The units are enabled, so they come back at every boot of the VM; there is no keep-alive.
   6b. After the tools and units are in place, the tool seeds `/etc/agent-net/endpoints.json` from the `endpoints.json` in its folder: it sends the file's bytes on stdin to `/usr/local/bin/refresh_endpoints`, never by path. It then checks the file is `root:root` mode 644. A seed failure fails creation. The file was already read and validated at the start (see below), before anything is changed.
7. Tool verifies the result (see acceptance criteria), writes the done marker `/etc/agent-host` (`AGENT_HOST_VERSION=1`, `root:root`, 644) and reports success or failure; on failure no marker is written.

Withdrawn 2026-11-22 (WSL2 path removed): the WSL version gate, the check that no distro `agent-host` is registered, the rootfs download and signature check, `wsl --import`, `wsl.conf`, `wsl --terminate` and the keep-alive.

`endpoints.json` must be in the same folder as the tools: JSON `{"endpoints":[...]}`, a non-empty list of unique host names (default `api.anthropic.com`, `platform.claude.com`). The tool reads and validates it together with the tool-build prerequisite check, before anything is changed. If it is missing or invalid, the run stops there with exit 1.

Later endpoint updates are not part of this story: edit `endpoints.json` and run `sudo refresh_endpoints < endpoints.json` in the VM.

No rollback: on failure the tool reports and stops; the developer deletes the half-configured VM (`limactl delete agent-host` or `multipass delete --purge agent-host`) and recreates it from the foundation config.

## Isolation — withdrawn `wsl.conf` settings

Withdrawn 2026-11-22 (WSL2 path removed): the `/etc/wsl.conf` settings (`[automount] enabled=false`, `[interop] enabled=false`, `appendWindowsPath=false`, `[user] default=agent-operator`, `[boot] systemd=true`) and the isolation units for `/mnt/wsl`, `WSLInterop` and `dxg`. The VM boundary replaces them: no host mounts and no host interop by construction, systemd from the stock image.

> Isolation scope: the VM boundary (own kernel, filesystem and network stack, no host mounts, SSH by key only). Containers on `agents-net` cannot reach the VM's sshd.

## Dependencies

Docker and gVisor are configured on the VM only; nothing changes on the reader's machine.

- Docker engine (`docker.io` and `docker-buildx`; service enabled and started)
- gvisor (`runsc`, registered as a Docker runtime via `runsc install`)
- Python 3
- gnome-keyring
- git (available on `agent-host` only; not in the agent container)
- tar
- nftables (the `nft` binary; used by `create_agent_net arm` (USS-004) for the per-port netdev tables)
- iptables (host-side `DOCKER-USER` / `INPUT` rules set by `create_agent_net` (USS-004))
- iproute2 (the `ip` and `bridge` binaries; used by `create_agent_net` (USS-004) for the bridge port flags, the static FDB entry and the veth lookup)
- ca-certificates, curl and gnupg (installed by `install_deps.sh`; used to fetch the gVisor apt repository key and check it against the pinned fingerprint)

## Connection and data transfer

The VM has no host mounts. Ways in and files:

| Aspect | Lima (macOS / Linux) | Multipass (Windows) |
|---|---|---|
| Shell | `limactl shell agent-host` | `multipass shell agent-host` |
| Plain SSH | `Include` the Lima `ssh.config` in `~/.ssh/config`, then `ssh lima-agent-host` (127.0.0.1 only) | own key via cloud-init, alias in `%USERPROFILE%\.ssh\config` pointing at the VM IP (`multipass info`); Hyper-V NAT, not reachable from the LAN |
| VS Code | Remote-SSH to `lima-agent-host` | Remote-SSH to the alias |
| Files | `limactl copy`, `scp`, `rsync` | `multipass transfer`, `scp`, `rsync` |
| Code in | `git clone` inside the VM (readers) or `git push` to a bare repository in the VM (developers) | same |
| Run a tool | `limactl shell agent-host sudo <tool>` | `multipass exec agent-host -- sudo <tool>` |

Agent containers: Remote-SSH to the VM, then Dev Containers "Attach to Running Container".

## Acceptance criteria

- The preflight passes: Linux, root, `/etc/agent-foundation` with `FOUNDATION_VERSION=1` and `UBUNTU=26.04`, no virtiofs/9p/sshfs/fuse.sshfs/fuse.reverse-sshfs mount, no `/etc/agent-host`, no user `agent-operator`, UID and GID 1001 free; otherwise exit 1 and nothing changed (a rerun is refused).
- No host mounts are listed in `/proc/mounts` after the run.
- `/etc/apt/apt.conf.d/51agent-host-no-docker-upgrade` exists (`root:root`, 644) and blacklists `docker.io`, `docker-buildx`, `containerd`, `runc` and `runsc` from unattended-upgrades.
- `agent-operator` cannot write the home of any login user (UID 1000-65533 except `agent-operator`): the home, `.bashrc`, `.profile`, `.ssh`, `.ssh/authorized_keys`, or anything `find -writable` finds.
- `/etc/agent-host` exists (`AGENT_HOST_VERSION=1`, `root:root`, 644) once the verification passed.
- Docker, Python, gnome-keyring, git and tar are available inside `agent-host` (command on `PATH`, checked as `agent-operator`); each of the 13 packages is installed (`dpkg -s`).
- `nft`, `iptables`, `ip` and `bridge` are available inside `agent-host` (checked as root, e.g. `nft --version`, `iptables --version`, `ip -V`, `bridge -V`).
- `runsc` is registered as a Docker runtime in `/etc/docker/daemon.json` (checked as root, e.g. `docker info --format '{{json .Runtimes}}'` has a `runsc` key; path `/usr/local/bin/runsc`, the eight runtimeArgs; `root:root`, not writable by `agent-operator`) and dockerd was restarted afterwards.
- `br_netfilter` is persisted and loaded: `/etc/modules-load.d/agent-host.conf` and `/etc/sysctl.d/90-agent-host-bridge.conf` exist with the expected content (`root:root`, 644), `/sys/module/br_netfilter` exists and `/proc/sys/net/bridge/bridge-nf-call-iptables` is `1`.
- gVisor works: a container runs with the `runsc` runtime (run as root, e.g. `docker run --rm --runtime=runsc hello-world`, pulled from the registry).
- `systemctl is-system-running` reports `running` (`degraded` is not accepted).
- The hostname is `agent-host` and the cloud-init drop-in `99-agent-host-hostname.cfg` has the expected content.
- `agent-operator` has UID and GID 1001 (shared with the image user in USS-003), no sudo (`sudo -n true` fails) and is in none of the groups `sudo`, `admin`, `wheel`, `adm`, `disk`, `lxd`, `libvirt`, `docker`.
- `/usr/local/bin/create_agent_container`, `/usr/local/bin/remove_agent_container`, `/usr/local/bin/create_agent_net`, `/usr/local/bin/remove_agent_net`, `/usr/local/bin/agent_lifecycle`, `/usr/local/bin/enter_agent` and `/usr/local/bin/refresh_endpoints` exist, are owned by `root:root` with mode 755, and their SHA-256 matches the builds read from the tools folder; `/usr/local/bin` is owned by root and not writable by `agent-operator`.
- `endpoints.json` is present and valid in the tool folder before anything is changed; otherwise the run stops with exit 1.
- `/etc/agent-net/endpoints.json` exists in `agent-host`, owned by `root:root` with mode 644, seeded via `refresh_endpoints` over stdin; a seed failure fails creation.
- `agent-restore.service` and `agent-guard.service` unit files exist (`root:root`, 644, expected content), `docker.service` and both units are enabled, `agent-guard.service` is active and `agent-restore.service` is active (exited).
- Docker/gVisor configuration exists only inside the VM; the reader's machine is untouched (the tool runs inside the VM and addresses no other machine).
- The whole setup is created by one run of the Go executable.

Withdrawn criteria (WSL2 path removed): `wsl -l -v` lists `agent-host` as version 2; `/mnt/c` absent; `powershell.exe` not runnable; `degraded` accepted for `user@0.service`; default user is `agent-operator`.
