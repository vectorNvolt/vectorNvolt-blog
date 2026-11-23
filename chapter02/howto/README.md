# Runbook: boost converter design demo on an agent-host VM (Multipass on Windows, Lima on macOS/Linux)

This user manual explains how to prepare a VM that runs agents (`agent-host`), how to communicate with that VM from your host, and how to run a demo in which Claude designs a DC-DC boost converter. 
The main steps in this manual: 
1. create Ubuntu 26.04 VM (Multipass with Hyper-V on Windows, Lima on macOS and Linux), 
2. turn into `agent-host` with `create_agent_host`, 
3. create an agent container, 
4. demo with Claude designing the converter with the ngspice tool and `agent_graph` (process `boost-design`).

## Scenarios

| Host | VM tool | Config file | Arch | Prerequisites | Part 0 section |
|---|---|---|---|---|---|
| Windows (Pro or Enterprise, Hyper-V) | Multipass (HCS on Hyper-V) | `vm/agent-host.cloud-init.yaml` | amd64 | Hyper-V enabled, Multipass (HCS driver), PowerShell 7.4+, Git, OpenSSH client | "Multipass (Windows)" |
| Windows Home | not covered | | amd64 | Hint: Home has no Hyper-V; use Multipass with the VirtualBox driver (`multipass set local.driver=virtualbox`) or a cloud VM | not covered |
| macOS, Apple Silicon | Lima (`vz`) | `vm/agent-host.lima.yaml` | arm64 | Lima 1.0.0+ (`brew install lima`), bash or zsh, Git, OpenSSH client | "Lima (macOS, Linux)" |
| macOS, Intel | Lima | `vm/agent-host.lima.yaml` | amd64 | Lima 1.0.0+ (`brew install lima`), bash or zsh, Git, OpenSSH client | "Lima (macOS, Linux)" |
| Linux, amd64 | Lima (QEMU with KVM) | `vm/agent-host.lima.yaml` | amd64 | Lima 1.0.0+ release, QEMU, `/dev/kvm`, bash or zsh, Git, OpenSSH client | "Lima (macOS, Linux)" |
| Linux, arm64 | Lima | `vm/agent-host.lima.yaml` | arm64 | Lima 1.0.0+ release, QEMU, `/dev/kvm`, bash or zsh, Git, OpenSSH client | "Lima (macOS, Linux)" |
| Cloud VM | the provider | `vm/agent-host.cloud-init.yaml` | amd64, arm64 | An instance created with the cloud-init file, OpenSSH client | none: start at Part A over SSH |

All scenarios:

- VM size: 4 CPUs, 8 GiB RAM, 60 GiB disk; outbound network access (including port 53 to 1.1.1.1 and 9.9.9.9).
- Host commands run only in Part 0; everything else runs in a shell inside the VM (login user `ubuntu` on Multipass, your host username on Lima; passwordless `sudo`).
- Placeholders `<agentID>`, `<repo-url>`, `<repo-dir>`, `<vm-ip>`, `<ssh-port>`, `<runID>`, `<execId>`: replace in every command ([Appendix A](APPENDIX.md#appendix-a-placeholders)). Per-step checks: [APPENDIX.md](APPENDIX.md#appendix-b-checks). Tests: [TESTS.md](TESTS.md).

Sections: 
- Part 0: create VM; 
- Part A: 1 Prerequisites, 2 Build, 3 Check the build output, 4 Create agent-host, 5 Build the agent image, 6 Create the agent container, 7 Check the tools, 8 Workspace folders
- Part B: 9 ngspice tool image and profile, 10 Optional smoke test, 11 Run the boost-design process, 12 Optional agent-view, 13 Results and independent verification; 
- Exit codes; 
- Troubleshooting; 
- Optional: VS Code;
- Teardown.


## Part 0. Create the VM and connect

Goal: a running Ubuntu 26.04 VM named `agent-host` that you can reach from the host by shell, SSH, file copy and port forward.

SSH key: the host talks to the VM only with a dedicated key, `~/.ssh/agent-host_ed25519` (comment `agent-host`), used for nothing else. Set a passphrase. Every SSH command, `scp`, `rsync` and VS Code ([Optional: VS Code](#optional-vs-code)) use the alias `agent-host`, which pins this key (`IdentitiesOnly yes`).

Exposure: local the VM's SSH port is reachable only from your own host. 
- Lima forwards it to `127.0.0.1` on the host. 
- Multipass on Hyper-V reaches it over the Default Switch, a NAT network internal to the Windows host that the LAN cannot reach. 
- A cloud VM has a public address: do not open port 22 to the internet; reach the VM over a VPN (for example WireGuard or Tailscale) and restrict port 22 in the provider firewall to the VPN.

Follow one section: Multipass (Windows) or Lima (macOS, Linux).

### Multipass (Windows)

Goal: create the VM with Multipass on Hyper-V and connect to it from PowerShell.

Install Multipass (once). In an admin PowerShell, turn on Hyper-V, then reboot:

```powershell
dism.exe /Online /Enable-Feature /FeatureName:Microsoft-Hyper-V-All /All
```

Install Multipass, choosing Hyper-V when the installer asks for a hypervisor (or use the installer from https://canonical.com/multipass/install):

```powershell
winget install --id Canonical.Multipass -e
```

In a new PowerShell window (so the updated PATH is loaded):

```powershell
multipass version
```

Where to run: PowerShell 7.4+ on Windows. Select the HCS driver (the `hyperv` driver is deprecated) and get the repo:

```powershell
multipass set local.driver=hcs
git clone <repo-url> <repo-dir>  # get agent-harness code to the host
cd <repo-dir>
```

Create the dedicated key, then write a local copy of the cloud-init file with its public key:

```powershell
ssh-keygen -t ed25519 -f $env:USERPROFILE\.ssh\agent-host_ed25519 -C agent-host
$key = (Get-Content $env:USERPROFILE\.ssh\agent-host_ed25519.pub -Raw).Trim()
$yaml = (Get-Content -Raw vm\agent-host.cloud-init.yaml).Replace("`r`n","`n").Replace('ssh-ed25519 AAAA...REPLACE_WITH_YOUR_PUBLIC_KEY', $key)
$mirror = 'http://archive.ubuntu.com/ubuntu'   # if downloads are slow: a country mirror, e.g. http://hu.archive.ubuntu.com/ubuntu
$yaml = $yaml.Replace('http://archive.ubuntu.com/ubuntu', $mirror).Replace('http://security.ubuntu.com/ubuntu', $mirror)
[IO.File]::WriteAllText((Join-Path $PWD 'agent-host.local.yaml'), $yaml)
Select-String -Path agent-host.local.yaml -Pattern 'REPLACE_WITH'     # prints nothing
```

Create the VM and wait for cloud-init to finish:

```powershell
multipass launch 26.04 --name agent-host --cpus 4 --memory 8G --disk 60G --timeout 1800 --cloud-init agent-host.local.yaml
multipass exec agent-host -- cloud-init status --wait
```

Check, before step 4: `multipass list` shows `agent-host` `Running`; the contract check prints `PASS 1` to `PASS 7` (item 7 reports `FAIL` after `create_agent_host` installs Docker; expected).

```powershell
multipass list
(Get-Content -Raw vm\check-foundation.sh) -replace "`r","" | multipass exec agent-host -- bash -s
```

Shell in the VM (the "VM" of the rest of the runbook):

```powershell
multipass shell agent-host
```

Plain SSH: get the VM address, then add an alias to `%USERPROFILE%\.ssh\config` (update `HostName` after a restart if the address changes):

```powershell
multipass info agent-host      # IPv4 line: <vm-ip>
```

```
Host agent-host
    HostName <vm-ip>
    User ubuntu
    IdentityFile ~/.ssh/agent-host_ed25519
    IdentitiesOnly yes
    HostKeyAlias agent-host
```

```powershell
ssh agent-host
```

Files in and out:

```powershell
multipass transfer .\file.txt agent-host:/tmp/file.txt  # host -> VM, one file
multipass transfer agent-host:/tmp/result.txt .         # VM -> host, into the current folder
scp .\file.txt agent-host:/tmp/                         # host -> VM over SSH (dedicated key)
```

Port access (for example 8787 for agent-view in step 12): open a tunnel and keep the window open:

```powershell
ssh -L 8787:127.0.0.1:8787 agent-host
```

Run a tool without a shell: `multipass exec agent-host -- sudo <tool>`.

Stop, start, delete (delete loses images, workspaces and the Claude login): `multipass stop agent-host`, `multipass start agent-host`, `multipass delete agent-host; multipass purge`.

### Lima (macOS, Linux)

Goal: create the VM with Lima and connect to it from the host terminal.

Where: host terminal (bash or zsh). Get the repo:

```bash
git clone <repo-url> <repo-dir>
cd <repo-dir>
```

Create and start the VM (downloads the pinned Ubuntu 26.04 image, runs `foundation.sh`, waits up to 15 minutes; Lima creates its own SSH key, used only by `limactl shell` and `limactl copy`):

```bash
limactl start --name=agent-host vm/agent-host.lima.yaml
```

Check, before step 4: `limactl list` shows `agent-host` `Running`; the contract check prints `PASS 1` to `PASS 7` (item 7 reports `FAIL` after `create_agent_host` installs Docker; expected).

```bash
limactl list
limactl shell agent-host bash -s < vm/check-foundation.sh
```

Shell in the VM (the "VM" of the rest of the runbook):

```bash
limactl shell agent-host
```

Plain SSH: create the dedicated key (once; enter a passphrase), add its public key to the VM, and print the forwarded SSH port on `127.0.0.1` (`<ssh-port>`):

```bash
ssh-keygen -t ed25519 -f ~/.ssh/agent-host_ed25519 -C agent-host
limactl shell agent-host sh -c 'mkdir -p ~/.ssh && chmod 700 ~/.ssh && cat >> ~/.ssh/authorized_keys && chmod 600 ~/.ssh/authorized_keys' < ~/.ssh/agent-host_ed25519.pub
limactl list agent-host --format '{{.SSHLocalPort}}'
```

Add an alias to `~/.ssh/config` (update `Port` after a restart if it changes), then connect:

```
Host agent-host
    HostName 127.0.0.1
    Port <ssh-port>
    User <your host username>
    IdentityFile ~/.ssh/agent-host_ed25519
    IdentitiesOnly yes
    HostKeyAlias agent-host
```

```bash
ssh agent-host
```

Files in and out:

```bash
limactl copy ./file.txt agent-host:/tmp/file.txt     # host -> VM, one file
limactl copy agent-host:/tmp/result.txt .            # VM -> host, into the current folder
scp ./file.txt agent-host:/tmp/                      # host -> VM over SSH (dedicated key)
rsync -av ./folder/ agent-host:/tmp/folder/          # host -> VM, whole folder; reruns copy only changes
```

Ports: Lima forwards listening VM ports (for example 8787 for agent-view) to `localhost` automatically.

Run a tool without a shell: `limactl shell agent-host sudo <tool>`.

Stop, start, delete (delete loses images, workspaces and the Claude login): `limactl stop agent-host`, `limactl start agent-host`, `limactl delete --force agent-host`.

## Part A. Prepare

Goal: turn the VM into `agent-host` with a running agent container, a Claude login and the demo workspace.

Every command runs in the VM shell.

### 1. Prerequisites

Goal: get the repo into the VM and confirm the foundation is complete.

Clone the repo and check the foundation (all 7 items must print `PASS`; run before step 4):

```bash
git clone <repo-url> <repo-dir>    # get agent-harness code to the agent-host VM
cd <repo-dir>/<actual-folder>
cat /etc/agent-foundation
/usr/local/go/bin/go version
bash vm/check-foundation.sh
```

Check: [B1 in APPENDIX.md](APPENDIX.md#b1-prerequisites)

### 2. Build everything in one pass

Goal: build every binary and collect every file the later steps install into `dist/`.

Where: VM, repo root. Native build into `dist/` (gitignored); the first build downloads Go modules.

```bash
export PATH="$PATH:/usr/local/go/bin"
mkdir -p dist/bin
(cd packages/agent-host && CGO_ENABLED=0 go build -buildvcs=false -o ../../dist/create_agent_host ./cmd/create_agent_host)
(cd packages/agent-image && CGO_ENABLED=0 go build -o ../../dist/create_agent_image ./cmd/create_agent_image)
(cd packages/agent-sandbox && go build -o ../../dist/create_agent_container ./cmd/create_agent_container)
(cd packages/agent-net && go build -o ../../dist/create_agent_net ./cmd/create_agent_net)
(cd packages/agent-sandbox && go build -o ../../dist/remove_agent_container ./cmd/remove_agent_container)
(cd packages/agent-net && go build -o ../../dist/remove_agent_net ./cmd/remove_agent_net)
(cd packages/agent-sandbox && go build -o ../../dist/agent_lifecycle ./cmd/agent_lifecycle)
(cd packages/agent-sandbox && go build -o ../../dist/enter_agent ./cmd/enter_agent)
(cd packages/agent-net && go build -o ../../dist/refresh_endpoints ./cmd/refresh_endpoints)
(cd packages/agent-exec && CGO_ENABLED=0 go build -o ../../dist/bin/run_ephemeral_container ./cmd/run_ephemeral_container)
(cd packages/agent-exec && CGO_ENABLED=0 go build -o ../../dist/bin/ephemeral-authz ./cmd/ephemeral-authz)
(cd packages/agent-exec && CGO_ENABLED=0 go build -o ../../dist/bin/agent-init ./cmd/agent-init)
cp packages/agent-image/Dockerfile.base packages/agent-image/Dockerfile.agent packages/agent-image/requirements.lock dist/
cp packages/agent-net/endpoints.json dist/        # skip if you already edited dist/endpoints.json
rm -rf dist/agent_graph
cp -r packages/agent-graph/src/agent_graph dist/agent_graph
find dist/agent_graph -name __pycache__ -prune -exec rm -rf {} +
```

Run `export PATH="$PATH:/usr/local/go/bin"` again in every new shell.

Check: [B2 in APPENDIX.md](APPENDIX.md#b2-build-everything-in-one-pass)

### 3. Check the build output

Goal: confirm that `dist/` holds every file before anything is installed.

Where: VM, repo root. Expect `ok` for every file and no `MISSING`.

```bash
cd dist
for f in create_agent_host create_agent_image create_agent_container remove_agent_container create_agent_net remove_agent_net agent_lifecycle enter_agent refresh_endpoints endpoints.json Dockerfile.base Dockerfile.agent requirements.lock bin/agent-init bin/ephemeral-authz bin/run_ephemeral_container agent_graph/__init__.py agent_graph/__main__.py; do test -s "$f" && echo "ok $f" || echo "MISSING $f"; done
cd ..
```

Check: [B3 in APPENDIX.md](APPENDIX.md#b3-check-the-build-output)

### 4. Create agent-host

Goal: turn the foundation VM into `agent-host` (Docker, sandboxed runtime, agent tools) and create the workspace root.

Where: VM, `dist/`. Needs network access. Run the foundation check first (it fails on item 7 afterwards).

```bash
cd dist
sudo ./create_agent_host
```

- No flags. It prints `[1/9]` to `[9/9]`; the first failing step stops the run and the report names the step and exit state (`NOT_CREATED`, `PARTIALLY_CONFIGURED`, `UNVERIFIED`). Each command: `sudo env AGENT_LOG_LEVEL=DEBUG ./create_agent_host`.
- Step 5 (install dependencies) takes several minutes; it prints indented progress lines per phase; `AGENT_LOG_LEVEL=DEBUG` also shows the apt output.
- No rollback and no repair on rerun: once step 3 has run (or `/etc/agent-host` exists), a rerun stops at step 1 with `NOT_CREATED` and changes nothing. After a failure, delete the VM (Multipass: `multipass delete agent-host; multipass purge`; Lima: `limactl delete --force agent-host`) and recreate it with the Part 0 commands.

Workspace root:

```bash
sudo install -d -o agent-operator -g agent-operator /home/agent-operator/Workspace-Agents
```

Check: [B4 in APPENDIX.md](APPENDIX.md#b4-create-agent-host)

### 5. Build the agent image

Goal: build the `agent-image:latest` image that every agent container runs.

Where: VM, `dist/`. Builds `agent-image:latest` in the VM's Docker engine.

```bash
cd dist
sudo ./create_agent_image --tag latest      # on a rerun, answer y at "overwrite agent-image:latest? [y/N]"
```

- A first run prints `building base image ... (needs snapshot.ubuntu.com)`; a later run prints `base image agent-base:<hash> present`.
- Exit 1 naming `bin/` or `Dockerfile.base`: redo steps 2 and 3.

Check: [B5 in APPENDIX.md](APPENDIX.md#b5-build-the-agent-image)

### 6. Create the agent container

Goal: start the agent container for `<agentID>` and log it in to Claude.

Where: VM.

```bash
sudo create_agent_container <agentID>
```

After rebuilding the image, recreate the container (workspace and Claude login are kept):

```bash
sudo remove_agent_container <agentID>
sudo create_agent_container <agentID>
```

Never `docker start` an agent container: `agent-guard.service` removes it.

Log in to Claude, then exit `claude` (the login is kept in `/var/lib/agent-sandbox/<agentID>/claude`):

```bash
sudo enter_agent <agentID> -- claude
```

Check: [B6 in APPENDIX.md](APPENDIX.md#b6-create-the-agent-container)

### 7. Check the tools in the container

Goal: confirm that `run_ephemeral_container`, `agent_graph` and the authorization plugin work inside the container.

Where: VM.

```bash
sudo enter_agent <agentID> -- /usr/local/bin/run_ephemeral_container list   # only the header line, no profiles yet
sudo enter_agent <agentID> -- agent_graph --help                            # usage with "agent_graph run [--resume RUNID]"
sudo docker logs agent-<agentID>                                            # "agent-init:" lines, no plugin restart loop
sudo docker exec agent-<agentID> sh -c 'ls -l /run/docker/plugins/ephemeral-authz.sock; pgrep -a ephemeral-authz'   # socket root 0600; ephemeral-authz runs as authz
```

### 8. Workspace folders

Goal: fill the agent workspace with the demo project and a clean netlist folder.

Where: VM, repo root. Copies the demo workspace (dotdirs included) to the agent workspace, owned by uid/gid 1001. For your own project, replace the `-C` folder of the first `tar`.

```bash
sudo install -d -o 1001 -g 1001 /home/agent-operator/Workspace-Agents/<agentID>
sudo tar -C test/TC1_boost_converter/workspace --owner=1001 --group=1001 --numeric-owner -cf - . | sudo tar -C /home/agent-operator/Workspace-Agents/<agentID> -xf -
```

Remove leftovers of an earlier demo run and create the netlist folder:

```bash
sudo sh -c 'W=/home/agent-operator/Workspace-Agents/<agentID>; rm -rf $W/.output/exec/bst* $W/.input/boost_design'
sudo enter_agent <agentID> -- mkdir -p /workspace/.input/boost_design
```

Container paths:

```
src/                      source code; mounted ro in every execution
build/                    build output (build kind)
.venv/ .npm-env/ .conda-env/   dependency env dirs
.packages/                operator-supplied wheels (install only)
.tools/<name>/profile.yml      one folder per profile; never mounted
.tools/<name>/image.tar        docker save of the profile's image
.output/exec/<execId>/         one folder per execution, created by the tool
.process/  .input/  identity/  .agent/<runID>/  .output/<runID>/     agent-graph (Part B)
```

Check: [B8 in APPENDIX.md](APPENDIX.md#b8-workspace-folders)

## Part B. Demo: Claude designs a boost converter

Goal: let Claude design and verify a 24 V boost converter inside the agent container, then check the result yourself.

Process `boost-design`: 9 V to 15 V in, 24 V out, 0.2 A to 2 A. Claude sizes L and C, simulates six corners (Vin 9, 12, 15 V at 0.2 A and 2 A) with `run_ephemeral_container tool ngspice`, and writes `REPORT.md`. Budgets: 16 steps, 40 Claude calls, 90 minutes.

### 9. ngspice tool image and profile

Goal: give the agent the ngspice simulator as a pinned tool image it may run.

Where: VM, repo root. Needs step 8 and network access. Builds the tool image, saves it as `image.tar`, pins its ID in `profile.yml`:

```bash
sudo docker build -t exec-ngspice:1 - < test/TC1_boost_converter/workspace/.tools/ngspice/Dockerfile
sudo sh -c 'D=/home/agent-operator/Workspace-Agents/<agentID>/.tools/ngspice; mkdir -p $D && docker save -o $D/image.tar exec-ngspice:1 && chown -R 1001:1001 $D'
sudo sh -c 'D=/home/agent-operator/Workspace-Agents/<agentID>/.tools/ngspice; ID=$(docker image inspect -f "{{.Id}}" exec-ngspice:1) && sed -i "s|^image_id: .*|image_id: $ID|" $D/profile.yml && chown -R 1001:1001 $D && grep ^image_id $D/profile.yml'
```

The last command prints `image_id: sha256:<64 hex>`. After any Dockerfile change, rerun all three.

Check: [B9 in APPENDIX.md](APPENDIX.md#b9-ngspice-image-and-profile)

### 10. Optional smoke test

Goal: confirm that one ngspice simulation runs end to end before Claude uses the tool.

Where: VM. Runs the filled reference netlist (Vin 12 V, 2 A) with exec id `smoke1`:

```bash
sudo enter_agent <agentID> -- sh -c 'run_ephemeral_container tool ngspice --exec-id smoke1 --in netlist=/workspace/src/ngspice/boost_design/reference_12V_2A.cir; echo "exit=$?"'
sudo sh -c 'W=/home/agent-operator/Workspace-Agents/<agentID>; cat $W/.output/exec/smoke1/result.json; ls $W/.output/exec/smoke1/out'
sudo enter_agent <agentID> -- sh -c 'grep -E "^(vout_avg|vout_avg_prev|vout_pp|il_avg|il_pp)" /workspace/.output/exec/smoke1/out/sim.log'
sudo enter_agent <agentID> -- python3 -I /workspace/src/ngspice/readraw.py /workspace/.output/exec/smoke1/out/sim.raw --at 0.0009,0.00095,0.001
```

An exec id works once (`exec dir for --exec-id <execId> already exists`, exit 125). To rerun, use `smoke2` or delete the folder:

```bash
sudo rm -rf /home/agent-operator/Workspace-Agents/<agentID>/.output/exec/smoke1
```

Optional SVG plot of `v(out)`, copied to `/tmp` for download:

```bash
sudo enter_agent <agentID> -- python3 -I /workspace/src/ngspice/readraw.py /workspace/.output/exec/smoke1/out/sim.raw --svg /workspace/.output/smoke1.svg --vars 'v(out)'
sudo cp /home/agent-operator/Workspace-Agents/<agentID>/.output/smoke1.svg /tmp/smoke1.svg && sudo chmod 644 /tmp/smoke1.svg
```

On the host:

```powershell
# Multipass (PowerShell)
multipass transfer agent-host:/tmp/smoke1.svg .
```

```bash
# Lima (bash or zsh)
limactl copy agent-host:/tmp/smoke1.svg .
```

Check: [B10 in APPENDIX.md](APPENDIX.md#b10-smoke-test)

### 11. Run the boost-design process

Goal: run the `boost-design` process with Claude until it reports `COMPLETED`.

Where: VM, interactive terminal (`multipass shell`, `limactl shell` or `ssh`). Needs steps 8 and 9 and the Claude login. Takes several minutes.

Start check: no `CLAUDE.md`, `CLAUDE.local.md` or `.claude/` in `/workspace`; `/home/agent/.claude/CLAUDE.md` empty or absent; `/home/agent/.claude/skills/` empty.

```bash
sudo enter_agent <agentID> -- sh -c 'ls -la /home/agent/.claude; ls -A /home/agent/.claude/skills; find /workspace -maxdepth 2 \( -name CLAUDE.md -o -name CLAUDE.local.md -o -name .claude \)'
```

Expect no `skills` content (`ls: cannot access '/home/agent/.claude/skills': No such file or directory` is fine), `CLAUDE.md` absent or empty, and no `find` output.

Run:

```bash
sudo enter_agent <agentID> -- agent_graph run
```

Optional model and effort (defaults `opus`, `medium`; pinned in `run.json`):

```bash
sudo enter_agent <agentID> -- env AGENT_GRAPH_MODEL=sonnet AGENT_GRAPH_EFFORT=low agent_graph run
```

Dialog:

| State | What you do |
|---|---|
| S1 start | Nothing |
| S2 clarify | Answer `Which process do you want to run?` (for example `Design the 24 V boost converter from the hardware requirements and verify it with ngspice`), answer questions until it proposes `boost-design`, confirm `Run process boost-design (<title>)? (y/n)` with `y`. Note the `<runID>` it prints (`Run boost-design-<yyyymmdd-hhmmss> created ...`) |
| S3, S4 | Nothing unless a finding opens S7 |
| S5 | Read the outline, confirm `Start the run? (y/n)` with `y` |
| S6 | Steps `calc`, `netlist`, `simulate`, `evaluate`, `report` run; you see `STEP n: DONE`. Answer briefly if a step asks something |
| S7 resolution | Any error ends here. Claude proposes `edit_file`, `override` or `choose_process`; answer `y` to apply. `/resolved` re-checks; `/quit` (confirm `y`) leaves the run |
| S8 report | Prints `exit_state: COMPLETED`, exit 0 |

Interrupt: Ctrl-C, SIGTERM, a closed terminal or `/quit` suspends the run (exit 130). Resume (list run IDs with the `ls`):

```bash
sudo ls /home/agent-operator/Workspace-Agents/<agentID>/.agent
sudo enter_agent <agentID> -- agent_graph run --resume <runID>
```

Check: [B11 in APPENDIX.md](APPENDIX.md#b11-run-the-boost-design-process)

### 12. Optional: watch the run live with agent-view

Goal: follow the run's ngspice executions in a browser on the host.

Where: VM for build and run (second VM shell for the viewer), host browser. Read-only viewer of `.output/exec/<execId>/`; start it before or during step 11, or after it. Install Node.js and npm:

```bash
sudo apt-get install -y nodejs npm
node --version; npm --version
```

Build (the UI bundle must be built before `go build`):

```bash
export PATH="$PATH:/usr/local/go/bin"
(cd packages/agent-view/web && npm ci && npm run build)
(cd packages/agent-view && CGO_ENABLED=0 go build -o ../../dist/agent_view ./cmd/agent_view)
sha256sum dist/agent_view
```

Install for `agent-operator` and compare the hash with the one above:

```bash
sudo -u agent-operator mkdir -p /home/agent-operator/.local/bin
sudo install -o agent-operator -g agent-operator -m 755 dist/agent_view /home/agent-operator/.local/bin/agent_view
sudo sha256sum /home/agent-operator/.local/bin/agent_view
```

Start it in a second VM shell and keep it open:

```bash
sudo -u agent-operator /home/agent-operator/.local/bin/agent_view --ws /home/agent-operator/Workspace-Agents/<agentID> --listen 127.0.0.1:8787
```

Expect `agent_view: serving /home/agent-operator/Workspace-Agents/<agentID> on http://127.0.0.1:8787`. Open http://127.0.0.1:8787 (use `localhost` or `127.0.0.1`; other host names get 421):

- Multipass: open a tunnel in a host PowerShell tab and keep it open:

  ```powershell
  ssh -L 8787:127.0.0.1:8787 agent-host
  ```

- Lima: nothing to do (automatic forward).

Stop with Ctrl+C. Remove the binary if no longer needed:

```bash
sudo rm -f /home/agent-operator/.local/bin/agent_view
```

Check: [B12 in APPENDIX.md](APPENDIX.md#b12-watch-the-run-live-with-agent-view)

### 13. Results and independent verification

Goal: find the run's outputs and check the reported results against the raw simulation values.

Where: VM. Paths are inside the workspace (`/workspace` in the container).

| Path | Content |
|---|---|
| `.output/<runID>/REPORT.md` | Final report |
| `.output/<runID>/report.md` | Run summary (exit state) |
| `.input/boost_design/<corner>.cir` | Latest netlist per corner (`v09_l02 v09_l20 v12_l02 v12_l20 v15_l02 v15_l20`) |
| `.output/exec/bst<N>/` | One folder per ngspice run: `result.json` (`exit`, `cause`), `stderr.log`, `in/netlist`, `out/sim.log` (`.meas` values), `out/sim.raw` |
| `.agent/<runID>/` | `run.json`, `history.jsonl`, `checkpoint.sqlite`, `nodes/<step>/`, `work/<step>/` (`design.md`, `netlist.md`, `sim.md`, `verify.md`) |

```bash
sudo ls -lR /home/agent-operator/Workspace-Agents/<agentID>/.output/<runID>
sudo cat /home/agent-operator/Workspace-Agents/<agentID>/.output/<runID>/REPORT.md
sudo ls -l /home/agent-operator/Workspace-Agents/<agentID>/.agent/<runID>
sudo ls /home/agent-operator/Workspace-Agents/<agentID>/.output/exec
```

Recompute every requirement from the `.meas` lines; if the report says PASS but your numbers fail, the result is wrong.

Check: [B13 in APPENDIX.md](APPENDIX.md#b13-results-and-independent-verification)

## Exit codes

Goal: map an exit code to its cause.

`run_ephemeral_container` (`result.json` `cause` is authoritative):

| Exit | Cause | Meaning |
|---|---|---|
| 0 | `ok` | The program exited 0 |
| its own | `exit` | The program exited non-zero (read `stderr.log`) |
| 124 | `timeout` | `timeout_s` (or `--timeout`) reached |
| 125 | `usage` | The call does not match the profile; nothing created |
| 125 | `policy` | `spice-safe` refused an input; nothing created |
| 125 | `setup` | Engine unreachable, invalid profile, missing workspace folder, image load or `image_id` mismatch, staging failure |
| 125 | `denied` | The engine authorization plugin refused a request |
| 125 | `verify` | The created container differs from the fixed settings; the command never starts |
| 125 | `busy` | Folder lock held longer than 2 s, or no resource slot |
| 130 | `cancel` | SIGTERM or SIGINT; container killed and removed |
| 137 | `oom` | Memory limit hit |
| 137 | `out_cap` | `out/` grew above `out_max` |

Errors before the exec dir exists (usage, policy, setup, busy) write no `result.json`; the record is the stderr line `run_ephemeral_container: <cause>: <message>`.

`agent_graph run`:

| Exit state | Exit code |
|---|---|
| `COMPLETED` | 0 |
| `NOT_STARTED` | 1 |
| `BUILD_FAILED`, `CONTEXT_OVER_BUDGET`, `NODE_FAILED`, `BUDGET_EXCEEDED`, `LLM_UNAVAILABLE`, `OUTPUT_INVALID` | 2 |
| `INTENT_REJECTED`, `PROCESS_INVALID`, `INPUT_INVALID` | 3 |
| `SUSPENDED` | 130 |

`enter_agent` passes the command's exit code through; itself it exits 1 or 2 (`NOT_ENTERED`) or with the create/remove/check-gate code.

`create_agent_host`: 0 created and verified, 1 prerequisite not met (`NOT_CREATED`), 2 runtime failure (`PARTIALLY_CONFIGURED`, `UNVERIFIED`).

## Troubleshooting

Goal: find the cause and fix of a failure from its symptom.

Read in this order: the first stderr line `run_ephemeral_container: exec_dir=<path>`; `<exec_dir>/result.json`, `stderr.log`, `stdout.log`, `meta.json`; then `sudo docker logs agent-<agentID>` for `ephemeral-authz: ... deny <METHOD> <URI>: <reason>`.

| Symptom | Cause and fix |
|---|---|
| `create_agent_host` step 1: `foundation marker /etc/agent-foundation is missing`, or `must run as root` | Not the foundation VM (recreate from the Part 0 config), or missing `sudo`. `bash vm/check-foundation.sh` shows what is lacking |
| `create_agent_host` step 5 or `cloud-init` hangs for many minutes in `apt-get` (`curl -o /dev/null http://archive.ubuntu.com/ubuntu/ls-lR.gz` shows a few KB/s) | Slow route to the main Ubuntu server: use a country mirror (Part 0 `$mirror`, then recreate the VM), or on a running VM `sudo sed -i 's#http://archive.ubuntu.com#http://hu.archive.ubuntu.com#; s#http://security.ubuntu.com#http://hu.archive.ubuntu.com#' /etc/apt/sources.list.d/ubuntu.sources` |
| `agent-restore.service` or `create_agent_net`: `bridge-nf-call-iptables unreadable (is br_netfilter loaded?)` | `br_netfilter` not loaded (an older `create_agent_host` did not load it): `sudo modprobe br_netfilter`, `sudo systemctl restart agent-restore.service`; recreate the VM with the current `create_agent_host` (step 6 loads it at every boot) |
| `create_agent_host` step 2 names `dist/...` or `endpoints.json` | Redo steps 2 and 3, run from `dist/` |
| `create_agent_host` step 1: `does not contain UBUNTU=26.04` | The VM is not Ubuntu 26.04: recreate it with the Part 0 commands (`multipass launch 26.04`, or the Lima file) |
| `create_agent_host` step 1: `the VM has host mount(s) ...` | Remove the mount (Multipass: `multipass umount agent-host`; Lima: keep `mounts: []` in `vm/agent-host.lima.yaml` and recreate the VM) |
| `create_agent_host` step 1: `... recreate the VM (create_agent_host has no repair on rerun)` | `create_agent_host` already ran, `agent-operator` exists, or UID/GID 1001 is taken (the message names it). Delete the VM and recreate (Multipass: `multipass delete agent-host; multipass purge`; Lima: `limactl delete --force agent-host`) |
| `create_agent_host` step 8 lists `systemd reports running` | `systemctl --failed` shows the failed unit; fix it or recreate the VM |
| `create_agent_host` step 8 lists `agent-operator cannot write under <user>'s home ...` | `agent-operator` can write the listed paths, so it could take over the login user. A fresh foundation VM has the home at mode 750: find what changed it, then recreate the VM |
| `create_agent_host` step 8 lists host mounts | A mount appeared during the run: remove it as in the step 1 row above, then recreate the VM |
| `bash vm/check-foundation.sh` prints `FAIL 7` | Expected after `create_agent_host` (Docker installed) |
| `create_agent_image`: `"/bin/ephemeral-authz": not found`, or exit 1 naming `bin/...` or `Dockerfile.base` | Redo steps 2, 3, 5; run from `dist/` |
| Base image build: `503 Service Unavailable` | `snapshot.ubuntu.com` is down: wait and rerun step 5 |
| `unknown profile`, `--in X: the profile declares no input of that name`, `... is not under an allowed folder`, glob mismatch | Compare with `list --json`; fix the input name or path (`*.cir` netlist, `*.lib` includes) |
| `exec dir for --exec-id ... already exists` | Pick a new id or delete the folder |
| `profile <name>: <reason>`, `VALID false`, exit 125 cause `setup` | Fix `profile.yml` (all five limits, `image_id` as `sha256:...`, name equals folder, no unknown fields) |
| `TAR_MATCH false`, or `image.tar holds ... but the profile pins ...` | Rerun the `docker save` and `image_id` commands of step 9 |
| `engine unreachable` | `sudo docker logs agent-<agentID>` for `agent-init:` lines; recreate the container (step 6) |
| `workspace folder <f> is missing` | Create it as root, owned by 1001:1001 (step 8) |
| `container is not running` / `No such container`, `docker logs` ends with `agent-init: forwarding terminated` | After a reboot or `dockerd` restart the container is removed: `sudo enter_agent <agentID>` recreates it |
| `runc create failed: ... operation not permitted`, or `exec ...: permission denied` | Reinstall the agent-sandbox tools (block below), recreate the container |
| `runc create failed: cgroup namespaces aren't enabled in the kernel` | Redo steps 2 to 6 |
| `denied` | The `deny` line in the container log names the violated rule (usually a profile limit above its cap) |
| `busy` | Wait, or lower the profile limits |
| `timeout`, `out_cap`, `oom` | Raise the profile limits within the caps, or fix the command |
| Claude not logged in, or `LLM_UNAVAILABLE` | 1. Model API endpoint IP changed: the gate resolves `api.anthropic.com` and `platform.claude.com` to IPs once, at arm time, and does not refresh them automatically. If the agent reached Claude before and now gets `LLM_UNAVAILABLE`, the allowed IPs are stale; apply step 2.<br>2. If egress is blocked: `sudo refresh_endpoints --dry-run < dist/endpoints.json; sudo refresh_endpoints < dist/endpoints.json` (repo root), then `sudo create_agent_net check-gate agent-<agentID>` (exit 0).<br>3. If Claude is not logged in: `sudo enter_agent <agentID> -- claude` |
| S3 `no_agent_snippet`, or a start check about `CLAUDE.md`, `.claude` or skills | Add a `layer: agent` snippet in `identity/.snippet/` (step 8) or remove the leftover Claude files; then `/resolved` |
| S3 `tools_not_subset`, or `boost-design` not offered | `sudo enter_agent <agentID> -- grep -c tool_entry_comma /opt/agent_graph/agent_graph/process/validate.py` must print 0 (else redo steps 2 to 6) |
| `BUDGET_EXCEEDED` in `boost-design` | Read `verify.md`, `sim.md`, `design.md` under `.agent/<runID>/work/` for the failing requirement |
| `--resume` refused | Run is `ENDED`, wrong ID, or unreadable `run.json`: `sudo ls /home/agent-operator/Workspace-Agents/<agentID>/.agent` |

Reinstall the agent-sandbox tools (repo root, after rebuilding in step 2), then recreate the container:

```bash
cd dist
for t in create_agent_container remove_agent_container agent_lifecycle enter_agent; do sudo install -o root -g root -m 755 $t /usr/local/bin/$t; done
cd ..
sudo systemctl restart agent-guard.service
sudo remove_agent_container <agentID>
sudo create_agent_container <agentID>
```

## Optional: VS Code

Goal: edit and run commands in the VM from VS Code on the host.

Where: host, after Part 0 (the SSH alias `agent-host` works). Install the Remote-SSH extension, then run "Remote-SSH: Connect to Host..." and pick `agent-host`:

```
code --install-extension ms-vscode-remote.remote-ssh
```

## Teardown

Goal: remove the VM and everything the runbook created on the host.

Where: host. Deleting the VM loses the images, workspaces and the Claude login inside it; copy out any results first (Part 0, "Files in and out").

Multipass (PowerShell):

```powershell
multipass delete --purge agent-host                       # remove the VM and its disk
ssh-keygen -R agent-host                                  # remove its host key from known_hosts
Remove-Item <repo-dir>\agent-host.local.yaml              # local cloud-init copy with your public key
```

Lima (bash or zsh):

```bash
limactl delete --force agent-host                         # stop and remove the VM and its disk
ssh-keygen -R agent-host                                  # remove its host key from known_hosts
```

Both: delete the `Host agent-host` block from `~/.ssh/config` (Windows: `$env:USERPROFILE\.ssh\config`). To also drop the dedicated key, delete `~/.ssh/agent-host_ed25519` and `~/.ssh/agent-host_ed25519.pub`; keep them if you will recreate the VM.
