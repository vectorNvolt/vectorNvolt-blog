# Runbook: bare Windows + WSL2 machine to a running agent container

Repo: https://github.com/vectorNvolt/vectorNvolt-blog. All paths are relative to `chapter01/` (layout: `chapters/01_foundation`, `design`, `howto`, `packages`, `.gitignore`).

Scope: Windows + WSL2 only. Other platforms are out of scope; adapt the steps yourself.

Conventions:

- PowerShell = Windows PowerShell 7.4+.
- Dev distro = your Ubuntu WSL distro (examples use the name `Ubuntu`), bash as root.
- agent-host = the isolated `agent-host` WSL distro that `create_agent_host.exe` creates. Every `wsl.exe -d agent-host` command runs from PowerShell.
- `C:\agent-setup` = Windows staging folder for the built binaries. Any path works; replace it everywhere.
- `<chapter01-wsl-path>` = absolute path of your `chapter01/` clone inside the dev distro (`pwd` there).
- `<agentID>` must match `^[a-z][a-z0-9-]{0,30}$`; the container is named `agent-<agentID>`.
- Items marked "(inferred, not in repo)" or "(verify)" are not confirmed by the repo.

Steps: 0 Prerequisites, 1 Check image inputs, 2 Build and copy binaries, 3 Create agent-host, 4 Verify tools, 5 Create workspace root, 6 Check agents-net, 7 Build image, 8 Create container, 9 Log in, 10 Update the model endpoints, 11 Teardown.

## 0. Install prerequisites

Where: dev distro, `chapter01/`.

Install Go (1.22 or newer; `agent-host/go.mod` pins toolchain go1.22.2) and `uv`. Install Go from https://go.dev/dl/ (inferred, not in repo).

Do not run Docker in the dev distro (all WSL2 distros share one VM network stack; only `agent-host` runs Docker). If it is installed, disable it:

```powershell
wsl.exe -d Ubuntu -u root -e systemctl disable --now docker docker.socket
```

The agent-host needs outbound udp/tcp port 53 to `1.1.1.1` and `9.9.9.9`: containers and endpoint resolution (`create_agent_net arm`, `check-gate`, `refresh_endpoints`) both use them, not the host's `/etc/resolv.conf`. The host's own DNS (including apt during install) uses them too: `create_agent_host.exe` writes a static `/etc/resolv.conf` with these two resolvers and masks systemd-resolved, which does not run on agent-host. Networks that block public DNS are not supported as is.

Install `uv` (inferred, not in repo), then open a new shell or run `source /root/.local/bin/env`:

```bash
curl -LsSf https://astral.sh/uv/install.sh | sh
```

Check (dev distro):

```bash
go version
uv --version
```

Check (PowerShell, any directory):

```powershell
$PSVersionTable.PSVersion          # 7.4 or newer
wsl -l -v                          # Ubuntu listed, VERSION 2
wsl --version                      # WSL 2.4.10 or newer (create_agent_host.exe enforces this; fix with: wsl --update)
```

## 1. Check the image inputs

Where: dev distro, `chapter01/packages/agent-image`.

```bash
grep -c -- '--hash=' requirements.lock    # non-zero
grep '^FROM' Dockerfile.agent             # shows @sha256
```

Only if you change dependencies: edit `requirements.in`, delete `requirements.lock` (uv keeps the pins of an existing output file), then regenerate:

```bash
uv pip compile requirements.in --generate-hashes --python-version 3.14 --python-platform x86_64-manylinux_2_39 -o requirements.lock
```

Only if you change `CLAUDE_CODE_VERSION`: update the two sha512 values in `Dockerfile.agent` (see `packages/agent-image/README.md`, "Refreshing the pins").

## 2. Build the binaries and copy them to Windows

`dist/` is gitignored, so you build it yourself. The Dockerfile and lock come from `packages/agent-image/`; `endpoints.json` comes from `packages/agent-net/`.

Where: dev distro, `chapter01/`.

```bash
mkdir -p dist
(cd packages/agent-host && CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -buildvcs=false -o ../../dist/create_agent_host.exe ./cmd/create_agent_host)
(cd packages/agent-image && GOOS=windows GOARCH=amd64 go build -buildvcs=false -o ../../dist/create_agent_image.exe ./cmd/create_agent_image)
(cd packages/agent-sandbox && GOOS=linux GOARCH=amd64 go build -o ../../dist/create_agent_container ./cmd/create_agent_container)
(cd packages/agent-net && GOOS=linux GOARCH=amd64 go build -o ../../dist/create_agent_net ./cmd/create_agent_net)
(cd packages/agent-sandbox && GOOS=linux GOARCH=amd64 go build -o ../../dist/remove_agent_container ./cmd/remove_agent_container)
(cd packages/agent-net && GOOS=linux GOARCH=amd64 go build -o ../../dist/remove_agent_net ./cmd/remove_agent_net)
(cd packages/agent-sandbox && GOOS=linux GOARCH=amd64 go build -o ../../dist/agent_lifecycle ./cmd/agent_lifecycle)
(cd packages/agent-sandbox && GOOS=linux GOARCH=amd64 go build -o ../../dist/enter_agent ./cmd/enter_agent)
(cd packages/agent-net && GOOS=linux GOARCH=amd64 go build -o ../../dist/refresh_endpoints ./cmd/refresh_endpoints)
(cd packages/agent-net && GOOS=windows GOARCH=amd64 go build -buildvcs=false -o ../../dist/refresh_endpoints.exe ./cmd/refresh_endpoints_win)
cp packages/agent-image/Dockerfile.agent packages/agent-image/requirements.lock dist/
cp packages/agent-net/endpoints.json dist/
```

The first build downloads Go modules (`agent-host` depends on `github.com/ProtonMail/go-crypto`), so it needs network access.

Default `endpoints.json`: `{"endpoints": ["api.anthropic.com", "platform.claude.com"]}`. If you need other names, edit it in `dist/` (or in `C:\agent-setup` after the copy) before step 3.

Check: `ls -l dist` lists thirteen files: three `.exe` files, seven Linux binaries, `Dockerfile.agent`, `requirements.lock` and `endpoints.json`.

Where: PowerShell, any directory. Copy all of `dist/` (the seven Linux builds must sit next to `create_agent_host.exe`).

```powershell
New-Item -ItemType Directory -Force C:\agent-setup | Out-Null
wsl.exe -d Ubuntu -u root -e tar -C <chapter01-wsl-path>/dist -cf - . | tar.exe -xf - -C C:\agent-setup
```

Check: hashes match, and `Get-ChildItem C:\agent-setup` lists thirteen files.

```powershell
wsl.exe -d Ubuntu -u root -e sh -c 'sha256sum <chapter01-wsl-path>/dist/*'
Get-FileHash C:\agent-setup\* | Format-Table Hash, Path
```

## 3. Create the agent-host distro

Where: PowerShell, `C:\agent-setup`.

```powershell
cd C:\agent-setup
.\create_agent_host.exe
```

`create_agent_host.exe` needs `endpoints.json` in its folder (missing or invalid: it fails at step 3 before creating anything) and seeds `/etc/agent-net/endpoints.json` at the end of step 11, sending the file bytes through `wsl.exe` stdin. It downloads the Ubuntu 26.04 WSL image and verifies its signature, so it needs network access.

Keep-alive is on by default: the exe writes a Startup script that holds a hidden `wsl.exe` session open at every Windows logon, and starts one now, so WSL does not idle-stop `agent-host` (see sections 9 and 11). Skip it with `.\create_agent_host.exe --no-keepalive`.

The exe refuses if `agent-host` already exists; there is no upgrade path yet. To update (new tool builds, new unit files), run `wsl --unregister agent-host` and create it again. This loses everything inside the distro: images, workspaces, Claude login and history. Rebuild the image (section 7) afterwards. An agent-host created before `refresh_endpoints` existed lacks it and must be recreated the same way to get it.

If Windows refuses to start the exe with "An Application Control policy has blocked this file", nothing was created. The binaries are unsigned, and Windows judges each build separately, so this can also hit `create_agent_image.exe` (section 7) and `refresh_endpoints.exe` (section 10). Check the Smart App Control state (0 off, 1 on, 2 evaluation):

```powershell
(Get-ItemProperty "HKLM:\SYSTEM\CurrentControlSet\Control\CI\Policy").VerifiedAndReputablePolicyState
```

If it is `1`, rebuild the blocked exe with a new build ID and copy it again: add `-ldflags=-buildid=<any-new-value>` to that exe's section 2 command. Rerunning the section 2 command unchanged does not help, because Go builds are reproducible: the same source, path and flags recreate the identical blocked file. A new build ID only produces a different file, and that file can be blocked too. On the author's host, one build of the complete code stayed blocked while builds of the same code with other build IDs ran, so the block was not caused by what the code does; why Windows refused that one file is unknown. `-buildvcs=false` is a build setting, not a fix for this block. The lasting fixes are to turn off Smart App Control (Windows Security > App & browser control > Smart App Control settings; this lowers protection for the whole machine; Microsoft's FAQ says recent Windows updates let you turn it back on without a clean install, but you cannot return to evaluation mode without resetting or reinstalling Windows) or to sign the exes with a publicly trusted certificate. A self-signed certificate does not satisfy Smart App Control. If it is not `1`, another App Control policy is blocking the exe; on a managed machine, ask your admin.

Undo: `wsl --unregister agent-host` (and delete the Startup file, section 11)

Check:

```powershell
wsl -l -v                                   # agent-host, VERSION 2
Test-Path "$env:APPDATA\Microsoft\Windows\Start Menu\Programs\Startup\agent-host-keepalive.cmd"   # True (not with --no-keepalive)
wsl.exe -d agent-host -u root -e pgrep -a -x sleep   # "sleep infinity" = keep-alive session running
wsl.exe -d agent-host -u root -e docker info --format '{{json .Runtimes}}'   # contains runsc
wsl.exe -d agent-host -u root -e docker run --rm --runtime=runsc hello-world
wsl.exe -d agent-host -u root -e stat -c '%U:%G %a' /etc/agent-net/endpoints.json   # root:root 644
```

## 4. Verify the Linux tools in agent-host

Where: PowerShell, any directory. Nothing to install; `create_agent_host.exe` did it.

```powershell
wsl.exe -d agent-host -u root -e ls -l /usr/local/bin/create_agent_net /usr/local/bin/create_agent_container /usr/local/bin/remove_agent_container /usr/local/bin/remove_agent_net /usr/local/bin/agent_lifecycle /usr/local/bin/enter_agent /usr/local/bin/refresh_endpoints
Get-FileHash C:\agent-setup\create_agent_net, C:\agent-setup\create_agent_container, C:\agent-setup\remove_agent_container, C:\agent-setup\remove_agent_net, C:\agent-setup\agent_lifecycle, C:\agent-setup\enter_agent, C:\agent-setup\refresh_endpoints | Format-Table Hash, Path
wsl.exe -d agent-host -u root -e sha256sum /usr/local/bin/create_agent_net /usr/local/bin/create_agent_container /usr/local/bin/remove_agent_container /usr/local/bin/remove_agent_net /usr/local/bin/agent_lifecycle /usr/local/bin/enter_agent /usr/local/bin/refresh_endpoints
```

Check: hashes equal (`Get-FileHash` uppercase, `sha256sum` lowercase).

Also check the lifecycle units that `create_agent_host.exe` installed. Together they rebuild the network gate every time dockerd starts and remove agent containers started outside the tools:

```powershell
wsl.exe -d agent-host -u root -e systemctl status agent-restore.service agent-guard.service   # restore: active (exited), guard: active (running)
wsl.exe -d agent-host -u root -e cat /run/agent-sandbox/ready                                  # dockerd MainPID
```

## 5. Create the workspace root

Where: PowerShell, any directory.

```powershell
# (inferred, not in repo)
wsl.exe -d agent-host -u root -e install -d -o agent-operator -g agent-operator /home/agent-operator/Workspace-Agents
```

Check:

```powershell
wsl.exe -d agent-host -u root -e id agent-operator    # uid=1001(agent-operator) gid=1001(agent-operator) ...
wsl.exe -d agent-host -u root -e ls -ld /home/agent-operator/Workspace-Agents
```

## 6. Check the agents-net network

Where: PowerShell, any directory. Nothing to run: `agent-restore.service` (started by `create_agent_host.exe`, and again at every dockerd start) runs `remove_agent_net` then `create_agent_net` and writes `/run/agent-sandbox/ready`.

Check:

```powershell
wsl.exe -d agent-host -u root -e systemctl status agent-restore.service   # active (exited)
wsl.exe -d agent-host -u root -e cat /run/agent-sandbox/ready
wsl.exe -d agent-host -u root -e docker network inspect agents-net
```

Troubleshooting fallback (the service failed, or the network predates the gate): rebuild by hand, then restart the service so the ready marker is written.

```powershell
wsl.exe -d agent-host -u root -e sh -c 'remove_agent_net; create_agent_net'
wsl.exe -d agent-host -u root -e systemctl restart agent-restore.service
```

If `create_agent_net` fails with a leftover-bridge/overlap error: `wsl --shutdown` (stops all distros), then check the service again. See `journalctl -u agent-restore.service` inside agent-host.

## 7. Build the agent image

Where: PowerShell, `C:\agent-setup` (the exe reads `Dockerfile.agent` and `requirements.lock` from the current directory).

```powershell
cd C:\agent-setup
.\create_agent_image.exe --tag latest
```

Check:

```powershell
wsl.exe -d agent-host -u root -e docker image ls agent-image
```

## 8. Create the agent container

Where: PowerShell, any directory.

```powershell
wsl.exe -d agent-host -u root -e create_agent_container <agentID>
```

Check:

```powershell
wsl.exe -d agent-host -u root -e docker inspect -f '{{.HostConfig.Runtime}} {{json .Mounts}}' agent-<agentID>   # runsc; /workspace rw, /home/agent/.claude rw, /etc/resolv.conf ro
wsl.exe -d agent-host -u root -e docker ps
wsl.exe -d agent-host -u root -e create_agent_net check-gate agent-<agentID>   # exit 0 = every control ok
wsl.exe -d agent-host -u root -e docker exec agent-<agentID> cat /etc/resolv.conf   # 1.1.1.1 and 9.9.9.9, no 127.0.0.11
```

`create_agent_container` waits up to 60 s for the ready marker written by `agent-restore.service`; if it is missing it exits 3 (`NET_NOT_READY`): see `systemctl status agent-restore.service`.

Normal path: use `enter_agent` (section 9), which creates the container when needed and attaches. Standalone `create_agent_container` is for checking the create steps; the container survives because of the keep-alive (section 9). After `wsl --shutdown` or a reboot, `agent-restore.service` has already removed the dead containers and rebuilt the gate. Never `docker start` an agent container: `agent-guard.service` removes it.

## 9. Log in and run

Status: `create_agent_container` was observed passing on the author's host; `enter_agent` and the keep-alive hidden start are not yet verified on a real host.

Where: PowerShell, any directory.

```powershell
wsl.exe -d agent-host -u root -e enter_agent <agentID>
wsl.exe -d agent-host -u root -e enter_agent <agentID> -- bash
wsl.exe -d agent-host -u root -e enter_agent <agentID> -- claude
```

`enter_agent` does, by state of `agent-<agentID>`:

| State of `agent-<agentID>` | Action before attaching |
|---|---|
| running | `check-gate` must pass; otherwise it exits with the gate's code and hints `remove_agent_container <agentID>`. A running container is never removed. |
| absent | create (as `create_agent_container`) |
| any other (exited, paused/contained after a failed create, created, dead) | remove (as `remove_agent_container`, no purge; workspace and Claude state kept), then create |

Then it runs `docker exec -it -u 1001 agent-<agentID> <cmd>`. Default `<cmd>` is `claude --continue` if a previous conversation exists (`/var/lib/agent-sandbox/<agentID>/claude/projects/*/*.jsonl`), else `claude`; text after `--` replaces it. Failures exit `NOT_ENTERED` (1 usage or not root, 2 internal) or with the create/remove/check-gate exit code, before attaching. Create waits for the ready marker (up to 60 s, exit 3 `NET_NOT_READY`), so a launch right after dockerd started is fine.

Re-enter any time with the same command, also after exiting `claude`. `docker exec -it -u 1001 agent-<agentID> ...` still works directly on a running container.

Claude login and history live in `/var/lib/agent-sandbox/<agentID>/claude` on the host (mounted at `/home/agent/.claude`) and survive remove and recreate, so log in is not needed again. The running claude process is lost when the container dies.

Keep-alive: WSL stops the `agent-host` distro once no `wsl.exe` session is attached, which kills dockerd and every agent container (exit 255). The keep-alive from section 3 is a hidden `wsl.exe -d agent-host -e sleep infinity` session started at each Windows logon (and once by `create_agent_host.exe`), so the distro stays up and containers survive exiting `claude`. Containers still die on Windows reboot or logoff, or `wsl --shutdown`; then `enter_agent` recreates the container (nothing revives it, `--restart no`). Without the keep-alive (`--no-keepalive`), the container lives only while a session is attached.

Never `docker start` an agent container: `agent-guard.service` removes it. To drop the login and history of one agent: `remove_agent_container <agentID> --purge` (section 11).

## 10. Update the model endpoints (not yet verified on a real host)

Where: PowerShell, `C:\agent-setup`.

The model API names live in `endpoints.json` next to the exes (default `{"endpoints": ["api.anthropic.com", "platform.claude.com"]}`). That Windows file is the source. `create_agent_host.exe` seeds it into the distro as `/etc/agent-net/endpoints.json` (root, 0644), and `refresh_endpoints.exe` updates it. Never edit `/etc/agent-net/endpoints.json` inside the distro. If it is invalid or writable by group/world, arming (`create_agent_container`, `enter_agent`) and `create_agent_net check-gate` fail.

Names are resolved on the host when a container is armed (A records only; an IPv6-only name fails). The addresses stay pinned in iptables rules until the container is re-armed or a refresh runs. After a CDN rotation, egress to the model API drops and `check-gate` fails until refreshed.

Edit `endpoints.json` in `C:\agent-setup` to change the names, or leave it as is after an address rotation. Then:

```powershell
cd C:\agent-setup
.\refresh_endpoints.exe --dry-run   # prints planned changes, writes nothing
.\refresh_endpoints.exe
```

`refresh_endpoints.exe` reads `endpoints.json` from the current directory and validates it locally (bad file: exit 1, nothing sent). It then runs `wsl.exe -d agent-host -u root -e /usr/local/bin/refresh_endpoints [--dry-run]` with the file bytes on stdin (no path is passed into the distro) and returns that exit code. The Linux tool validates, resolves all names, installs `/etc/agent-net/endpoints.json` atomically (skipped if the bytes are identical), then updates the rules of all armed containers.

Per armed container, new accept rules are inserted before stale ones are deleted (not one atomic transaction). Any lookup failure exits 2 and changes nothing. Exit 1 = usage, invalid input or privilege, 2 = runtime.

Check:

```powershell
wsl.exe -d agent-host -u root -e create_agent_net check-gate agent-<agentID>   # exit 0
wsl.exe -d agent-host -u root -e cat /etc/agent-net/endpoints.json   # matches the Windows file
wsl.exe -d agent-host -u root -e sh -c 'iptables -S DOCKER-USER | grep agent-net:'   # shows the new addresses
```

## 11. Tear down

Where: PowerShell, any directory.

```powershell
# one container: disarm, remove, verify; workspace and Claude state kept
wsl.exe -d agent-host -u root -e remove_agent_container <agentID>

# one container and its workspace folder
wsl.exe -d agent-host -u root -e remove_agent_container <agentID> --purge-workspace

# one container and its Claude login and history (/var/lib/agent-sandbox/<agentID>); combinable with --purge-workspace
wsl.exe -d agent-host -u root -e remove_agent_container <agentID> --purge

# whole agent network: all agent containers (workspaces kept), then leftover port tables, leftover rules and agents-net
wsl.exe -d agent-host -u root -e remove_agent_net
# bring agents-net, the gate and the ready marker back (otherwise only the next dockerd start does)
wsl.exe -d agent-host -u root -e systemctl restart agent-restore.service

# keep-alive only: delete the Startup file, then end the running session (the distro idle-stops again)
Remove-Item "$env:APPDATA\Microsoft\Windows\Start Menu\Programs\Startup\agent-host-keepalive.cmd"
wsl --terminate agent-host

# whole host (removes everything, including images and networks); also delete the Startup file as above
wsl --unregister agent-host
Remove-Item "$env:APPDATA\Microsoft\Windows\Start Menu\Programs\Startup\agent-host-keepalive.cmd"
wsl --shutdown    # clears the VM's leftover Docker bridges (stops all distros)
```

## Known limitations (WSL2)

- Idle stop: mitigated by the keep-alive (section 3). Only a Windows reboot or logoff, or `wsl --shutdown`, kills the containers now (also any time with `--no-keepalive` and no attached session). Restore rebuilds the gate and `enter_agent` recreates the container; login and history are kept, the running claude process is lost.
- Per-port controls stay bound to the veth (the WSL kernel has no nft bridge family), so a restarted container is replaced, never repaired.
- Model endpoint addresses are pinned in iptables rules until `refresh_endpoints` runs or the container is re-armed (section 10); there is no automatic refresh.
- `agent-host` is not a VM, kernel or network boundary: all WSL2 distros share one VM, and Windows can still reach its filesystem via `\\wsl$`.
- Not yet verified on a real host: the keep-alive hidden start (`conhost.exe --headless`) and its survival after `create_agent_host.exe` exits; `enter_agent`; the unit checks of `create_agent_host.exe` running as `agent-operator`.

## Not covered

- No git worktree, git proxy or credential step in this chapter.
