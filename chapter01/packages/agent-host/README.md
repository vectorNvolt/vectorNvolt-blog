# agent-host (`create_agent_host`)

Go CLI that creates the isolated `agent-host` WSL2 distro from Windows (PowerShell 7.4+) in one run (only flag: `--no-keepalive`): it downloads and verifies the Ubuntu 26.04 LTS (`resolute`) WSL image, imports it as WSL 2 (requires WSL 2.4.10 or later), creates the non-root user `agent-operator`, writes `/etc/wsl.conf` and the isolation files (no `/mnt/c`, no Windows interop, no GPU, no WSLg, own hostname and DNS, systemd on), installs Docker, gVisor, Python 3, gnome-keyring, git, tar, nftables, iptables and iproute2 inside the distro, and verifies the acceptance criteria. Isolation scope: Windows drive mounts and interop are disabled and the filesystem is separate from `dev-distro`, but all WSL2 distros share one VM, so `agent-host` is not a VM, kernel or network boundary (Windows can still reach the filesystem via `\\wsl$`). The hardening limits the distro's reach toward Windows; it does not block `wsl -d agent-host` or `\\wsl.localhost\agent-host` from Windows. Spec: `design/01_user_story/uss-001_create_agent_host.md`.

## Steps

Keep in sync with `internal/steps/sNN_*.go`. Step 1 is the developer starting the executable (`cmd/create_agent_host`, SFR-HOST-001). The first failing step stops the run.

| Step | File | Requirement | Exit code on failure | Purpose |
|---|---|---|---|---|
| 2 | `s02_confirm_no_distro.go` | SFR-HOST-002 | 1 | Fail if WSL is unavailable or a distro named `agent-host` already exists (left untouched). Then check `wsl.exe --version` is 2.4.10 or newer; otherwise stop with exit status 1 and "WSL 2.4.10 or newer is required; run `wsl --update`". |
| 3 | `s03_tool_hashes.go` | SFR-HOST-013 | 1 | Read the seven Linux tool builds (`create_agent_container`, `remove_agent_container`, `create_agent_net`, `remove_agent_net`, `agent_lifecycle`, `enter_agent`, `refresh_endpoints`) from the folder the exe runs from and compute their reference SHA-256 on the host. Also read `endpoints.json` from the same folder and validate it (`endpoints.go`): JSON `{"endpoints":[...]}`, at most 64 KiB, no unknown keys, a non-empty list of unique (case-insensitive) host names or IPv4 literals; a missing or invalid file stops the run here. Changes nothing. |
| 4 | `s04_obtain_rootfs.go` | SFR-HOST-003 | 1 | Download the Ubuntu 26.04 WSL image, `SHA256SUMS` and `SHA256SUMS.gpg`. Verify `SHA256SUMS.gpg` as a detached signature over `SHA256SUMS` first; only then accept the image if its SHA-256 matches `SHA256SUMS`. |
| 5 | `s05_register_distro.go` | SFR-HOST-004 | 2 | Create the install directory and `wsl --import` the image as WSL 2. |
| 6 | `s06_create_user.go` | SFR-HOST-005 | 2 | Remove the `ubuntu` user and group only if present (the Ubuntu 26.04 WSL image ships no `ubuntu` user; earlier images did, with GID 1001, which would collide with `agent-operator` 1001:1001), then create `agent-operator` as root with UID and GID 1001 (fails if any other user or group holds it), with no sudo/admin group. |
| 7 | `s07_write_wsl_conf.go`, `isolation_units.go` | SFR-HOST-006 | 2 | Write `/etc/wsl.conf` via stdin: `[boot] systemd=true`; `[automount] enabled=false, mountFsTab=false`; `[interop] enabled=false, appendWindowsPath=false`; `[network] hostname=agent-host, generateHosts=false, generateResolvConf=false`; `[gpu] enabled=false` (does not remove `/dev/dxg`, see the isolate service below); `[time] useWindowsTimezone=false`; `[user] default=agent-operator`. Write `/etc/hosts` (root:root 644; localhost plus `127.0.1.1 agent-host`) so the Windows hostname from first boot does not remain. Mask `systemd-resolved.service`, `systemd-resolved-monitor.socket`, `systemd-resolved-varlink.socket` (in `/etc/systemd/system/`) and the tmpfiles rule `/etc/tmpfiles.d/systemd-resolve.conf` with `ln -sfn /dev/null`, so the packaged rule cannot turn `/etc/resolv.conf` into a symlink at the next start. Install the embedded `scripts/agent_host_isolate.sh` as `/usr/local/libexec/agent-host-isolate` (755), `agent-host-isolate.service`, and the docker drop-in `/etc/systemd/system/docker.service.d/agent-host-isolate.conf` (`Requires=` and `After=` the service). At boot the service lazily unmounts `/mnt/wslg`, `/tmp/.X11-unix`, `/mnt/wsl` and `/usr/lib/wsl` (WSL injects them regardless of `wsl.conf`) and unregisters the `WSLInterop` and `WSLInterop-late` binfmt entries. It also bind-mounts `/dev/null` over `/dev/dxg` in the mount namespace of `agent-host`: the node is a shared kernel device (`/dev` is devtmpfs, shared across distros in the VM), so it is masked, not deleted, because deleting it would affect other distros. Root inside the distro could undo the bind mount. If it fails, docker does not start and no agent can run. It takes effect from the step 8 restart, so step 9 already runs isolated. |
| 8 | `s08_terminate.go` | SFR-HOST-007 | 2 | `wsl --terminate` so `wsl.conf` applies on the next start. |
| 9 | `s09_install_deps.go` | SFR-HOST-008 | 2 | Wait for systemd to finish starting (up to 120 s; a timeout never fails the step). Then write `/etc/resolv.conf` as a static regular file (root:root 644; `nameserver 1.1.1.1`, `nameserver 9.9.9.9`), replacing any symlink. It is written here, after the first boot under the new `wsl.conf`, because on Ubuntu 26.04 a file written before that boot was gone by this step (what removed it was not identified), while a file written after it survives restarts; step 12 verifies it. Host tools, including apt, resolve directly through this file; `systemd-resolved` does not run on `agent-host`. Then remove empty leftover `/mnt/<drive>` dirs, and run embedded `scripts/install_deps.sh` as root inside the distro (gVisor apt key checked against the pinned fingerprint). |
| 10 | `s10_register_runtime.go` | SFR-HOST-012, SSR-HOST-006 | 2 | Make `/usr/local/bin` root:root 755, copy `runsc` there, run `runsc install` (writes `daemon.json`), restart docker. |
| 11 | `s11_install_tools.go`, `lifecycle_units.go` | SFR-HOST-011, SSR-HOST-007 | 2 | Install the seven tools (including `agent_lifecycle`, `enter_agent` and `refresh_endpoints`) into `/usr/local/bin` as root:root 755 and compare the SHA-256 computed in the distro with the step 3 reference. Then write `/etc/systemd/system/agent-restore.service` (oneshot, `agent_lifecycle restore`) and `agent-guard.service` (`agent_lifecycle guard`, restarts always) as root:root 644, rewriting a unit only if its content differs; `systemctl daemon-reload`, enable `docker.service`, `agent-restore.service` and `agent-guard.service`, and start the last two. Then seed the endpoints file (`endpoints.go`): run `wsl.exe -d agent-host -u root -e /usr/local/bin/refresh_endpoints` with the bytes of `endpoints.json` on stdin (never a path; it validates, resolves the names on the host and installs `/etc/agent-net/endpoints.json`), then check with `stat -c '%U:%G %a'` that the file is `root:root 644`; a non-zero exit fails the step with the tool's stderr. This comes after the units because `refresh_endpoints` needs root, `CAP_NET_ADMIN`, the host network namespace and DNS, and the restore service has run. Finally the keep-alive (`keepalive.go`, skipped with `--no-keepalive`): write the Startup script and start the keep-alive process (see "Keep-alive"). |
| 12 | `s12_verify.go` | SFR-HOST-009, SSR-HOST-001..007 | 2 | Run every acceptance check, collect all failures. Includes `/etc/wsl.conf` content; `/etc/resolv.conf` and `/etc/hosts` content and ownership, and that `resolv.conf` is not a symlink; the hostname; that `/mnt/wslg`, `/tmp/.X11-unix`, `/mnt/wsl` and `/usr/lib/wsl` are not mountpoints; `/dev/dxg` absent or masked by `/dev/null`, and `/usr/lib/wsl/lib` and `/usr/lib/wsl/drivers` empty; the isolation files' content, owner and mode; `agent-host-isolate.service` enabled and active (exited); both unit files (content, root:root 644), the three units enabled, `agent-guard` active and `agent-restore` active (exited), and (unless `--no-keepalive`) the keep-alive Startup script content. |
| 13 | `s13_report.go` | SFR-HOST-010 | none (exit 0) | Print the success report. |

Rootfs verification (step 4): the `SHA256SUMS.gpg` URL is the `SHA256SUMS` URL plus `.gpg`. The signature must be valid for the pinned Ubuntu CD Image Automatic Signing Key (2012), fingerprint `8439 38DF 228D 22F7 B374 2BC0 D94A A3F0 EFE2 1092`. The public key is embedded in the tool at `internal/services/ubuntu-cdimage-signing-key.asc`; nothing is fetched to trust it. The image's SHA-256 is checked against `SHA256SUMS` only after the signature verifies.

## Inputs in the exe's folder

`create_agent_host.exe` runs from the folder (`dist`) that holds the seven Linux tool builds and `endpoints.json` (the model endpoints, `{"endpoints":["api.anthropic.com","platform.claude.com"]}`). All eight files must be present before anything is created (exit 1, `NOT_CREATED`). `endpoints.json` is sent to `refresh_endpoints` as bytes on stdin, the way `create_agent_image` sends `Dockerfile.agent`; no `/mnt/c`, `wslpath`, `\\wsl$` or staging directory is used.

## How to build / run

```
GOOS=windows GOARCH=amd64 go build -buildvcs=false -o create_agent_host.exe ./cmd/create_agent_host
```

```
.\create_agent_host.exe
```

```
.\create_agent_host.exe --no-keepalive
```

`--no-keepalive` is the only flag; any other argument prints usage and exits 1. Unless the flag is given, `%APPDATA%` must be set (checked in step 2, exit 1).

## Keep-alive

WSL stops `agent-host` when no `wsl.exe` session is attached, which kills `dockerd` and all agent containers. Step 11 therefore:

- writes `%APPDATA%\Microsoft\Windows\Start Menu\Programs\Startup\agent-host-keepalive.cmd` (CRLF; rewritten only if missing or different). Windows runs it at logon; it runs `start "" /b conhost.exe --headless wsl.exe -d agent-host -e sleep infinity`, a hidden `wsl.exe` that holds one session open;
- starts the same `conhost.exe --headless wsl.exe -d agent-host -e sleep infinity` now, detached (`DETACHED_PROCESS | CREATE_NEW_PROCESS_GROUP`, not waited for), so it outlives `create_agent_host.exe`. Running it again while one is active only adds another harmless session.

Step 12 checks the script exists with the expected content (not the process). `--no-keepalive` skips the write, the start and that check.

To disable: delete the script, then run `wsl --terminate agent-host` (this also ends the running keep-alive session; the distro then idle-stops again).

| Exit code | Meaning | Exit state |
|---|---|---|
| 0 | `agent-host` created and verified | none |
| 1 | Prerequisite not met (steps 2-4, or invalid arguments) | `NOT_CREATED` |
| 2 | Runtime failure (steps 5-12) | `NOT_CREATED` (step 5), `PARTIALLY_CONFIGURED` (steps 6-11), `UNVERIFIED` (step 12) |

The failure report names the step, message and exit state; step 12 also lists each failed check.

## Configuration

All via environment variables; empty, non-numeric or non-positive numeric values fall back to the default.

| Variable | Default |
|---|---|
| `AGENT_LOG_LEVEL` | `INFO` (upper-cased; `DEBUG` also traces each `wsl.exe` call) |
| `AGENT_HOST_ROOTFS_URL` | `https://releases.ubuntu.com/26.04.1/ubuntu-26.04.1-wsl-amd64.wsl` |
| `AGENT_HOST_ROOTFS_SHA256SUMS_URL` | `https://releases.ubuntu.com/26.04.1/SHA256SUMS` (same directory as `AGENT_HOST_ROOTFS_URL`; its signature is fetched from this URL + `.gpg` and verified against the embedded Ubuntu signing key before the SHA-256 is used) |
| `AGENT_HOST_TOOLS_DIR` | folder of the running executable (holds the seven tool builds and `endpoints.json`) |
| `AGENT_HOST_GVISOR_KEY_FINGERPRINT` | pinned default (40 hex characters) |
| `AGENT_HOST_INSTALL_DIR` | `%LOCALAPPDATA%\agent-host`, else `<user cache dir>\agent-host`, else `<temp dir>\agent-host` |
| `AGENT_HOST_DOWNLOAD_TIMEOUT_SECONDS` | `300` |
| `AGENT_HOST_DOWNLOAD_ATTEMPTS` | `3` |
| `AGENT_HOST_DOWNLOAD_BACKOFF_MS` | `2000` |

## Tests

```
go test ./...
```

## Layout

Mapped to `packages/package_template`.

| Here | package_template |
|---|---|
| `cmd/create_agent_host/` | `main.py` |
| `internal/config/` | `config.py` |
| `internal/services/` (`wsl`, `rootfs`, `render`) | `services/` |
| `internal/steps/` | `nodes/` |
| `internal/sequence/` | `graph.py` |
| `tests/` | `tests/` |
| `examples/` | `examples/` |

## Open points

- No rollback on failure: report and stop; user removes a half-built distro with `wsl --unregister agent-host`. A failed run can leave a partially configured `agent-host`; the exit state says how far it got. Remove it before retrying, since step 2 refuses an existing distro.
- `agent-operator` has no `docker` group membership and is not in sudo/admin. The container check (`docker run --rm --runtime=runsc hello-world`) in s12 runs as root.
- SSR-HOST-005 ("host/dev-distro unchanged") is enforced structurally (the wsl adapter can only target `agent-host`), not by a before/after diff.
