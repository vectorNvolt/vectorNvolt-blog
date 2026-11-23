# agent-host (`create_agent_host`)

Go CLI that turns the foundation VM (Ubuntu 26.04, see `vm/README.md`) into `agent-host`. It is built natively for Linux and runs inside the VM as root (`sudo ./create_agent_host`, no flags). It checks the foundation marker, creates the non-root user `agent-operator` (UID/GID 1001), sets the hostname `agent-host`, installs Docker, gVisor, Python 3, gnome-keyring, git, tar, nftables, iptables and iproute2, registers the `runsc` runtime, installs the agent tools, lifecycle units and endpoints file, and verifies the acceptance criteria. Isolation of the agent host from your machine comes from the VM itself (own kernel, no host mounts, key-only SSH); the tool only customizes the VM from inside; it does no host-OS or mount handling. Specs: `design/01_user_story/uss-001_create_agent_host.md`, `design/local-vm-migration-plan.md`.

## Steps

Keep in sync with `internal/sequence/sequence.go` and `internal/steps/`. The step numbers match USC-001 main_flow. The first failing step stops the run.

| Step | File | Requirement | Exit code on failure | Purpose |
|---|---|---|---|---|
| 1 | `s01_preflight_foundation.go` | SFR-HOST-014 | 1 | Refuse unless GOOS is `linux`, euid is 0 and `/etc/agent-foundation` contains the line `FOUNDATION_VERSION=1`. The message points to `vm/check-foundation.sh`. Changes nothing. |
| 2 | `s02_tool_hashes.go` | SFR-HOST-013 | 1 | Read the seven tool builds (`create_agent_container`, `remove_agent_container`, `create_agent_net`, `remove_agent_net`, `agent_lifecycle`, `enter_agent`, `refresh_endpoints`) from the folder the executable runs from and compute their reference SHA-256. Also read `endpoints.json` from the same folder and validate it (`endpoints.go`): JSON `{"endpoints":[...]}`, at most 64 KiB, no unknown keys, a non-empty list of unique (case-insensitive) host names or IPv4 literals; a missing or invalid file stops the run here. Changes nothing. |
| 3 | `s03_create_user.go` | SFR-HOST-005 | 2 | Create group and user `agent-operator` with UID and GID 1001 (`groupadd`, `useradd --create-home`; fails if any other user or group holds 1001), no supplementary groups, no sudo. The foundation login user (`ubuntu` or the Lima host user) is never touched. |
| 4 | `s04_set_hostname.go` | SFR-HOST-015 | 2 | `hostnamectl set-hostname agent-host`; write `/etc/cloud/cloud.cfg.d/99-agent-host-hostname.cfg` (`preserve_hostname: true`, root:root 644) so cloud-init does not reset the name at the next boot; append `127.0.1.1 agent-host` to `/etc/hosts` unless the name is already there. |
| 5 | `s05_install_deps.go` | SFR-HOST-008 | 2 | Wait for systemd to finish starting (up to 120 s; a timeout never fails the step). Then run the embedded `scripts/install_deps.sh` as root: `apt-get install` of docker.io, docker-buildx, python3, gnome-keyring, git, tar, nftables, iptables, iproute2, ca-certificates, curl and gnupg; download the gVisor key and refuse it unless its fingerprint equals the pinned one; add the gVisor apt repository (for the VM's architecture), install `runsc`, `systemctl enable --now docker`. The script prints `progress: ` marker lines per phase; the step shows them as indented progress lines with elapsed time (via `ExecRunner.RunStream`), and `AGENT_LOG_LEVEL=DEBUG` also streams the apt output. |
| 6 | `s06_register_runtime.go` | SFR-HOST-012, SSR-HOST-006 | 2 | Make `/usr/local/bin` root:root 755, copy `runsc` there, run `runsc install` (writes `daemon.json` with the eight fixed `runtimeArgs`), make `daemon.json` root:root 644, persist and load `br_netfilter` with `bridge-nf-call-iptables`/`ip6tables` = 1 (`modules-load.d`, `sysctl.d`, `modprobe`, `sysctl -p`), restart docker. |
| 7 | `s07_install_tools.go`, `lifecycle_units.go`, `endpoints.go` | SFR-HOST-011, SSR-HOST-007 | 2 | Install the seven tools into `/usr/local/bin` as root:root 755 and compare the SHA-256 computed after the write with the step 2 reference. Then write `/etc/systemd/system/agent-restore.service` (oneshot, `agent_lifecycle restore`) and `agent-guard.service` (`agent_lifecycle guard`, restarts always) as root:root 644, rewriting a unit only if its content differs; `systemctl daemon-reload`, enable `docker.service`, `agent-restore.service` and `agent-guard.service`, start the last two. Then seed the endpoints file: run `/usr/local/bin/refresh_endpoints` with the bytes of `endpoints.json` on stdin (never a path; it validates, resolves the names and installs `/etc/agent-net/endpoints.json`) and check with `stat` that the file is `root:root 644`. This comes after the units because `refresh_endpoints` needs root, `CAP_NET_ADMIN`, the host network namespace and DNS. |
| 8 | `s08_verify.go` | SFR-HOST-009, SSR-HOST-001..007 | 2 | Run every acceptance check, collect all failures. Checks run as `agent-operator` (through `runuser`) unless marked root: `docker`, `python3`, `gnome-keyring-daemon`, `git`, `tar` on the PATH; the 13 packages installed (`dpkg -s`, root); docker lists `runsc`, buildx, `nft`, `iptables`, `ip`, `bridge`, CA bundle, `curl`, `gpg` and `runsc` work (root); `daemon.json` has the `runsc` path and the eight `runtimeArgs` in order, is root:root and not writable by `agent-operator`; every tool is root:root 755 and matches its reference SHA-256; `/usr/local/bin` is owned by root and not writable by `agent-operator`; both unit files (content, root:root 644), the three units enabled, `agent-guard` active, `agent-restore` active (exited); hostname and the cloud-init drop-in; the two `br_netfilter` files (content, root:root 644), `/sys/module/br_netfilter` present and `bridge-nf-call-iptables` reads `1`; `docker run --rm --runtime=runsc hello-world` (root); `systemctl is-system-running` prints exactly `running` (`degraded` fails); `agent-operator` has UID and GID 1001, no root-equivalent group (`sudo`, `admin`, `wheel`, `adm`, `disk`, `lxd`, `libvirt`, `docker`) and no sudoers entry; `agent-operator` cannot write under the home of any login user (UID 1000-65533 in `/etc/passwd`): the home, `.bashrc`, `.profile`, `.ssh`, `.ssh/authorized_keys` and `find -writable` (one check per user). |
| 9 | `s09_report.go` | SFR-HOST-010 | none (exit 0) | Print the success report. |

## Inputs in the executable's folder

`create_agent_host` runs from the folder (`dist`) that holds the seven tool builds and `endpoints.json` (the model endpoints, `{"endpoints":["api.anthropic.com","platform.claude.com"]}`). All eight files must be present before anything is changed (exit 1, `NOT_CREATED`). `endpoints.json` is sent to `refresh_endpoints` as bytes on stdin, the way `create_agent_image` sends `Dockerfile.agent`. `AGENT_HOST_TOOLS_DIR` overrides the folder.

## How to build / run

Inside the foundation VM, from the repo root (Go is in `/usr/local/go`; the build is native, so `amd64` or `arm64` follows the VM):

```
export PATH="$PATH:/usr/local/go/bin"
mkdir -p dist
(cd packages/agent-host && CGO_ENABLED=0 go build -buildvcs=false -o ../../dist/create_agent_host ./cmd/create_agent_host)
```

The seven tool builds and `endpoints.json` must be in `dist/` too (build lines: `packages/agent-sandbox`, `packages/agent-net`, and the howto). Then:

```
cd dist
sudo ./create_agent_host
```

Any argument prints usage and exits 1. `sudo` resets the environment, so pass variables through `env`: `sudo env AGENT_LOG_LEVEL=DEBUG ./create_agent_host`.

| Exit code | Meaning | Exit state |
|---|---|---|
| 0 | `agent-host` created and verified | none |
| 1 | Prerequisite not met (step 1 or 2, or invalid arguments) | `NOT_CREATED` |
| 2 | Runtime failure (steps 3-8) | `PARTIALLY_CONFIGURED` (steps 3-7), `UNVERIFIED` (step 8) |

The failure report names the step, message and exit state; step 8 also lists each failed check.

## Configuration

All via environment variables; an empty value falls back to the default.

| Variable | Default |
|---|---|
| `AGENT_LOG_LEVEL` | `INFO` (upper-cased; `DEBUG` also traces each command) |
| `AGENT_HOST_TOOLS_DIR` | folder of the running executable (holds the seven tool builds and `endpoints.json`) |
| `AGENT_HOST_GVISOR_KEY_FINGERPRINT` | pinned default (40 hex characters) |

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
| `internal/services/` (`host`, `render`) | `services/` |
| `internal/steps/` | `nodes/` |
| `internal/sequence/` | `graph.py` |
| `tests/` | `tests/` |
| `examples/` | `examples/` |

## Limitations

- No rollback on failure: the tool reports and stops. A failed run can leave a partially configured VM; the exit state says how far it got. A rerun is not a repair: step 1 refuses (exit 1, NOT_CREATED, nothing changed) once `/etc/agent-host`, `agent-operator` or UID/GID 1001 exists. Delete the VM and create it again (`limactl delete --force agent-host`, or `multipass delete agent-host && multipass purge`).
- `agent-operator` has no `docker` group membership and is not in sudo/admin. The container check (`docker run --rm --runtime=runsc hello-world`) in step 8 runs as root.
- The foundation login user (passwordless sudo, SSH key) stays and is the operator account; `agent-operator` is only the owner of the agent workspaces.
- Untested targets and the arm64 image gap: see "Verification status" in `howto/README.md`.
