# TC1: boost converter design, run by agent-graph, verified with ngspice, watched in agent-view

End-to-end test case. `agent_graph` runs Claude through the process `boost-design`: Claude sizes L and C of a synchronous boost converter by hand, writes one ngspice netlist per corner, simulates all six corners with the `ngspice` tool (`run_ephemeral_container tool ngspice`), judges the requirements from the measured numbers and writes a report. `agent_view` shows every simulation (schematic and waveforms) while the run goes on.

Everything needed is in this folder; the commands below are copied inline. Conventions (as in the [howto](../../howto/README.md)):

- PowerShell = Windows PowerShell 7.4+. Dev distro = your Ubuntu WSL distro (name `Ubuntu` below). agent-host = the `agent-host` WSL distro. Every `wsl.exe -d agent-host` command runs from PowerShell.
- `<repo-wsl-path>` = absolute path of this repo checkout in the dev distro (`pwd` in the `chapter02/` folder, the one that holds `packages/` and `howto/`). Commands that say "repo root" run from there.
- `<agentID>` must match `^[a-z][a-z0-9-]{0,30}$`; the container is `agent-<agentID>`; its workspace on agent-host is `/home/agent-operator/Workspace-Agents/<agentID>`, mounted at `/workspace`.
- `<runID>` = `<PROCESS_ID>-<yyyymmdd-hhmmss>`, printed when the run starts (here `boost-design-<yyyymmdd-hhmmss>`).
- `C:\agent-setup` = Windows staging folder of the built binaries (any path works).

## Layout

```
test/TC1_boost_converter/
  README.md
  workspace/                                  copied verbatim to /workspace
    .process/boost-design.yml                 process: calc -> netlist -> simulate -> evaluate -> report
    .tools/ngspice/profile.yml                ngspice tool profile (image_id patched in step 1)
    .tools/ngspice/Dockerfile                 image recipe (ubuntu:26.04 + apt ngspice + ngspice-run wrapper)
    identity/.snippet/agent-identity.md       layer agent
    identity/.snippet/boost-process.md        layer process: the rules ("CLAUDE.md" of this test)
    identity/.skills/boost-converter-design/SKILL.md   sizing, bench filling, reading .meas
    src/ngspice/readraw.py                    sim.raw reader
    src/ngspice/boost_design/HW_REQ.md        requirements (process ruleset)
    src/ngspice/boost_design/bench.cir        fixed bench template (process template)
    src/ngspice/boost_design/reference_12V_2A.cir   filled reference netlist, for the smoke test
```

## Requirements under test

Full text in `workspace/src/ngspice/boost_design/HW_REQ.md`.

| ID | Requirement |
|---|---|
| HW-BST-01 | Vin 9 V to 15 V (nominal 12 V) |
| HW-BST-02 | Vout average 24 V +/- 2 % at every corner |
| HW-BST-03 | Vout ripple p-p <= 240 mV (1 %) at every corner |
| HW-BST-04 | Inductor ripple p-p <= 30 % of IL_avg at Iout = 2 A, all Vin (0.2 A: record only) |
| HW-BST-05 | Load 0.2 A to 2 A, resistive (120 ohm / 12 ohm) |
| HW-BST-06 | fsw = 200 kHz |
| HW-BST-07 | One L and one C for all corners, E6 values >= 1.1 x calculated minimum |
| HW-BST-08 | Steady state: abs(vout_avg - vout_avg_prev) <= 0.5 % of 24 V |

Corners (Vin x Iout): `v09_l02 v09_l20 v12_l02 v12_l20 v15_l02 v15_l20`. Parts are ideal (no DCR, no ESR, switches Ron 1 mohm / Roff 1 Mohm, no dead time, forced CCM, open loop).

## Where "CLAUDE.md" went

`agent_graph` refuses a user-written `CLAUDE.md`, `CLAUDE.local.md` or `.claude/` in `/workspace` (start check). The rules a project would put in `CLAUDE.md` are therefore delivered as snippets in `identity/.snippet/`:

| Would-be content | Delivered as |
|---|---|
| Who the agent is | `agent-identity.md` (layer `agent`, composed into `system.md` of each step) |
| Process rules (units, never edit bench/requirements, exec ids, reporting) | `boost-process.md` (layer `process`, composed into the step `CLAUDE.md`) |
| Step procedure knowledge | skill `boost-converter-design` (copied to `.claude/skills/` of each step) |

The process file must name the snippet and the skill (`runtime.snippets: [boost-process]`, `runtime.skills: [boost-converter-design]`); a name that is missing from the pools is a finding at the start of the run.

## 0. Prerequisites

Done once, with the same `<agentID>` everywhere. All of this is described in the [howto](../../howto/README.md), Part A steps 1 to 8 (checks B1 to B8); check that it is in place:

```powershell
wsl -l -v                                                                                       # agent-host, VERSION 2
wsl.exe -d agent-host -u root -e docker image ls agent-image                                    # image with agent_graph and run_ephemeral_container
wsl.exe -d agent-host -u root -e docker ps                                                      # agent-<agentID> is listed
wsl.exe -d agent-host -u root -e enter_agent <agentID> -- /usr/local/bin/run_ephemeral_container list
wsl.exe -d agent-host -u root -e enter_agent <agentID> -- agent_graph --help
wsl.exe -d agent-host -u root -e ls -ld /home/agent-operator/Workspace-Agents/<agentID>
```

- Claude is logged in inside the container (`enter_agent <agentID> -- claude`, then exit).
- The agent image has an `agent_graph` that accepts `Name(spec)` entries in a step's `tools`: `enter_agent <agentID> -- grep -c tool_entry_comma /opt/agent_graph/agent_graph/process/validate.py` prints `1` or more. If not, rebuild the image and recreate the container as in the [howto](../../howto/README.md) steps 2 to 6.
- No `CLAUDE.md`, `CLAUDE.local.md`, `.claude/` in `/workspace`, no `/home/agent/.claude/skills/` content (checked in step 2).

Create the workspace root if missing:

```powershell
wsl.exe -d agent-host -u root -e install -d -o agent-operator -g agent-operator /home/agent-operator/Workspace-Agents
```

## 1. Copy the workspace to agent-host

Where: PowerShell. Copies the whole `workspace/` folder, dotdirs included (`.process`, `.tools`), owned by uid/gid 1001. The image tar of section 2 lands in `.tools/ngspice/`, next to `profile.yml`.

```powershell
wsl.exe -d agent-host -u root -e install -d -o 1001 -g 1001 /home/agent-operator/Workspace-Agents/<agentID>
wsl.exe -d Ubuntu -u root -e tar -C <repo-wsl-path>/test/TC1_boost_converter/workspace --owner=1001 --group=1001 --numeric-owner -cf - . | wsl.exe -d agent-host -u root -e tar -C /home/agent-operator/Workspace-Agents/<agentID> -xf -
```

The pipe copies the dotdirs because the source is `.` (on a single machine the equivalent is `cp -a workspace/. <dest>/`). Create the folder where Claude writes the netlists (owned by uid 1001, because `enter_agent` runs as uid 1001) and remove leftovers of an earlier run of this test:

```powershell
wsl.exe -d agent-host -u root -e sh -c 'W=/home/agent-operator/Workspace-Agents/<agentID>; rm -rf $W/.output/exec/bst* $W/.input/boost_design'
wsl.exe -d agent-host -u root -e enter_agent <agentID> -- mkdir -p /workspace/.input/boost_design
```

Check the files and the start preconditions of `agent_graph`:

```powershell
wsl.exe -d agent-host -u root -e enter_agent <agentID> -- ls -la /workspace /workspace/.process /workspace/.tools/ngspice /workspace/identity/.snippet /workspace/identity/.skills /workspace/src/ngspice/boost_design /workspace/.input/boost_design
wsl.exe -d agent-host -u root -e enter_agent <agentID> -- sh -c 'ls -la /home/agent/.claude; ls -A /home/agent/.claude/skills; find /workspace -maxdepth 2 \( -name CLAUDE.md -o -name CLAUDE.local.md -o -name .claude \)'
```

Expected: everything owned by 1001; `boost-design.yml` in `.process`; `agent-identity.md` and `boost-process.md` in `identity/.snippet`; `boost-converter-design/` in `identity/.skills`; `HW_REQ.md`, `bench.cir`, `reference_12V_2A.cir` in `boost_design`; an empty `.input/boost_design`; no `skills` content; `find` prints nothing. Other files already in the workspace are untouched; the copy only overwrites files with the same names.

## 2. Build the ngspice image and install the profile

Where: PowerShell. Needs section 1 done (`profile.yml` must be in place to be patched). The tool image is built in agent-host (never in the dev distro) and saved as `.tools/ngspice/image.tar` in the workspace; the profile pins its image ID. The build needs network access for `apt-get`.

The profile runs the wrapper `ngspice-run` (created by the Dockerfile), which runs ngspice twice because ngspice skips `.meas` in batch mode when `-r` is set: once without `-r` (`.meas` results in `sim.log`), once with `-r` (`sim.raw`, log in `sim_raw.log`). After any change to the Dockerfile, rebuild the image and re-patch `image_id` with the commands below (build, save, patch).

Build the image from the Dockerfile in this test (read from stdin, no build context):

```powershell
wsl.exe -d Ubuntu -u root -e cat <repo-wsl-path>/test/TC1_boost_converter/workspace/.tools/ngspice/Dockerfile | wsl.exe -d agent-host -u root -e docker build -t exec-ngspice:1 -
wsl.exe -d agent-host -u root -e docker image inspect -f '{{.Id}}' exec-ngspice:1
```

Save the image into the workspace (can take a minute) and patch `image_id` in the profile:

```powershell
wsl.exe -d agent-host -u root -e sh -c 'D=/home/agent-operator/Workspace-Agents/<agentID>/.tools/ngspice; mkdir -p $D && docker save -o $D/image.tar exec-ngspice:1 && chown -R 1001:1001 $D'
wsl.exe -d agent-host -u root -e sh -c 'D=/home/agent-operator/Workspace-Agents/<agentID>/.tools/ngspice; ID=$(docker image inspect -f "{{.Id}}" exec-ngspice:1) && sed -i "s|^image_id: .*|image_id: $ID|" $D/profile.yml && chown -R 1001:1001 $D && grep ^image_id $D/profile.yml'
```

Check: the last command prints `image_id: sha256:<64 hex>`, and `list` shows the profile as valid:

```powershell
wsl.exe -d agent-host -u root -e enter_agent <agentID> -- /usr/local/bin/run_ephemeral_container list
wsl.exe -d agent-host -u root -e enter_agent <agentID> -- /usr/local/bin/run_ephemeral_container list --json
```

Expected: a row `ngspice tool ... VALID true`, `TAR true`, `TAR_MATCH true` (`LOADED` is `false` until the first run). In the JSON, `"input_policy": "spice-safe"` and the `netlist` input allows `src/`, `.input/` and `.output/exec/`.

## 3. Optional: smoke-test the tool with the reference netlist

Where: PowerShell. `reference_12V_2A.cir` is a filled bench (Vin 12 V, 2 A corner) that was prepared by hand; it checks the image, the profile and the `spice-safe` policy before any Claude run. Exec id `smoke1`.

```powershell
wsl.exe -d agent-host -u root -e enter_agent <agentID> -- sh -c 'run_ephemeral_container tool ngspice --exec-id smoke1 --in netlist=/workspace/src/ngspice/boost_design/reference_12V_2A.cir; echo "exit=$?"'
wsl.exe -d agent-host -u root -e sh -c 'W=/home/agent-operator/Workspace-Agents/<agentID>; cat $W/.output/exec/smoke1/result.json; ls $W/.output/exec/smoke1/out'
wsl.exe -d agent-host -u root -e enter_agent <agentID> -- sh -c 'grep -E "^(vout_avg|vout_avg_prev|vout_pp|il_avg|il_pp)" /workspace/.output/exec/smoke1/out/sim.log'
wsl.exe -d agent-host -u root -e enter_agent <agentID> -- python3 -I /workspace/src/ngspice/readraw.py /workspace/.output/exec/smoke1/out/sim.raw --at 0.0009,0.00095,0.001
```

Expected: `exit=0`; `result.json` with `"exit": 0` and `"cause": "ok"`; `sim.log`, `sim.raw` and `sim_raw.log` in `out/`; five `.meas` lines (for the reference design `vout_avg` is close to 24 V). The first stderr line is `run_ephemeral_container: exec_dir=/workspace/.output/exec/smoke1`. An exec id works only once; to rerun, use `smoke2`, or delete the old folder:

```powershell
wsl.exe -d agent-host -u root -e rm -rf /home/agent-operator/Workspace-Agents/<agentID>/.output/exec/smoke1
```

Optional plot, opened on Windows:

```powershell
wsl.exe -d agent-host -u root -e enter_agent <agentID> -- python3 -I /workspace/src/ngspice/readraw.py /workspace/.output/exec/smoke1/out/sim.raw --svg /workspace/.output/smoke1.svg --vars 'v(out)'
wsl.exe -d agent-host -u root -e cat /home/agent-operator/Workspace-Agents/<agentID>/.output/smoke1.svg > $env:TEMP\smoke1.svg; start $env:TEMP\smoke1.svg
```

## 3a. Validate the TC1 fixture (offline)

Where: dev distro, `chapter02/` folder (repo root of this runbook). Loads the process, validates it and checks its skill and snippet declarations against the pools; no container needed. Do not use `python3 -I` here (it drops `PYTHONPATH`).

```bash
PYTHONPATH=packages/agent-graph/src python3 - <<'EOF'
from pathlib import Path
from agent_graph.services.runfs import WorkspacePaths
from agent_graph.process import loader, validate, pools
W = Path("test/TC1_boost_converter/workspace")
ws = WorkspacePaths(root=W)
proc, f = loader.load_process(W / ".process/boost-design.yml", ws)
print("load", f)
print("validate", validate.validate_process(proc, ws))
idx, idx_findings = pools.build_index(ws)
print("pool index", idx_findings)
print("pools", pools.validate_against_pools(proc, idx))
EOF
```

Expected: each printed list is empty (`load []`, `validate []`, `pool index []`, `pools []`).

## 4. Run the process

Where: PowerShell, an interactive terminal (non-interactive runs are not supported). It can take several minutes. Optional settings go through `env` (defaults `opus`, `medium`, pinned in `run.json` at the start):

```powershell
wsl.exe -d agent-host -u root -e enter_agent <agentID> -- agent_graph run
# or: wsl.exe -d agent-host -u root -e enter_agent <agentID> -- env AGENT_GRAPH_MODEL=sonnet AGENT_GRAPH_EFFORT=low agent_graph run
```

Start the viewer (section 5) first if you want to watch the simulations live. Dialog (every line is prefixed with its source, `[S2 clarify]`, `[node simulate]`, ...):

1. S2: the runtime lists the processes in `.process/`. Answer `Which process do you want to run?` with, for example, `Design the 24 V boost converter from the hardware requirements and verify it with ngspice`. Answer Claude's questions, if any, until it proposes `boost-design`, then confirm `Run process boost-design (<title>)? (y/n)` with `y`. The runtime prints `Run boost-design-<yyyymmdd-hhmmss> created for process boost-design.` Note this run ID.
2. S3, S4: validation and input binding; nothing to answer unless a finding opens S7 (a read-only Claude explains it and proposes a fix; nothing changes without your `y`; `/quit`, confirm `y`, leaves the run).
3. S5: the outline (nodes `calc`, `netlist`, `simulate`, `evaluate`, `report`; edges; budgets). Confirm `Start the run? (y/n)` with `y`.
4. S6: per step you see Claude's text, panels for tool calls and results, and `STEP n: DONE`. Expected order: `calc` (hand calculation, writes the design), `netlist` (writes six netlists to `/workspace/.input/boost_design/`), `simulate` (six `run_ephemeral_container tool ngspice` calls with new exec ids, reads `.meas`), `evaluate` (requirement table), `report`. A tool error goes back to `netlist`; a failed requirement goes back to `calc`. No step should ask you anything; if one does, answer briefly.
5. S8: the run summary. Expected: exit code 0 and `exit_state: COMPLETED`.

Interrupt and resume: Ctrl-C, a closed terminal or `/quit` at a question suspends the run (exit 130). Finished steps are not repeated on resume (see section 8).

## 5. Watch the simulations in agent-view

Optional but recommended. `agent_view` is a read-only viewer of `<ws>/.output/exec/<execId>/`; it serves GET only on loopback and updates live (SSE) while the run goes on. It runs on agent-host as `agent-operator`.

Build (dev distro; `npm run build` must run before `go build`, the Go binary embeds the UI):

```bash
cd <repo-wsl-path>/packages/agent-view/web
npm ci
npm run build
cd ..
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o ../../dist/agent_view ./cmd/agent_view
sha256sum ../../dist/agent_view
```

Copy into agent-host (PowerShell; `cmd /c` keeps the binary pipe byte-exact):

```powershell
cmd /c "wsl.exe -d Ubuntu -e cat <repo-wsl-path>/dist/agent_view | wsl.exe -d agent-host -u agent-operator -e sh -c 'mkdir -p ~/.local/bin && cat > ~/.local/bin/agent_view.tmp && chmod 755 ~/.local/bin/agent_view.tmp && mv -f ~/.local/bin/agent_view.tmp ~/.local/bin/agent_view'"
wsl.exe -d agent-host -u agent-operator -e sha256sum /home/agent-operator/.local/bin/agent_view      # same hash as above
```

Start (its own Windows Terminal tab; keep it open, it also keeps agent-host from idling out):

```powershell
wsl.exe -d agent-host -u agent-operator -e /home/agent-operator/.local/bin/agent_view --ws /home/agent-operator/Workspace-Agents/<agentID> --listen 127.0.0.1:8787
```

Expected stderr: `agent_view: serving /home/agent-operator/Workspace-Agents/<agentID> on http://127.0.0.1:8787`. Check from another PowerShell tab, then open http://127.0.0.1:8787 in a Windows browser:

```powershell
curl.exe -s http://localhost:8787/api/execs
```

If `curl.exe` fails from Windows but this works, WSL localhost forwarding is off on your machine (host config is not changed here: view from inside agent-host only):

```powershell
wsl.exe -d agent-host -u agent-operator -e curl -s http://127.0.0.1:8787/api/execs
```

Only loopback is accepted: `--listen` must be 127.0.0.1 (or `::1`), and the page must be opened through `localhost` or `127.0.0.1` (another host name gives 421).

What you see:

- Exec selector, newest first, with cause and exit badge per exec. "Follow newest" is on by default: the view switches to each new exec as `simulate` runs its corners.
- Schematic drawn from `in/netlist` (the exact file ngspice ran): the converter with L, C, both switches and the load; the per-corner values (Vin, L, C, Rload, duty width) are those of that exec's netlist. Click a net to toggle its `v(net)` trace.
- Waveform from `out/sim.raw`: `v(out)` and `i(vsense)` (the inductor current) over the 1 ms run, V and A on separate axes; drag to zoom, double-click to reset; the cursor shows values at that time. A settled run shows `v(out)` flat around 24 V with a small ripple at 200 kHz.
- An exec with an error shows its cause and `stderr.log` instead of plots.

Stop with Ctrl+C in the viewer tab. Remove the binary:

```powershell
wsl.exe -d agent-host -u agent-operator -e rm -f /home/agent-operator/.local/bin/agent_view
```

## 6. Where the results are

Paths inside the workspace (`/workspace` in the container, `/home/agent-operator/Workspace-Agents/<agentID>` on agent-host).

| Path | Content |
|---|---|
| `.output/exec/<id>/` | One folder per ngspice run: `result.json`, `stdout.log`, `stderr.log`, `meta.json`, `in/netlist`, `out/sim.log`, `out/sim.raw`, `out/sim_raw.log` |
| `.input/boost_design/<corner>.cir` | The latest netlist per corner |
| `.input/boost_design/<corner>.svg` | Optional waveform plot of `v(out)` per corner (written by `simulate` through `readraw.py` when `sim.raw` exists; linked from `REPORT.md` when present) |
| `.agent/<runID>/nodes/<step>/` | Per step: `system.md`, `CLAUDE.md` (generated contract plus the process snippet), `.claude/skills/`, `context.md`, `result.json`, `transcript.jsonl` (stream-json of Claude's turns); an earlier visit of a repeated step is `nodes/<step>.1`, `.2`, ... |
| `.agent/<runID>/work/<step>/` | Step products (kept or archived according to the process) |
| `.output/<runID>/REPORT.md` | The final report (output target of `report`) |
| `.output/<runID>/report.md` | The run summary of S8 (exit state) |

Commands (replace `<runID>` and `<id>`):

```powershell
wsl.exe -d agent-host -u root -e ls -lR /home/agent-operator/Workspace-Agents/<agentID>/.output/<runID>
wsl.exe -d agent-host -u root -e cat /home/agent-operator/Workspace-Agents/<agentID>/.output/<runID>/REPORT.md
wsl.exe -d agent-host -u root -e ls -l /home/agent-operator/Workspace-Agents/<agentID>/.agent/<runID>
wsl.exe -d agent-host -u root -e ls /home/agent-operator/Workspace-Agents/<agentID>/.output/exec
wsl.exe -d agent-host -u root -e cat /home/agent-operator/Workspace-Agents/<agentID>/.output/exec/<id>/result.json
wsl.exe -d agent-host -u root -e enter_agent <agentID> -- sh -c 'grep -E "^(vout_avg|vout_avg_prev|vout_pp|il_avg|il_pp)" /workspace/.output/exec/<id>/out/sim.log'
```

## 7. Expected result / acceptance

Do not trust the report: check the files and the numbers yourself.

Reference answer (operator only, not given to Claude):

| Quantity | Expected |
|---|---|
| L | 33 uH (L_min = 29.3 uH, set by Vin = 15 V at 2 A; x 1.1 = 32.2 uH; next E6 value 33 uH) |
| C | 33 uF (C_min = 26 uF, set by Vin = 9 V at 2 A; x 1.1 = 28.6 uF; next E6 value 33 uF) |
| Duty D | 0.625 (9 V), 0.5 (12 V), 0.375 (15 V) |
| `D_TON` | D x 5 us - 1 ns: 3.124 us, 2.499 us, 1.874 us |
| `IL0` (= IL_avg at 2 A) | 5.333 A (9 V), 4 A (12 V), 3.2 A (15 V) |
| `VOUT0` | 24 V |

Another valid design is possible: any single L and C from E6 that is >= 1.1 x the minimum and passes all requirements in simulation is acceptable (for example a larger E6 value). A smaller value than the minimum, or a value off the E6 series, fails HW-BST-07.

Pass criteria (all):

- The run ended with exit code 0 and `exit_state: COMPLETED`.
- Six netlists `/workspace/.input/boost_design/<corner>.cir` exist for the six corners, identical to `bench.cir` except for the placeholder values; no placeholder `<...>` left, no `.control`, no `.include`. All six use the same L and the same C, both E6 values.
- Per corner the values match the table above (Vin, Rload 120 ohm or 12 ohm, `D_TON`, `IL0`, `VOUT0`). Check D_TON by hand: D x 5 us - 1 ns.
- Six exec dirs (at least; tool-error loops or a design iteration add more), each with `"exit": 0` and `"cause": "ok"` for the final design, and the exec ids are all distinct.
- For the final design, from `out/sim.log` of each corner: 23.52 V <= `vout_avg` <= 24.48 V; `vout_pp` <= 0.240 V; abs(`vout_avg` - `vout_avg_prev`) <= 0.120 V; at the three 2 A corners: `il_pp / il_avg` <= 0.30. Recompute these yourself from the `.meas` lines.
- Sanity check for L = 33 uH and C = 33 uF (hand calculation, ideal parts, CCM; ngspice has the final word): `il_pp` about 0.85 A (9 V), 0.91 A (12 V), 0.85 A (15 V), the same at both loads; `vout_pp` about 0.19 V at 9 V / 2 A.
- `.output/<runID>/REPORT.md` quotes every number with its corner and measure (HW-BST-02, -03, -04, -08 per corner; -07 with the calculation), matches your values, and its last line is exactly `RESULT: PASS`.
- Claude ran ngspice only through `run_ephemeral_container tool ngspice` (check the transcripts) and never edited `bench.cir` or `HW_REQ.md`:

```powershell
wsl.exe -d agent-host -u root -e enter_agent <agentID> -- sh -c 'for d in /workspace/.agent/<runID>/nodes/simulate*; do echo "== $d"; grep -o "\"command\": \"[^\"]*\"" $d/transcript.jsonl; done'
wsl.exe -d agent-host -u root -e enter_agent <agentID> -- sh -c 'cd /workspace/src/ngspice/boost_design && ls -l HW_REQ.md bench.cir'
```

If the report says PASS but your own numbers fail (or the run ended without a pass but your numbers pass for its final design), the process result is wrong and the test fails.

Known failure causes (details in `workspace/identity/.skills/boost-converter-design/SKILL.md`, section 6): leftover placeholder or `M` instead of `Meg` in a netlist (ngspice error in `stderr.log`), a `D_TON` that is not D x T - 1 ns (settled but off by a few percent), wrong `IL0`/`VOUT0` (run not settled, `vout_avg` differs from `vout_avg_prev`), a reused exec id (exit 125, cause `usage`), a `.control`/`.include`/`.lib` line (exit 125, `policy: spice-safe`).

## 8. Cleanup and rerun

A used exec id cannot be reused, and the netlists of a previous run are overwritten by the next. Before a new run, remove the old exec dirs and netlists (workspace files only; the image, profile and login are kept):

```powershell
wsl.exe -d agent-host -u root -e sh -c 'W=/home/agent-operator/Workspace-Agents/<agentID>; rm -rf $W/.output/exec/bst* $W/.output/exec/smoke* $W/.input/boost_design/*.cir'
```

If you copied a changed `workspace/` (process, snippet or skill) after a run was created, start a new run: a changed process file makes the old run's binding stale. Copy again with the tar command of section 1 (it overwrites files with the same names; the `image_id` patch in `.tools/ngspice/profile.yml` is overwritten too: redo the `sed` command of section 2).

New run: section 4 (a new `<runID>`). `simulate` names its exec ids `bst<N>`: the smallest base B >= 1 such that `bst<B>` to `bst<B+5>` are all absent from `.output/exec/`, then N = B + corner index (0 to 5). Older exec dirs of other runs therefore do not collide.

Resume an interrupted run (state saved, finished steps are not repeated, a Claude step continues its session):

```powershell
wsl.exe -d agent-host -u root -e ls /home/agent-operator/Workspace-Agents/<agentID>/.agent
wsl.exe -d agent-host -u root -e enter_agent <agentID> -- agent_graph run --resume <runID>
```

`--resume` refuses a run that is `ENDED` (completed or rejected). After a `/quit` in S7 the run stays `ESCALATED` and `--resume` re-enters S7.

Remove the test workspace files entirely (keeps the container and the Claude login):

```powershell
wsl.exe -d agent-host -u root -e sh -c 'W=/home/agent-operator/Workspace-Agents/<agentID>; rm -rf $W/.process/boost-design.yml $W/.tools/ngspice $W/identity/.snippet/boost-process.md $W/identity/.skills/boost-converter-design $W/src/ngspice/boost_design $W/.input/boost_design $W/.output/exec/bst* $W/.output/exec/smoke*'
```

Leave `agent-identity.md` and `src/ngspice/readraw.py` in place if other tests of this workspace use them.
