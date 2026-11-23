# vm/ - agent-host foundation VM

One standard Ubuntu 26.04 VM, created from aligned config files by different
tools. The Go binaries later run inside it as root.

| File | Purpose |
|---|---|
| `foundation.sh` | Single source of truth. Idempotent, root only: base packages, pinned Go (sha256 per arch), sshd hardening drop-in, unattended-upgrades, marker file. |
| `agent-host.lima.yaml` | Lima config (macOS / Linux). Generated. |
| `agent-host.cloud-init.yaml` | cloud-init config (Multipass on Windows, cloud providers). Generated. Replace the placeholder SSH public key first. |
| `agent-host.*.yaml.in` | Templates; the `@@FOUNDATION@@` line is replaced by `foundation.sh`. |
| `generate.sh` | Regenerates the two yaml files. |
| `check-foundation.sh` | Asserts the contract; prints `PASS\|FAIL <n> <description>`. Run before `create_agent_host`; item 7 (no Docker) fails afterwards by design. |

## Create the VM

```
limactl start --name=agent-host vm/agent-host.lima.yaml
multipass launch 26.04 --name agent-host --cpus 4 --memory 8G --disk 60G --cloud-init vm/agent-host.cloud-init.yaml
```

## Regenerate / drift check

```
vm/generate.sh          # rewrite the two yaml files
vm/generate.sh --check  # exit 1 if a committed file is stale
```

## Verify a VM

```
limactl shell agent-host bash -s < vm/check-foundation.sh
multipass exec agent-host -- bash -s < vm/check-foundation.sh
```

## Contract (FOUNDATION_VERSION=1)

1. Ubuntu 26.04 LTS, stock cloud image and kernel (amd64 or arm64).
2. systemd running; `cloud-init status --wait` reports `done`.
3. Login user with passwordless `sudo`; SSH key login only.
4. `git`, `ca-certificates`, `curl`, `build-essential` installed.
5. Pinned Go toolchain in `/usr/local/go`, sha256-verified per arch.
6. Marker `/etc/agent-foundation` containing `FOUNDATION_VERSION=1`.
7. Nothing else: no Docker, no containerd, no host mounts.

The Go version is also pinned in `check-foundation.sh`; keep both in sync.
