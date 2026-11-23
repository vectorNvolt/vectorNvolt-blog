# Appendix: boost converter demo runbook

## Appendix A. Placeholders

| Placeholder | Meaning |
|---|---|
| `<agentID>` | Agent name, `^[a-z][a-z0-9-]{0,30}$`; container `agent-<agentID>`; workspace `/home/agent-operator/Workspace-Agents/<agentID>` (`/workspace` in the container) |
| `<repo-url>` | Git URL of this repo holding agent harness code |
| `<repo-dir>` | Folder of the repo checkout (holds `packages/`, `vm/`, `howto/`) |
| `<vm-ip>` | Multipass only: IPv4 from `multipass info agent-host` |
| `<ssh-port>` | Lima only: host port on `127.0.0.1` forwarded to the VM's SSH port (`limactl list agent-host --format '{{.SSHLocalPort}}'`) |
| `<runID>` | `<PROCESS_ID>-<yyyymmdd-hhmmss>`, printed when an `agent_graph` run starts |
| `<execId>` | Tool execution folder name `.output/exec/<execId>`, `[a-z0-9][a-z0-9-]{0,63}` |

## Appendix B. Checks

Run in step order; all in the VM unless marked host.

### B1. Prerequisites

Where: VM, repo root.

```bash
cat /etc/agent-foundation           # FOUNDATION_VERSION=1, GO_VERSION=1.27.4, UBUNTU=26.04
/usr/local/go/bin/go version        # go1.27.4
bash vm/check-foundation.sh         # PASS 1 .. PASS 7 (before create_agent_host; item 7 fails after it)
```

```bash
grep -c -- '--hash=' packages/agent-image/requirements.lock    # non-zero
grep '^FROM' packages/agent-image/Dockerfile.base              # shows @sha256
```

### B2. Build everything in one pass

Where: VM, repo root. All step 2 commands exit 0.

```bash
ls dist/bin dist/agent_graph/__init__.py dist/agent_graph/__main__.py dist/agent_graph/schemas
uname -m                           # x86_64 or aarch64
```

Expected: `agent-init`, `ephemeral-authz`, `run_ephemeral_container`, and the three `agent_graph` entries.

### B3. Check the build output

Where: VM, repo root. The loop of step 3 prints `ok` for all 18 files and no `MISSING`.

### B4. Create agent-host

Where: VM, repo root.

```bash
hostname                                              # agent-host
sudo docker info --format '{{json .Runtimes}}'        # contains runsc
sudo docker run --rm --runtime=runsc hello-world
sudo stat -c '%U:%G %a' /etc/agent-net/endpoints.json   # root:root 644
```

Every tool is byte-identical to its `dist/` build, `root:root`, mode 755; restore is `active (exited)`, guard `active (running)`:

```bash
for t in create_agent_net create_agent_container remove_agent_container remove_agent_net agent_lifecycle enter_agent refresh_endpoints; do cmp dist/$t /usr/local/bin/$t && echo "same $t"; done
ls -l /usr/local/bin/create_agent_net /usr/local/bin/create_agent_container /usr/local/bin/remove_agent_container /usr/local/bin/remove_agent_net /usr/local/bin/agent_lifecycle /usr/local/bin/enter_agent /usr/local/bin/refresh_endpoints
sudo systemctl status agent-restore.service agent-guard.service --no-pager
sudo cat /run/agent-sandbox/ready      # dockerd MainPID
```

`id agent-operator` shows `uid=1001(agent-operator)`; `ls -ld` shows the folder; the done marker shows `AGENT_HOST_VERSION=1`; the apt file blocks automatic upgrades of the Docker packages; the login home is `drwxr-x---`:

```bash
id agent-operator
sudo ls -ld /home/agent-operator/Workspace-Agents
cat /etc/agent-host
cat /etc/apt/apt.conf.d/51agent-host-no-docker-upgrade
ls -ld ~
```

The `agents-net` network exists:

```bash
sudo docker network inspect agents-net
```

If it is missing, rebuild by hand and restart the service:

```bash
sudo sh -c 'remove_agent_net; create_agent_net'
sudo systemctl restart agent-restore.service
```

If `create_agent_net` fails with a leftover-bridge or overlap error: `sudo reboot`, open a new shell, check again.

### B5. Build the agent image

Where: VM. `agent-image:latest` is listed with a fresh CREATED time.

```bash
sudo docker image ls agent-image
```

### B6. Create the agent container

Where: VM. Both lines print the same ID:

```bash
sudo docker image inspect -f '{{.Id}}' agent-image:latest
sudo docker inspect -f '{{.Image}}' agent-<agentID>
```

After the Claude login, `check-gate` exits 0:

```bash
sudo create_agent_net check-gate agent-<agentID>
```

### B8. Workspace folders

Where: VM.

```bash
sudo enter_agent <agentID> -- ls -la /workspace /workspace/.process /workspace/.tools/ngspice /workspace/identity/.snippet /workspace/identity/.skills /workspace/src/ngspice/boost_design /workspace/.input/boost_design
```

Expect: everything owned by 1001; `boost-design.yml` in `.process`; `profile.yml` and `Dockerfile` in `.tools/ngspice`; `agent-identity.md` and `boost-process.md` in `identity/.snippet`; `boost-converter-design/` in `identity/.skills`; `HW_REQ.md`, `bench.cir`, `reference_12V_2A.cir` in `src/ngspice/boost_design`; `readraw.py` in `src/ngspice`; empty `.input/boost_design`.

### B9. ngspice image and profile

Where: VM. `list` shows `VALID true`, `TAR true`, `TAR_MATCH true` (`LOADED` is `false` until the first run).

```bash
sudo enter_agent <agentID> -- /usr/local/bin/run_ephemeral_container list
sudo enter_agent <agentID> -- /usr/local/bin/run_ephemeral_container list --json
```

The JSON has `"input_policy": "spice-safe"` and a `netlist` input allowing `src/`, `.input/` and `.output/exec/`.

### B10. Smoke test

Where: VM. Step 10 output: first stderr line `run_ephemeral_container: exec_dir=/workspace/.output/exec/smoke1`, last line `exit=0`; `result.json` has `"exit": 0` and `"cause": "ok"`; `out/` holds `sim.log`, `sim.raw`, `sim_raw.log`; `grep` prints five `.meas` lines with `vout_avg` near 24 V; `readraw.py` shows `v(out)` near 24 V at 0.9 ms to 1 ms. `list` now shows `LOADED true` for `ngspice`:

```bash
sudo enter_agent <agentID> -- /usr/local/bin/run_ephemeral_container list
```

### B11. Run the boost-design process

Where: VM. After the run: `STEP 1: DONE` to `STEP 5: DONE` (more when a loop ran), `exit_state: COMPLETED`, shell exit code 0 (`echo $?` right after the run). Six netlists and `REPORT.md` exist:

```bash
sudo enter_agent <agentID> -- ls -l /workspace/.input/boost_design /workspace/.output/<runID>
```

### B12. Watch the run live with agent-view

Where: second VM shell while the viewer runs; the host check needs the tunnel (Multipass) or automatic forward (Lima). The viewer lists execs as JSON (`{"execs":[]}` on a fresh workspace, else their ids newest first); the page at http://127.0.0.1:8787 shows the same ids.

```bash
curl -s http://127.0.0.1:8787/api/execs
```

Host:

```powershell
# Multipass (PowerShell)
curl.exe -s http://127.0.0.1:8787/api/execs
```

```bash
# Lima (bash or zsh)
curl -s http://127.0.0.1:8787/api/execs
```

If only the host check fails: Multipass tunnel not open, or Lima forward missing (the viewer must listen on `127.0.0.1:8787` in the VM).

### B13. Results and independent verification

Where: VM. The last line of `REPORT.md` is exactly `RESULT: PASS`:

```bash
sudo enter_agent <agentID> -- tail -n 1 /workspace/.output/<runID>/REPORT.md
```

Per corner, print netlist values and measures of every ngspice run (the report lists the ids of the final design; earlier `bst<N>` are failed iterations or other runs):

```bash
sudo enter_agent <agentID> -- sh -c 'for d in /workspace/.output/exec/bst*; do echo "== $d"; grep -o "\"exit\": *[0-9]*\|\"cause\": *\"[a-z_]*\"" $d/result.json; grep -E "^(VIN|L1|C1|RLOAD|VG) " $d/in/netlist; grep -E "^(vout_avg|vout_avg_prev|vout_pp|il_avg|il_pp)" $d/out/sim.log; done'
```

Pass criteria (all):

- Every final exec has `"exit": 0` and `"cause": "ok"`; ids distinct (at least six).
- Six netlists in `/workspace/.input/boost_design/` equal `bench.cir` except placeholder values; no `<...>`, `.control` or `.include`. All six use the same L and C, both E6 values and at least 1.1 times the calculated minimum (HW-BST-07; reference design L_min = 29.3 uH, C_min = 26 uF).
- Per corner, from `sim.log`:

| Requirement | Measure | Limit |
|---|---|---|
| HW-BST-02 | `vout_avg` | 23.52 V to 24.48 V, every corner |
| HW-BST-03 | `vout_pp` | at most 0.240 V, every corner |
| HW-BST-04 | `il_pp / il_avg` | at most 0.30 at the three 2 A corners (0.2 A: record only) |
| HW-BST-08 | abs(`vout_avg` - `vout_avg_prev`) | at most 0.120 V, every corner |

- Netlist values per corner: `VIN` 9, 12 or 15; `RLOAD` 120 (0.2 A) or 12 (2 A); `VOUT0` (`C1 ... IC=`) 24; `IL0` (`L1 ... IC=`) = Iout x 24 / Vin: 5.333 A (9 V), 4 A (12 V), 3.2 A (15 V) at 2 A; `D_TON` (`VG` pulse width) = D x 5 us - 1 ns with D = 1 - Vin / 24 V: 3.124 us, 2.499 us, 1.874 us.
- `REPORT.md` quotes every number with its corner and measure and matches your values.

Reference answer: L = 33 uH, C = 33 uF; `il_pp` about 0.85 A (9 V), 0.91 A (12 V), 0.85 A (15 V) at both loads; `vout_pp` about 0.19 V at 9 V and 2 A. Any other single E6 L and C meeting the limits is also valid.

Claude must have run ngspice only through `run_ephemeral_container tool ngspice` and not edited `bench.cir` or `HW_REQ.md`:

```bash
sudo enter_agent <agentID> -- sh -c 'for d in /workspace/.agent/<runID>/nodes/simulate*; do echo "== $d"; grep -o "\"command\": \"[^\"]*\"" $d/transcript.jsonl; done'
sudo enter_agent <agentID> -- sh -c 'cd /workspace/src/ngspice/boost_design && ls -l HW_REQ.md bench.cir'
```

Expect only `run_ephemeral_container tool ngspice ...` and `readraw.py` commands, and `HW_REQ.md` and `bench.cir` unchanged from the repo files (compare with `diff`).
