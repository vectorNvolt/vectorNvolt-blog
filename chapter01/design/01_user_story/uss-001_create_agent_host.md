# USS-001 — Create the `agent-host` WSL distro

## Participants

| Participant | Role |
|---|---|
| Developer | Owns the Windows machine; works day-to-day in `dev-distro` |
| Provisioning tool | Go executable, run from host Windows (PowerShell 7.4+), that creates and configures the distro |
| WSL2 | Windows platform that hosts the distros |
| `dev-distro` | Developer's daily distro; builds all Go executables (Go toolchain and uv live here); source for data transfer into `agent-host` |
| `agent-host` | New distro, dedicated to agent operation |

## Goal

As a developer, I want to create a dedicated `agent-host` WSL2 distro by running a single Go executable, so that agents run in a separate environment from my `dev-distro` and from Windows, with all required dependencies already installed.

Isolation claim (precise):

- `agent-host` has no access to Windows drives (`/mnt/c`) and no Windows interop (cannot launch Windows binaries).
- `agent-host` has its own filesystem, separate from `dev-distro`.
- This is NOT a VM, kernel, or network boundary: all WSL2 distros share one VM. The hardening limits the distro's reach toward Windows; it does not block `wsl -d agent-host` or `\\wsl.localhost\agent-host` from Windows.

## Build in `dev-distro` (precondition)

All Go executables are built in `dev-distro`, where the Go toolchain lives (together with uv), cross-compiled with `GOOS`/`GOARCH`; nothing is built on host Windows. Go is needed in `dev-distro` only, not on Windows and not in `agent-host`. There are four Go modules and ten binaries, all written to one folder `dist/` at the repo root. In bash, from the repo root:

```bash
mkdir -p dist
(cd packages/agent-host && CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -buildvcs=false -o ../../dist/create_agent_host.exe ./cmd/create_agent_host)
(cd packages/agent-image && CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -buildvcs=false -o ../../dist/create_agent_image.exe ./cmd/create_agent_image)
(cd packages/agent-net && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o ../../dist/create_agent_net ./cmd/create_agent_net)
(cd packages/agent-net && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o ../../dist/remove_agent_net ./cmd/remove_agent_net)
(cd packages/agent-sandbox && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o ../../dist/create_agent_container ./cmd/create_agent_container)
(cd packages/agent-sandbox && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o ../../dist/remove_agent_container ./cmd/remove_agent_container)
(cd packages/agent-sandbox && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o ../../dist/agent_lifecycle ./cmd/agent_lifecycle)
(cd packages/agent-sandbox && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o ../../dist/enter_agent ./cmd/enter_agent)
(cd packages/agent-net && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o ../../dist/refresh_endpoints ./cmd/refresh_endpoints)
(cd packages/agent-net && CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -buildvcs=false -o ../../dist/refresh_endpoints.exe ./cmd/refresh_endpoints_win)
```

| Module | `cmd/` package | Binary | Target |
|---|---|---|---|
| `packages/agent-host` | `create_agent_host` | `create_agent_host.exe` | windows/amd64 |
| `packages/agent-image` | `create_agent_image` | `create_agent_image.exe` | windows/amd64 |
| `packages/agent-net` | `create_agent_net`, `remove_agent_net`, `refresh_endpoints` | same names | linux/amd64 |
| `packages/agent-net` | `refresh_endpoints_win` | `refresh_endpoints.exe` | windows/amd64 |
| `packages/agent-sandbox` | `create_agent_container`, `remove_agent_container`, `agent_lifecycle`, `enter_agent` | same names | linux/amd64 |

The built files are then copied from `dist/` to the Windows folder the Windows executables are run from (e.g. `C:\agent-setup`), together with `Dockerfile.agent` and `requirements.lock` (USS-003) and `endpoints.json`. `endpoints.json` is the model endpoint list, `{"endpoints":["api.anthropic.com","platform.claude.com"]}` by default; it is a hand-kept file, not a build output. `create_agent_image.exe` is the USS-003 image tool and runs on Windows; `create_agent_host` neither installs nor uses it. `refresh_endpoints.exe` is the Windows tool for later endpoint updates; `create_agent_host` neither installs nor uses it either.

`dist/` is untracked (repo `.gitignore`, `/dist/`); binaries are not committed.

## Interaction

1. Developer runs the Go executable (name: create_agent_host) from host Windows in PowerShell 7.4+. WSL 2.4.10 or later is required (Windows 11, or Windows 10 21H2+); the tool checks this first and stops if WSL is older. It takes no arguments.
2. Tool creates the `agent-host` distro with `wsl --import`: it downloads the official Ubuntu 26.04 LTS (`resolute`) WSL image (gzip-compressed tar, `https://releases.ubuntu.com/26.04.1/ubuntu-26.04.1-wsl-amd64.wsl`), accepts it only if it verifies against `https://releases.ubuntu.com/26.04.1/SHA256SUMS` (signed by `SHA256SUMS.gpg`), and imports it into an install directory on Windows (`%LOCALAPPDATA%\agent-host` by default) as WSL version 2. The downloaded image is a temporary file, removed when the run ends
3. Tool creates the non-root default user `agent-operator` (no sudo, not in the `docker` group) with an explicit pinned numeric UID, not the next free UID. The value is 1001, one shared value, identical to the UID of the non-root user in the agent image (USS-003). The 26.04 image ships no `ubuntu` user; the tool removes the `ubuntu` user and group only if present.
4. Tool writes `/etc/wsl.conf`, `/etc/hosts`, the isolate service and masks systemd-resolved (see below). The `wsl.conf` content is delivered on stdin to a shell command (`/bin/sh -c`: `cat` into a temporary file, `chown root:root`, `chmod 644`, `mv -f` over `/etc/wsl.conf`) inside the distro, not as a command argument.
5. Tool runs `wsl --terminate agent-host` to apply it. This only stops the distro; it restarts on the next command.
6. Tool waits for systemd to finish starting (up to 120 s; a timeout does not fail the step), writes the static `/etc/resolv.conf` (see below), removes the empty `/mnt/<drive>` leftovers and installs the dependencies (see below), including gVisor `runsc` from the gVisor apt repository, whose signing key must match a pinned fingerprint before the repository is trusted. It then registers `runsc` as a Docker runtime with `runsc install` (writes the runtime into `/etc/docker/daemon.json` inside `agent-host`) and restarts `dockerd` (`systemctl restart docker`). USS-001 is the only owner of this registration; USS-002, USS-003 and USS-004 never touch `daemon.json`.
7. Tool installs the seven Linux tools `create_agent_container` and `remove_agent_container` (USS-002, USS-006), `create_agent_net` and `remove_agent_net` (USS-004, USS-007), `agent_lifecycle` (restore and guard, see step 7a), `enter_agent` and `refresh_endpoints` (model endpoint list, see step 7b) from the Windows folder it is run from (copied there from `dist/`) into `/usr/local/bin/` (owner `root:root`, mode 755; they run as root, so only root may replace them) via `wsl.exe`, then compares the SHA-256 inside `agent-host` with the Windows file, for each binary:
   `Get-Content -LiteralPath $bin -AsByteStream -Raw | wsl.exe -d agent-host -u root -e /bin/sh -c 't="$1.tmp" && cat > "$t" && chown root:root "$t" && chmod "$2" "$t" && mv -f "$t" "$1"' sh /usr/local/bin/<name> 755` (`<name>` is `create_agent_container`, `remove_agent_container`, `create_agent_net`, `remove_agent_net`, `agent_lifecycle`, `enter_agent` or `refresh_endpoints`; `install /dev/stdin` is not used because the Ubuntu 26.04 rootfs ships uutils coreutils, whose `install` fails to read `/dev/stdin` under `wsl.exe -e`)
   7a. In the same step the tool writes two systemd units (`root:root`, mode 644): `/etc/systemd/system/agent-restore.service` (oneshot, `RemainAfterExit`, `Requires`/`After`/`PartOf` `docker.service`, runs `agent_lifecycle restore`, `ExecStopPost` removes `/run/agent-sandbox/ready`, `WantedBy` `docker.service` and `multi-user.target`) and `/etc/systemd/system/agent-guard.service` (simple, `Restart=always`, runs `agent_lifecycle guard`). It runs `systemctl daemon-reload`, enables `docker.service` and both units and starts them. After every dockerd start `agent-restore.service` removes the old `agent-*` containers, rebuilds the agents-net gate and writes the ready marker that `create_agent_container` waits for (USS-002, USS-004); `agent-guard.service` follows container start, unpause, die and update and network connect and disconnect: on `die` it disarms the name, on the other events it runs `check-gate` and removes the container on a clean failure; on an error it disarms first and removes only after a successful disarm; it also reconciles at start-up and every 60 s (check-gate of every agent container whose lock is free, eviction of untagged `released` elements, disarm of names whose container is gone).
   7b. After the tools and units are in place, the tool seeds `/etc/agent-net/endpoints.json` from the `endpoints.json` in the same Windows folder: it sends the file's bytes through `wsl.exe` stdin to `/usr/local/bin/refresh_endpoints` (`Get-Content -LiteralPath $endpoints -AsByteStream -Raw | wsl.exe -d agent-host -u root -e /usr/local/bin/refresh_endpoints`), never by path; the transport is the same one `create_agent_image.exe` uses for `Dockerfile.agent` (USS-003). It then checks the file is `root:root` mode 644. A seed failure fails creation. The file was already read and validated at the start (see below), before anything is created.
8. Tool verifies the result (see acceptance criteria) and reports success or failure.

`endpoints.json` must be in the same folder as the tools: JSON `{"endpoints":[...]}`, a non-empty list of unique host names (default `api.anthropic.com`, `platform.claude.com`). The tool reads and validates it together with the tool-build prerequisite check, before the distro is downloaded or created. If it is missing or invalid, creation fails there with exit 1 and nothing is created.

Later endpoint updates are not part of this story: edit `endpoints.json` and run `refresh_endpoints.exe` from Windows.

No rollback: on failure the tool reports and stops; the developer removes a half-built distro with `wsl --unregister agent-host`.

## Isolation — `wsl.conf` key attributes

| Section | Key | Value | Effect |
|---|---|---|---|
| `[automount]` | `enabled` | `false` | No Windows drives (`/mnt/c`) |
| `[interop]` | `enabled` | `false` | No launching of Windows binaries |
| `[interop]` | `appendWindowsPath` | `false` | Windows `PATH` not appended |
| `[automount]` | `mountFsTab` | `false` | `/etc/fstab` not processed |
| `[network]` | `hostname` | `agent-host` | Own hostname, not the Windows one |
| `[network]` | `generateHosts` | `false` | `/etc/hosts` is written by the tool (localhost plus `127.0.1.1 agent-host`) |
| `[network]` | `generateResolvConf` | `false` | `/etc/resolv.conf` is a static file written by the tool after the restart, at the start of dependency installation (`1.1.1.1`, `9.9.9.9`) |
| `[gpu]` | `enabled` | `false` | Requests no GPU passthrough; on the tested WSL 2.6.3 host `/dev/dxg` stays and the isolate service masks it |
| `[time]` | `useWindowsTimezone` | `false` | Windows timezone not copied |
| `[user]` | `default` | `agent-operator` | Default login user |
| `[boot]` | `systemd` | `true` | Required (docker, gnome-keyring/dbus) |

`/etc/resolv.conf` is a regular file (`root:root`, 644), replacing any symlink, because WSL's resolv.conf may point into `/mnt/wsl`, which is unmounted at boot, and because the systemd-resolved tmpfiles rule would otherwise turn it into a symlink at boot. The tool writes it after the restart, once systemd has started, because on Ubuntu 26.04 a file written before the first boot under the new `wsl.conf` was gone by then (what removed it was not identified), while a file written after that boot survives restarts. The tool also masks `systemd-resolved.service`, `systemd-resolved-monitor.socket`, `systemd-resolved-varlink.socket` and the tmpfiles rule `/etc/tmpfiles.d/systemd-resolve.conf`; systemd-resolved does not run on `agent-host`, and host tools (including apt) resolve directly through `/etc/resolv.conf`.

WSL injects some mounts and binfmt entries regardless of `wsl.conf`. The tool therefore installs `agent-host-isolate.service` (script `/usr/local/libexec/agent-host-isolate`, mode 755, plus the docker drop-in `/etc/systemd/system/docker.service.d/agent-host-isolate.conf` with `Requires=` and `After=`). At boot it lazily unmounts `/mnt/wslg`, `/tmp/.X11-unix`, `/mnt/wsl` and `/usr/lib/wsl` (which is why `/usr/lib/wsl/lib` and `/usr/lib/wsl/drivers` are empty) and unregisters the `WSLInterop` and `WSLInterop-late` binfmt entries. It also bind-mounts `/dev/null` over `/dev/dxg` in the mount namespace of `agent-host`. The node is a shared kernel device (`/dev` is devtmpfs, shared across distros in the VM) and stays present despite `[gpu] enabled=false`, so it is masked, not deleted, because deleting it would affect other distros. Root inside the distro could undo the bind mount. If it fails, docker does not start and no agent can run. It is written in step 4 and takes effect from the restart in step 5, so dependency installation already runs isolated.

> Isolation scope: Windows drive mounts and Windows interop are disabled and the filesystem is separate from `dev-distro`. All WSL2 distros still share one VM (kernel, network), `agent-host` is not a VM, kernel, or network boundary.

## Dependencies

Docker and gVisor are configured inside `agent-host` only; nothing changes on Windows or in `dev-distro`.

- Docker engine (`docker.io` and `docker-buildx`; service enabled and started)
- gvisor (`runsc`, registered as a Docker runtime via `runsc install`)
- Python 3
- gnome-keyring
- git (available on `agent-host` only; not in the agent container)
- tar
- nftables (the `nft` binary; used by `create_agent_net arm` (USS-004) for the per-port netdev tables, since the WSL2 kernel has no nftables bridge family but has netdev)
- iptables (host-side `DOCKER-USER` / `INPUT` rules set by `create_agent_net` (USS-004))
- iproute2 (the `ip` and `bridge` binaries; used by `create_agent_net` (USS-004) for the bridge port flags, the static FDB entry and the veth lookup)
- ca-certificates, curl and gnupg (installed by `install_deps.sh`; used to fetch the gVisor apt repository key and check it against the pinned fingerprint)

## Data transfer (placeholder)

`$tar` is a placeholder for the path of a tar archive on host Windows. The target directory must exist in `agent-host` and be owned by `agent-operator`. Extract into the parent directory (`-C`), so the archive's top-level folder is not nested twice.

host-windows -> agent-host: 
e.g.: Get-Content -LiteralPath $tar -AsByteStream -Raw | wsl.exe -d agent-host -u agent-operator -e tar -C /home/agent-operator -xf -

dev-distro -> agent-host
1. via git remote
2. running on powershell 7.4+ at host windows:
wsl.exe -d dev-distro -u myuser -e tar -C /home/<parent_folder> -cf - <folder> |
  wsl.exe -d agent-host -u agent-operator -e tar -C /home/agent-operator -xf -

agent-host has access to git repo and it can share the code base in this way

## Acceptance criteria

- `wsl -l -v` lists `agent-host` as version 2.
- Docker, Python, gnome-keyring, git and tar are available inside `agent-host` (command on `PATH`, checked as `agent-operator`).
- `nft`, `iptables`, `ip` and `bridge` are available inside `agent-host` (checked as root, e.g. `nft --version`, `iptables --version`, `ip -V`, `bridge -V`).
- `runsc` is registered as a Docker runtime in `/etc/docker/daemon.json` inside `agent-host` (checked as root, e.g. `docker info --format '{{json .Runtimes}}'` has a `runsc` key) and dockerd was restarted afterwards.
- gVisor works: a container runs with the `runsc` runtime (run as root, e.g. `docker run --rm --runtime=runsc hello-world`, pulled from the registry).
- Isolation settings from `wsl.conf` are active after restart:
  - `/mnt/c` is absent.
  - `powershell.exe` is not runnable in `agent-host`.
  - `/etc/wsl.conf` has the expected content; `/etc/resolv.conf` and `/etc/hosts` have the expected content and are `root:root`, and `resolv.conf` is not a symlink; the hostname is `agent-host`.
  - `/mnt/wslg`, `/tmp/.X11-unix`, `/mnt/wsl` and `/usr/lib/wsl` are not mountpoints; `/dev/dxg` is absent or masked by `/dev/null`, and `/usr/lib/wsl/lib` and `/usr/lib/wsl/drivers` are empty.
  - The isolation files (`/usr/local/libexec/agent-host-isolate`, `agent-host-isolate.service`, the docker drop-in) have the expected content, owner and mode, and `agent-host-isolate.service` is enabled and active (exited).
  - `systemctl is-system-running` reports ok (`running`); `degraded` is accepted only when `user@0.service` (which WSL fails to start) is the sole failed unit.
  - Default user is `agent-operator`, with no sudo (`sudo -n true` fails) and not in the `docker` group (e.g. `id -nG agent-operator` has no `docker`).
  - `agent-operator` has the pinned numeric UID (1001, shared with the image user in USS-003), e.g. `id -u agent-operator`.
- `/usr/local/bin/create_agent_container`, `/usr/local/bin/remove_agent_container`, `/usr/local/bin/create_agent_net`, `/usr/local/bin/remove_agent_net`, `/usr/local/bin/agent_lifecycle`, `/usr/local/bin/enter_agent` and `/usr/local/bin/refresh_endpoints` exist, are owned by `root:root` with mode 755, and their SHA-256 matches the Windows copies (built in `dev-distro` into `dist/`).
- `endpoints.json` is present and valid in the tool folder before anything is created; otherwise creation fails with exit 1 and nothing is created.
- `/etc/agent-net/endpoints.json` exists in `agent-host`, owned by `root:root` with mode 644, seeded from the Windows `endpoints.json` via `refresh_endpoints` over stdin; a seed failure fails creation.
- `agent-restore.service` and `agent-guard.service` unit files exist (`root:root`, 644, expected content), `docker.service` and both units are enabled, `agent-guard.service` is active and `agent-restore.service` is active (exited).
- Docker/gVisor configuration exists only inside `agent-host`; `dev-distro` is untouched (the tool addresses only `agent-host`) and Windows is changed only by the install directory.
- The whole setup is created by one run of the Go executable.
