# USS-009 — Agent tool usage, tool runs and hook-protected command control

USS-009 applies inside an agent container created by USS-002, from the image created by USS-003. It defines what the agent itself may do there, in the interactive session and in agent-graph cognitive steps (USS-008): the base toolset, the guarded Bash, the tool kind of the execution tool (moved here from USS-005), and where the policy that limits all this lives. Every execution goes through the ephemeral execution container of USS-005.

Status: partly implemented. `run_ephemeral_container` is implemented (`packages/agent-exec`, USS-005); the kind `tool` is not (see "Implementation status"). Today the interactive session (`enter_agent`, `packages/agent-sandbox/internal/sequence/enter.go:116`) starts plain `claude` with the default Claude Code tool set, including an unrestricted Bash. agent-graph cognitive steps run `claude -p` with `--tools <runtime.tools>` (default `Read, Write`, `packages/agent-graph/src/agent_graph/config.py:19`), `--allowedTools` per step (`services/claude_cli.py:85-86`; whether it really narrows is an open item, see "Enforcement layers"), `--setting-sources project`, `--strict-mcp-config --mcp-config mcp.json` (empty today), cwd `.agent/<run>/active/`, products in `.agent/<run>/work/<node>/`. `runtime.tools` in the process YAML has no allowlist (`process_v1.schema.yml`). No managed settings, `agent-tool-guard` hook or policy mount exist.

Scope: tools whose binary is already on board (Claude Code built-ins and allowlisted commands, including `run_ephemeral_container`), run through Bash under a guarding PreToolUse hook; the kind `tool`; the secure placement of hooks and tool limitation. MCP servers are not covered: MCP-based tools are a separate, later user story. Managed settings allow no MCP server for now.

## Execution model

- The basic codebase tools (`Read`, `Glob`, `Grep`, `Edit`, `Write`, `TodoWrite`) run directly in the agent, with no container.
- Every complex tool a process adds (`runtime.profiles`, the kind `tool`) runs only in an ephemeral container, through `run_ephemeral_container` (USS-005). There is no other way to run it.
- This holds for both step types of agent-graph:
  - cognitive step: the agent calls `run_ephemeral_container tool <profile> ...` through the guarded Bash;
  - procedure step: an isolated step (USS-008) may name a tool profile, and agent-graph runs it through the same exec tool. A non-isolated procedure step cannot run a process tool.

## Participants

| Participant | Role |
|---|---|
| Agent | Claude Code session as user `agent` (uid 1001) in the agent container: the interactive session, or one cognitive step of agent-graph |
| Claude Code | `claude` in the image (USS-003), pinned version, root-owned, started by absolute path `/usr/local/bin/claude`. Reads the managed settings, enforces tool permissions and runs hooks |
| Policy bundle | Read-only bind mount `/etc/claude-code/` (see "Policy placement and bind structure"): managed settings, the hook binary, the guard rules |
| `agent-tool-guard` | Static Go binary `/etc/claude-code/hooks/agent-tool-guard`, run by Claude Code as the managed PreToolUse hook. Decides Bash commands and path arguments. Named `agent-tool-guard` to avoid a clash with the host systemd unit `agent-guard.service` (`packages/agent-host/internal/steps/lifecycle_units.go`) |
| `run_ephemeral_container` | Execution tool of USS-005 (binary in the image). The agent starts it through Bash; it runs as uid 1001 |
| In-container engine | USS-005. Reached directly by the agent user (group `docker`, SSR-AGC-011). Guarded by the engine authorization plugin `ephemeral-authz` (USS-005), layer L4 |
| Execution profile | Operator-supplied `/workspace/.tools/<tool>/profile.yml` (USS-005 "Execution profiles"); for the kind `tool` it is defined below |
| Tool pool | Folder on agent-host holding tool folders (`profile.yml`, `image.tar`, `tool.json`); the operator copies them into `.tools/` (USS-005 "Profile store and supply") |
| agent-graph runtime | USS-008. Sets the tools, environment and write grant of each cognitive step; runs the integrity check |
| Developer | Owns the process package; cannot change the platform policy |
| Operator | Supplies the policy bundle on agent-host and the tools into `.tools/` |

Workspace layout is as in USS-005 and USS-008, including the per-execution folder `.output/exec/<execId>/`. Credentials are in `/home/agent/.claude` (host bind; `.credentials.json`). git is not in the agent container (USC-001); version control is on agent-host and out of scope.

## Goal

As an agent (interactive session or agent-graph cognitive step), I want a fixed, process-independent base toolset for code engineering (research, read, edit, file management and execution requests) and no free shell, enforced by a policy that neither the agent nor workspace content can change, so that every execution, build, install and tool run goes through an ephemeral container (USS-005), engineering tools such as ngspice run as profile-defined tool runs, and process packages can only add tools from a platform allowlist.

Control claim (precise):

- The base toolset and the Bash guard are a guard on the agent's own actions. They are not the containment boundary.
- The agent user is intentionally in the in-container `docker` group (SSR-AGC-011), so any process of uid 1001 can reach the engine socket. What the engine will do is limited by the authorization plugin of USS-005 (L4), independent of this story.
- The agent model can start a process only through Bash, and Bash is allowed only as a single simple command that `agent-tool-guard` matches against a command allowlist. Containers are therefore started only through `run_ephemeral_container` with operator-supplied profiles.
- Policy (settings, hook, rules) is read only from a read-only bind mount that uid 1001 cannot change, and never from the workspace.
- If the guard is bypassed (a hook defect, or Claude Code not enforcing it), the agent has a free shell as uid 1001. It can then change the workspace and read `/home/agent/.claude/.credentials.json`, but it cannot change the policy (read-only mount), cannot start a more powerful container (USS-005 authorization plugin), and stays contained by gVisor (USS-002) and the host network controls (USS-004).
- Claude Code permission rules and hooks are a policy layer. Their exact semantics, including failure behaviour, depend on the pinned Claude Code version (open item; see "Fail-closed caveat").
- A host operator's own `docker exec` shell is outside this story.

## Base toolset

| Operation | Tool | Where it runs | Notes |
|---|---|---|---|
| Research | `Read`, `Glob`, `Grep` | Claude Code (built-in) | `Grep` uses Claude Code's bundled ripgrep; no shell |
| Edit | `Edit`, `Write` | Claude Code (built-in) | Limited to the write zones |
| Planning | `TodoWrite` | Claude Code (built-in) | No side effects |
| Commands | `Bash`, guarded | Claude Code, after `agent-tool-guard` | Single simple command from the allowlist only (see "Command control") |
| Excluded | `WebFetch`, `WebSearch`, `Task` and subagents, `NotebookEdit`, every MCP tool | | `Skill`: only for runtime-generated skills (open item). MCP-based tools are a separate later story |

Rationale:

- The built-in tools cover research and editing without a shell.
- Prefix-filtering a free shell is unreliable: common commands can start other programs (`find -exec`, `xargs`, GNU `sed` `e`, `awk system()`, `git -c`). A strict grammar plus an exact command allowlist with argument rules, checked in a hook, is tractable.
- The agent image contains interpreters (Python for agent-graph, the Claude Code runtime). They are not on the allowlist, so the agent cannot start them through Bash.
- No toolchains (compilers, ngspice) are in the agent image. They exist only in exec images.

## Command control

Bash is allowed only as a single simple command, checked by the managed PreToolUse hook `agent-tool-guard` before it runs.

Grammar (fail closed: anything the parser does not understand is denied):

- no shell metacharacters or operators anywhere, also inside quotes: `; & | < > $ \` ( ) { }`, backslash `\`, newline, `&&`, `||`;
- no environment assignment prefix (`A=b cmd`), no `cd`;
- quoting: single quotes `'...'` and double quotes `"..."` group a word. Because `$`, backtick and backslash are denied everywhere, both forms are literal and equivalent. An unterminated quote is denied;
- no expansion characters outside quotes: the glob characters `*`, `?`, `[`, `]`, and `~`, `!`, `#`. Inside quotes they are literal, so a pattern argument such as `find src -name '*.cir'` or `rg 'R[0-9]+' src` is allowed. The guard expands nothing;
- a path argument (one an argument rule checks against a zone) contains none of `* ? [ ]`, quoted or not, so the checked path is exactly the path the command opens;
- `argv[0]` is a bare name from the allowlist, resolved by the guard to an absolute path in a root-owned directory (no PATH lookup by the caller).

Command allowlist (illustrative, the final list is an open item), each entry with argument rules in `tool-guard/rules.yml`:

| Group | Commands | Argument rules |
|---|---|---|
| Research | `ls`, `head`, `tail`, `wc`, `diff` | Path arguments in a readable zone |
| Research | `rg` | Deny `--pre`, `--pre-glob` |
| Research | `find` | Deny `-exec`, `-execdir`, `-ok`, `-okdir`, `-delete`, `-fprint*`, `-fls` |
| File management | `mkdir`, `mv`, `cp`, `touch` | Every path argument in a write zone |
| File management | `rm` | Write zones only; recursive only under `src/` |
| Execution | `run_ephemeral_container` | All subcommands: `run`, `build`, `install`, `tool`, `list`, `sweep`. `sweep --label <k>=<v>` may also remove executions the agent does not own (e.g. an isolated step agent-graph runs in parallel); accepted, it gives no extra power (USS-005 authorization plugin). Profiles restricted to `EXEC_PROFILES` in agent-graph cognitive steps (see "Process-specific tools") |

Not allowed (examples): `docker`, `sh`, `bash`, `python`, `sed`, `awk`, `xargs`, `env`, `ngspice` (not in the agent image anyway), `curl`.

The guard checks the argument vector after quote removal. With no unquoted expansion character left, the shell builds the same vector, so what the guard checked is what runs. A pattern with `|` (alternation) is denied even quoted; the agent uses `Grep` or repeated `rg -e` instead.

Path arguments are checked against the zones below: the guard resolves the real path, and a symlink leaving the zone is rejected. The built-in `Edit`, `Write`, `Read`, `Glob` and `Grep` are checked by the same hook on their path input.

### Write and read zones

Paths relative to `/workspace`.

| Path | Agent write | Agent read | Written by |
|---|---|---|---|
| `src/**` | yes | yes | Edit, Write, `mkdir`/`mv`/`cp`/`rm`/`touch` |
| `.agent/<run>/work/<node>/` of the running cognitive step | yes | yes | Edit, Write; granted per turn by agent-graph |
| `build/`, env dirs (`.venv`, `.npm-env`, `.conda-env`) | no | yes | Only `run_ephemeral_container build` / `install` |
| `.input/` | no | agent-graph: only the bound files (InputReadGuard); interactive: yes | Developer, host |
| `.process/` (process YAML, `.process/scripts/`) | no | yes | Developer |
| `.tools/` (`<tool>/{profile.yml,image.tar,tool.json}`) | no | yes | Operator, on the host |
| `.agent/` except the step's work dir (run state, `nodes/`, `checkpoint.sqlite`, `cache/`) | no | no | agent-graph runtime |
| `.output/` (the agent's own outputs, in any folder that is not `exec/` or a run folder, e.g. `.output/notes/`) | yes | yes | Agent (Edit, Write, `mkdir`/`mv`/`cp`/`rm`/`touch`) |
| `.output/exec/<execId>/` except `meta.json` (`result.json`, `stdout.log`, `stderr.log`, `in/`, `out/`) | yes | yes | Exec tool; agent |
| `.output/exec/<execId>/meta.json` | no | yes | Exec tool only |
| `.output/<run>/` (run folder, see below) | no | yes | agent-graph |
| Any `.claude/` dir in the workspace, `.git`, `identity/` | no | yes (nothing sensitive in them) | Developer, host |
| `/etc/claude-code/` | no (read-only mount) | yes | Operator, on the host |
| `/home/agent/.claude/.credentials.json` | no | no | Host |
| Anything else outside `/workspace` except `/tmp` | no | not restricted by this story | |

The exec folder model is that of USS-005: the kind (compiled into the exec tool) and the operator-supplied profile define every mount within that kind, and arguments never change visibility. `stdout.log` and `stderr.log` are never mounted into an execution container.

`.output/` is a write zone for the agent, so it can store its own outputs there, with two protected parts: the run folders of agent-graph and the `meta.json` of every execution. The guard enforces them on the real path of every write (Edit, Write and the path arguments of `mkdir`, `mv`, `cp`, `rm`, `touch`):

- Run folder: a direct child `.output/<name>/` (other than `exec/`) is a run folder when `.agent/<name>/` exists. The guard denies every write into it and its creation, renaming and removal. agent-graph never reuses a folder: S2 creates `.agent/<runID>/` and `.output/<runID>/` exclusively and takes a suffix `-<n>` if either exists (USS-008 "Invocation", USC-008 step 3 and err-19, SFR-AGG-003), so a folder the agent made cannot become a run folder.
- `meta.json`: a file `meta.json` directly in `.output/exec/<execId>/` is denied to every write, including creating one in a folder the agent made. Managed deny rules cover it as well (see the managed settings below).
- Structure: `mv` and `rm` of `.output`, `.output/exec`, an exec dir `.output/exec/<execId>` or a run folder are denied, because they would move or delete a `meta.json` or run outputs; `cp` and `mv` onto a protected path are denied.

Consequences: `meta.json` (request, inputs with sha256, inspected settings, `image_id`) stays as the exec tool wrote it. `result.json` and the logs are writable by the agent, so they are authoritative only as the exec tool returns them (exit code, stream) and as long as nothing else wrote them. A folder the agent creates under `.output/exec/` blocks that exec ID (`--exec-id` must not exist, usage error 125). Symlinks the agent places in `.output/` do not widen an execution: the exec tool resolves paths beneath `/workspace` and the authorization plugin rejects mount sources with a symlink component (USS-005). A cognitive step still writes only what agent-graph grants per turn (its work dir); a step's final products reach `.output/<runID>/` through agent-graph (USS-008 `output.target`).

## Tool runs (kind `tool`)

The kind `tool` runs a fixed command line of an engineering tool (for example ngspice) in an ephemeral container, on declared inputs, with typed parameters. The exec dir, the fixed container settings, profile supply and trust, and the engine authorization policy are those of USS-005; the tool-specific parts are defined here.

The agent invokes it through Bash, so it passes the guard like any command:

```
run_ephemeral_container tool <profile> [--exec-id <name>] --in <name>=<path>... [--param <k>=<v>]... [--label <k>=<v>]... [--timeout <s>]
```

Example:

```
run_ephemeral_container tool ngspice --in netlist=/workspace/src/tb_amp.cir --in include=/workspace/src/models/opamp.lib
```

Products are in `/workspace/.output/exec/<execId>/out/`, the result in `result.json`; the agent reads them with `Read`. The tool prints `run_ephemeral_container: exec_dir=<path>` as its first stderr line.

| Argument | Meaning |
|---|---|
| `<profile>` | Profile name; its `kind` must be `tool`, else usage error 125 |
| `--in <name>=<path>` | Declared input, as in USS-005: the path must be under `/workspace`, readable by the agent user, and match the profile's `inputs` for `<name>`. A path under `src/` (or the mounted `build/`) is used in place; any other path is copied into the exec dir (USS-005 staging: `in/<name>` for a single input, `in/<name>/<basename>` for `multiple`; total cap 1 GiB, provisional) |
| `--param <k>=<v>` | Typed parameter, checked against the `params` of the profile (types and the `usage` error for a bad value are in USS-005) |

The agent supplies only the profile name, inputs (by declared name) and typed params. It cannot set argv, image, mounts or flags.

### Tool profile fields

Common profile fields, and the definitions of the tool-kind additions below (`entry: fixed`, the argv placeholders, `input_policy`, `params`, cause `policy`), are in USS-005. For the kind `tool`:

| Field | Meaning |
|---|---|
| `entry` | `fixed`: `argv` is the whole command line, nothing appended. Differs from `fixed-args`, where caller args are appended |
| `argv` | Fixed command line with the USS-005 placeholders `{out}`, `{work}`, `{in.<name>}`, `{param.<k>}` |
| `inputs` | Map `name` to the allowed path (`path_under`) and `glob` (a single string); `multiple: true` allows the name to be repeated with `--in`, `optional: true` allows it to be absent |
| `input_policy` | Pre-start filter on the `--in` source files, e.g. `spice-safe` |
| `params` | Typed parameters (USS-005) |

A tool's safety depends on its fixed command line (for example ngspice `-b -n`, the input filter, the output paths), which the profile fixes once. The mount model of the kind `tool` is compiled into the exec tool:

| Path in container | tool |
|---|---|
| `/workspace/src` | ro |
| `/workspace/build` | ro if the profile declares it |
| `/workspace/.output/exec/<execId>/in` | ro |
| `/workspace/.output/exec/<execId>/out` | rw |
| `/work` | tmpfs, size-limited, noexec |
| `/tmp`, `/home/run` (HOME) | tmpfs; `/home/run` noexec, `/tmp` noexec unless `tmp_exec: true` |
| Env dirs | not mounted |

Network is none, fixed per kind (`internal/kind/kind.go`), not a profile field. Never mounted: `.input/`, `.process/`, `.tools/`, `.agent/`, other exec dirs, `.git`, `/etc/claude-code`, credentials, the engine socket (USS-005).

Example profile:

```yaml
name: ngspice
kind: tool
image_id: sha256:<64 hex>
entry: fixed
argv: [ngspice, -b, -n, -r, "{out}/sim.raw", -o, "{out}/sim.log", "{in.netlist}"]
inputs:
  netlist: { path_under: [.input/, src/], glob: "*.cir" }
  include: { path_under: [.input/, src/], glob: "*.lib", multiple: true, optional: true }   # in place if under src/, else copied into in/include/<basename>
input_policy: spice-safe          # .include/.lib must resolve to a declared include input
limits: { mem: TBD, cpus: TBD, pids: TBD, timeout_s: TBD, out_max: TBD }
```

Notes (ngspice):

- A netlist is code. `.control` can run `shell` and `source`, `.include` and `.lib` read any file, and `.spiceinit` is auto-executed. So the profile always uses `-b -n`, the `spice-safe` filter, and containment.
- The tool kind sees only `src/` and its inputs, so every file a netlist includes must be passed as an `include` input (`--in include=<path>`, repeatable; in place if under `src/`, else copied). `spice-safe` rejects an `.include`/`.lib` that does not resolve to one of them.
- Open: a relative `.include models/x.lib` resolves against ngspice's cwd (`/work`, USS-005), not the netlist's directory. The spec must say how includes resolve (for example, the policy requires paths relative to the netlist's directory and the tool runs with cwd set to the netlist's staged directory, or similar). Not decided here.

The exec tool runs `input_policy` on the `--in` source files after USS-005 Interaction step 2 and before step 3 (exec dir creation).

Cause `policy` (exit 125): `input_policy` rejected an input; nothing is created. The reason is in stderr. It joins the USS-005 exit codes and causes table as:

| Exit | Cause | Meaning |
|---|---|---|
| 125 | `policy` | `input_policy` rejected an input (tool kind) |

`list --json` (USS-005) reports for tool profiles, besides the common fields: the input names with glob, multiple and optional flags, and the param schema, so the agent (or the agent-graph static check, USS-008) can call a tool correctly. The agent calls it as `run_ephemeral_container list --json` through Bash.

## Policy placement and bind structure

Requirement: no hook, no permission rule and no tool restriction is read from a location writable by uid 1001.

Policy bundle: `/etc/claude-code/` in the agent container is a read-only bind mount of the agent's own root-owned host folder `/var/lib/agent-sandbox/<agentID>/claude-code/`, separate from the workspace and from `/home/agent/.claude`. `create_agent_container` writes it at every creation from the host policy source `/usr/local/share/agent-sandbox/claude-code/` (installed with agent-sandbox, USC-001 follow-up) and replaces an existing folder; the final verification checks owner, modes and that uid 1001 cannot write it (USC-002 steps 3, 4 and 9).

| Path | Content |
|---|---|
| `/etc/claude-code/managed-settings.json` | Deny rules, hook declaration, `disableBypassPermissionsMode`, `allowManagedHooksOnly`, no MCP servers allowed, managed `env` (e.g. `DISABLE_AUTOUPDATER=1`) |
| `/etc/claude-code/hooks/agent-tool-guard` | The hook (static Go binary) |
| `/etc/claude-code/tool-guard/rules.yml` | Command allowlist, argument rules, zones |

Mounting the whole directory also covers any managed drop-in folder the pinned Claude Code version reads (to verify).

Why a read-only bind and not files in the image:

- The policy can be updated on the host (new policy source) without an image rebuild. Because the folder is per agent and written at creation, a running agent keeps the policy it was created with; an update takes effect when the container is recreated. Per-agent folders also allow a per-agent policy later.
- A read-only bind is enforced outside the sandbox (gofer or host side), so not even uid 0 inside the agent container can change it (to verify under gVisor). Image files could be changed by in-sandbox root. With the engine authorization plugin (USS-005) the agent has no route to in-sandbox root anyway: two independent locks.

Other locations:

- `/home/agent/.claude` stays a read-write bind (credentials, sessions, `projects/`). Its `settings.json`, `CLAUDE.md`, `skills/`, `agents/` and `commands/` MAY get nested read-only binds for content integrity, but the policy does not depend on them.
- Workspace `.claude/` (project `CLAUDE.md`, skills, agents) in an interactive session is project content the developer and the agent maintain. agent-graph runs refuse a `.claude/` in the workspace root or in `.agent/` at run start (`packages/agent-graph/src/agent_graph/services/preflight.py:39-44`). Either way hooks and tool limitation never come from it: `allowManagedHooksOnly` ignores its hooks, and managed deny rules win over its permissions (the guard zone and managed deny also block writes to it). The runtime-written `.agent/<run>/active/.claude/` (skills) is not subject to that preflight check and has the same status. So a workspace `.claude/` cannot weaken the policy.
- Ephemeral containers never see `/etc/claude-code`: the engine authorization plugin denies every bind source outside its fixed list (`packages/agent-exec/internal/authz/policy.go:516-519`).
- No ephemeral-authz plugin change is needed for the kind `tool`: its bind sources `src` (ro), `build`, `in` (ro), `out` (rw), the tmpfs mounts and the settings already pass the plugin (`internal/authz/policy.go:195-323, 396-409`).

Conditions:

- Claude Code is root-owned, started by absolute path `/usr/local/bin/claude` (by `enter_agent` and by agent-graph), auto-updater disabled (`DISABLE_AUTOUPDATER` in the managed `env`). Implementation note: the code still uses bare `claude` (`packages/agent-graph/src/agent_graph/config.py:14`, `packages/agent-sandbox/internal/sequence/enter.go:116`).
- The hook binary has no dependency on a writable path: absolute path, static, no PATH lookup, reads rules only from `/etc/claude-code/tool-guard/`.

Mount change: the policy bundle is a fourth mount on the agent container (USC-002 and SSR-AGC-004 updated). Follow-ups: SFR-AGC-006, SFR-AGC-009, USS-002, `interaction_points.yml`, and the agent-sandbox code (creation, `s08_verify` "exactly three mounts").

Illustrative managed settings. Key names and the path-rule syntax (absolute paths as `//workspace/...`) are to verify against the pinned Claude Code version. Bash is not denied: it is guarded by the hook. `.output/` is an agent write zone; only `meta.json` has a deny rule. Run folders have names known only at run time, so they are protected by the guard alone.

```json
{
  "permissions": {
    "deny": [
      "WebFetch", "WebSearch", "Task", "NotebookEdit",
      "Edit(//workspace/.process/**)", "Write(//workspace/.process/**)",
      "Edit(//workspace/.input/**)",   "Write(//workspace/.input/**)",
      "Edit(//workspace/.tools/**)",   "Write(//workspace/.tools/**)",
      "Edit(//workspace/build/**)",    "Write(//workspace/build/**)",
      "Edit(//workspace/.output/exec/*/meta.json)", "Write(//workspace/.output/exec/*/meta.json)",
      "Edit(//workspace/**/.claude/**)", "Write(//workspace/**/.claude/**)",
      "Edit(//etc/claude-code/**)",    "Write(//etc/claude-code/**)",
      "Read(//home/agent/.claude/.credentials.json)",
      "mcp__*"
    ],
    "disableBypassPermissionsMode": "disable"
  },
  "allowManagedHooksOnly": true,
  "env": { "DISABLE_AUTOUPDATER": "1" },
  "hooks": {
    "PreToolUse": [
      { "matcher": "Bash|Read|Edit|Write|Glob|Grep",
        "hooks": [ { "type": "command", "command": "/etc/claude-code/hooks/agent-tool-guard", "timeout": 10 } ] }
    ]
  }
}
```

No MCP server is allowed (no managed MCP config, no `mcp__*` tool; the exact way to forbid them, e.g. an empty managed server allowlist, is to verify). `allowManagedPermissionRulesOnly` is not set: agent-graph narrows per turn with CLI allow rules (for example `Write(<work>/**)`), and that setting would drop them.

### Fail-closed caveat

Claude Code blocks a tool call only on hook exit 2 or an explicit deny decision; a hook crash, a non-2 non-zero exit or a timeout lets the tool run (to verify against the pinned version). Therefore:

- `agent-tool-guard` must catch every error and exit 2 (deny), including parse errors and unreadable rules;
- it must be fast, and the hook `timeout` is set well above its runtime;
- the managed deny rules stay as a second policy layer;
- a hook that is bypassed leads to the case in the Control claim (free shell as uid 1001, policy and engine still protected).

## Enforcement layers

| Layer | Mechanism | Covers | Strength |
|---|---|---|---|
| L1 Tool surface | agent-graph `--tools` lists the process-level `runtime.tools` (`ports.py:61,75`, `engine/e1_load.py:42`); a step narrows only through `--allowedTools` (`services/claude_cli.py:85-86`). Managed `permissions.deny` removes the excluded tools in both the interactive session and agent-graph | Which tools exist | Weak alone: a CLI flag can widen it; L2 closes that |
| L2 Managed settings | `/etc/claude-code/managed-settings.json` in the read-only mount. Highest precedence: CLI flags, project and user settings cannot override it; deny rules always win | Tool deny, path deny, hook registration, no MCP | Strong, if the pinned version behaves as documented (verify) |
| L3 `agent-tool-guard` hook | Managed PreToolUse hook: Bash grammar and command allowlist, argument rules, path zones with real-path resolution | Which commands run, on which paths | Independent of rule glob semantics; fails open if the hook fails and Claude Code does not deny (see caveat) |
| L4 Engine authorization plugin | `ephemeral-authz` (USS-005): default-deny API policy and create invariants for every client of the engine socket | What any process of uid 1001 can make the engine do | Hard: holds even when L1 to L3 are bypassed |
| L5 Platform | gVisor (USS-002), host network controls (USS-004) | What a fully bypassed agent can reach | Hard boundary outside the agent's reach |
| L6 Detection | agent-graph hashes the protected files (`.process/`, `.input/`, `.tools/`, run state) before a cognitive turn and checks them after; a change fails the step. The exec tool records every execution in `meta.json` | A change that got through | Detective, not preventive |

Open: verify against the pinned Claude Code version that `--allowedTools` actually restricts the tool set rather than only auto-approving. If it does not, agent-graph must build a per-step `--tools` in code. L6 is not implemented: the runfs `WriteGuard` does not protect `.tools/`.

## Process-specific tools

- A process's `runtime.tools` and step tools must be a subset of the platform allowlist: the base toolset, plus Bash only where listed. `validate.py` rejects any other tool. The platform allowlist file now lives in the policy bundle (e.g. under `/etc/claude-code/tool-guard/` or a sibling read-only path; `TBD`), root-owned, not changeable by the developer. The platform allowlist for the guard commands is the same bundle.
- Bash in a cognitive step is available only if the process lists it; the command allowlist of the guard still applies. The default `runtime.tools` stays `Read, Write` (default policy for Bash is an open item).
- Profiles are operator-supplied in `.tools/`, bounded by the kind model compiled into the exec tool (USS-005 "Profile trust"). A process package cannot add a profile or an image.
- A process declares the profiles it uses in `runtime.profiles` (list of names). A procedure step references one with `profile:`. agent-graph passes them to the guard as `EXEC_PROFILES`; in a cognitive step `agent-tool-guard` accepts only those profiles in `run_ephemeral_container` calls. In the interactive session any profile in `.tools/` is accepted.
- The guard fails closed on the session mode. agent-graph also sets an explicit `AGENT_SESSION=cognitive` (the interactive session sets `AGENT_SESSION=interactive`). The variable is required: if `AGENT_SESSION` is absent or has another value, or `EXEC_PROFILES` is unset in `cognitive` mode, the guard denies every `run_ephemeral_container` call instead of allowing all profiles. Transport for agent-graph is `TurnRequest.env_extra` (`ports.py:99`). Open: verify that Claude Code passes the environment to hooks, and how `enter_agent` sets `AGENT_SESSION`.
- agent-graph checks before the run that every required profile is present and valid (USS-008 static check, via `list --json`). A missing tool is a pre-run error, not a run-time failure.
- An isolated procedure step may name a tool profile (see "Execution model"). Today agent-graph does not support that, so it must change in two places:
  - `process/validate.py:257-259` requires kind `code` for an isolated step; it must accept `tool` as well;
  - `services/container.py:170` (`ContainerExec.run_isolated`) always calls `run`; for a tool profile it must call the `tool` subcommand.
- A non-isolated procedure step cannot run a process tool.
- MCP-based tools are a separate later story; no MCP tool can be listed.

## Reference operation (ngspice)

Interactive session, `src/amp.cir` and `src/models/opamp.lib` exist.

1. Research: `Glob src/**/*.cir`, `Grep ".subckt|.model" src/`, `Read src/amp.cir`, `Read src/models/opamp.lib`.
2. Edit: `Edit src/amp.cir` (change R2); `Write src/tb_amp.cir` (testbench with `.tran` and `.include models/opamp.lib`).
3. `Bash: ngspice -b src/tb_amp.cir` is denied by `agent-tool-guard` (not in the allowlist). ngspice is not installed in the agent image anyway.
4. First tool run, without the include:

   ```
   run_ephemeral_container tool ngspice --exec-id e7 --in netlist=/workspace/src/tb_amp.cir
   ```

   `spice-safe` finds `.include models/opamp.lib` that does not resolve to a declared `include` input: exit 125, cause `policy`, reason in stderr; nothing is created.
5. Corrected call:

   ```
   run_ephemeral_container tool ngspice --exec-id e7 \
     --in netlist=/workspace/src/tb_amp.cir --in include=/workspace/src/models/opamp.lib
   ```

   Both inputs are under `src/`, so they are used in place (no copy). `spice-safe` passes, then the container runs `ngspice -b -n -r {out}/sim.raw -o {out}/sim.log {in.netlist}`. `result.json` has `exit` 0, cause `ok`.
6. `Read /workspace/.output/exec/e7/out/sim.log` (text). `sim.raw` is binary, so the agent writes `src/analyse.py` (Edit, Write) and calls:

   ```
   run_ephemeral_container run python --exec-id e8 \
     --in data=/workspace/.output/exec/e7/out/sim.raw -- python /workspace/src/analyse.py /workspace/.output/exec/e8/in/data
   ```

   `sim.raw` is outside `src/`, so it is copied into `in/data`. The gain and bandwidth printed by the script are in `stdout.log`, read with `Read`. (Whether the script needs packages from `.venv` is `TBD`.)
7. Negatives, each denied and changing nothing:
   - `Bash: cat x | sh` denied (grammar);
   - `Bash: docker run --privileged -v /:/host ...` denied (not in the allowlist); if the hook were bypassed, the authorization plugin of USS-005 denies the create;
   - `Write /workspace/.claude/settings.json` denied (managed deny, guard zone);
   - `Write /etc/claude-code/managed-settings.json` fails (read-only mount);
   - `Bash: rm -r .process` denied (zone);
   - a netlist with `.control` and `shell`: exit 125, cause `policy`; the agent removes the block and retries.
8. Same in agent-graph: a cognitive step with `tools: [Read, Glob, Grep, Edit, Write, Bash]` and `runtime.profiles: [ngspice, python]`. The products of an exec are in its `out/`; agent-graph copies them into the step work dir if needed (USS-005 open item). Alternatively the simulation is an isolated procedure step with `profile: ngspice` (the `ngspice` profile of this story, not the USS-005 example, which uses `profile: python`); it needs the two agent-graph changes in "Process-specific tools".

## Implementation status

What does not exist yet:

- Kind `tool` in agent-exec: `cli.go:72-73` reserves the subcommand and `--param` is rejected; `profile.go:183-184` rejects kind `tool`; `sequence.go` (`buildArgv`, mounts, locks) has no tool case; `kind.go` `Mounts` has no tool case, so `in/` would not be mounted; `app.go` `list --json` lacks inputs and params.
- agent-graph: `runtime.profiles` (schema `process_v1.schema.yml`, `loader.py`, `state.py` `RuntimeCfg`), `EXEC_PROFILES`, `AGENT_SESSION`, kind `tool` for isolated steps.
- The policy bundle (`managed-settings.json`, rules, hook source) and its copy step in agent-sandbox.
- The agent-sandbox fourth mount: `internal/services/docker.go` (`CreateArgs`) and `internal/steps/s08_verify.go` still expect three mounts.

## Build and delivery

Through USS-003 `Dockerfile.agent`:

- the agent image holds Claude Code (version pinned, root-owned, `/usr/local/bin/claude`) and `run_ephemeral_container` (USS-005);
- the policy bundle is not in the image: it is the read-only bind of the root-owned host folder (see "Policy placement and bind structure"), prepared by the operator on agent-host and mounted by agent creation (USC-002);
- the `agent-tool-guard` source and build location is `TBD`; it is a static Go binary delivered in the bundle.

No exec images or profiles are in the agent image; they come from the tool pool into `.tools/` (USS-005 "Profile store and supply"). Not implemented.

## Preconditions

- USS-002 is complete, with the policy bundle mounted read-only at `/etc/claude-code/`.
- The in-container engine and its authorization plugin (USS-005) are running; the agent user is in the `docker` group (SSR-AGC-011).
- The bundle files are `root:root`, not writable by uid 1001, and the hook binary is present and static.
- `run_ephemeral_container` is installed (USS-005); for tool runs the profile and image tarball are in `.tools/`.
- For agent-graph: the process is validated against the platform allowlist.

## Interaction

One tool call:

1. `claude` resolves the tool from the model's request.
2. Managed settings decide: deny, or allow (also checked against any CLI allow rules).
3. The managed PreToolUse hook `agent-tool-guard` checks the call: for Bash the grammar, the allowlist and the argument rules; for the built-ins the paths. Any error is a deny (exit 2).
4. The tool runs: a built-in, or the allowlisted command. `run_ephemeral_container` talks to the in-container engine as the agent user; the engine authorization plugin applies (USS-005).
5. The result (or the tool error) goes to the model.

agent-graph only: after the cognitive turn, the runtime checks the protected-file hashes (L6).

## Acceptance criteria

- The excluded tools (`WebFetch`, `WebSearch`, `Task`, `NotebookEdit`, any MCP tool) are not callable in the interactive session or in a cognitive step, also when a CLI flag or a workspace `.claude/settings.json` tries to enable them.
- A Bash command with a metacharacter or operator, an env prefix, `cd`, or an `argv[0]` not on the allowlist is denied; allowlisted commands with a forbidden argument (`rg --pre`, `find -exec`) are denied.
- No session has a way to start an arbitrary process or open a unix socket: only the allowlisted commands run.
- `Edit`, `Write` and write-type commands to each protected zone fail, including `/etc/claude-code/`.
- Reading the credentials file fails.
- The agent can create and change files in its own `.output/` folders (e.g. `.output/notes/`), but every write, `mv` or `rm` touching a run folder `.output/<run>/` or an `.output/exec/<execId>/meta.json` is denied.
- A symlink in `src/` pointing to `.process/` cannot be written through.
- A hook in a workspace `.claude/settings.json` does not run; its permissions do not widen the managed policy.
- No MCP server starts.
- `agent-tool-guard` errors (bad input, unreadable rules) deny the call.
- A process listing an unlisted tool fails validation.
- Tool run: the profile's fixed argv is run and the caller cannot change it; products are present in `out/`.
- A netlist with `.control` and `shell` is rejected with exit 125 and cause `policy`; no exec dir and no container are created.
- The ngspice reference operation completes with `sim.raw` and `sim.log` in `.output/exec/e7/out/`; the first call without `include` returns 125 with cause `policy`.
- `list --json` returns kind, input names (glob, multiple, optional) and param schema per tool profile.
- `/etc/claude-code` is a read-only mount; its files are `root:root`; `test -w` as uid 1001 fails on each.
- A change of a protected file during a cognitive turn fails the step.

## Failure cases

| Case | Behaviour |
|---|---|
| Tool denied by managed settings | Tool error to the model; the session continues |
| Bash command rejected by `agent-tool-guard` (grammar, allowlist, argument rule, path zone) | Hook exit 2; tool error with the reason; nothing is run |
| `agent-tool-guard` internal error or unreadable rules | Deny (exit 2) |
| `agent-tool-guard` crashes, is killed or times out | Claude Code may let the tool run (to verify); managed deny rules and L4 remain; see "Fail-closed caveat" |
| Exec error (usage, setup, policy, verify, denied, busy, timeout, cancel, oom, out_cap) | Passes through unchanged with the USS-005 exit code and cause |
| Policy bundle missing, not read-only, or writable by uid 1001 | agent-sandbox final verification fails (new check, open item) |
| Integrity check failure (protected file changed during a turn) | The step fails and the run stops; agent-graph policy `TBD` |
| Guard bypassed (L1 to L3 failed) | Free shell as uid 1001; policy unchanged (read-only mount); engine limited by the authorization plugin (USS-005); contained by gVisor (USS-002) and host network controls (USS-004) |

## Open items

- Verify managed-settings key names, precedence, path-rule syntax, drop-in folders and how to forbid all MCP servers, with the pinned Claude Code version.
- Verify hook failure semantics (crash, non-2 exit, timeout) and `allowManagedHooksOnly` behaviour with the pinned version.
- Verify that the read-only bind of `/etc/claude-code` holds against in-sandbox uid 0 under gVisor.
- Final command allowlist and argument rules.
- Verify that the Claude Code Bash tool passes the command string to the shell unchanged (no wrapping that adds expansion), so the guard's argument vector equals the executed one.
- Symlink handling of the built-in tools (`Edit`, `Write`, `Read`).
- `Skill` tool and the agent-graph runtime skills (`compose/skills.py:59` writes `active/.claude/skills`).
- Whether cognitive steps get Bash by default.
- Location and layout of the policy bundle on agent-host; the platform allowlist file location (`TBD`).
- Verify against the pinned Claude Code version that `--allowedTools` restricts the tool set rather than only auto-approving. If it does not, build a per-step `--tools` in code.
- `spice-safe`: how a relative `.include`/`.lib` resolves (ngspice cwd is `/work`, not the netlist's directory); see the ngspice notes.
- How a netlist parameter such as `tstop` is passed to ngspice (the reference profile has no param).
- `AGENT_SESSION` and `EXEC_PROFILES`: verify that Claude Code passes the environment to hooks.
- Run label: containers started from a cognitive-step Bash carry no `agent-graph.run` label, so the run-start sweep cannot find them. Option: a run-id environment variable that `run_ephemeral_container` turns into a label.
- Whether the guard restricts free `run -- <argv>` in cognitive steps (USS-005 leaves the argv of kind `code` open).
- Concurrent Bash tool calls against slot accounting (USS-005 race item).
- L6 integrity check is not implemented (runfs `WriteGuard` does not protect `.tools/`).
- agent-graph changes: allowlist in the bundle, `runtime.profiles` in the process schema, `EXEC_PROFILES` and `AGENT_SESSION` environment, kind `tool` for isolated steps (`validate.py`) and the `tool` subcommand in `ContainerExec`, static check of required profiles against `list --json`, integrity check (including `.tools/`), start `claude` by absolute path, drop `--setting-sources project` or rely on `allowManagedHooksOnly`, copy exec products from `out/` into the step work dir.
- Mount change follow-ups: USC-002 creation, SSR-AGC-004, agent-sandbox final verification ("exactly three mounts" becomes four).
- agent-sandbox final verification (follow-up): `/etc/claude-code` is a read-only mount; files `root:root` with expected modes; `test -w` as uid 1001 fails on each; the policy version on the host matches.
- `.output/exec/` cleanup and retention (`sweep --label`, USS-005).
- MCP-based tools: a separate later user story.
- Derived requirements: SFR-AGT-001 to SFR-AGT-017 (013 and 016 moved to SFR-AGG-042 and SFR-AGG-043) and SSR-AGT-001 to SSR-AGT-007 (`design/03_sys_req/agent-toolset/` and `agent-graph/`, from USC-009); to review.
- How a human operator gets a shell: host-side, out of scope.
- git and version control: host-side, out of scope.
