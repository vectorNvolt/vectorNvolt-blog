# agent-exec

Three Go binaries of USS-005 / USC-005 (SFR-AGE-001..019, SSR-AGE-001..015), plus the kind `tool` of USS-009 / USC-009 steps 4-6 (SFR-AGT-008..012, SSR-AGT-006):

- `run_ephemeral_container` (`cmd/run_ephemeral_container`): the execution tool. It runs inside the agent container as uid 1001, reads the operator-supplied profiles in `/workspace/.tools/*/profile.yml`, stages inputs, and runs, builds or installs in one throwaway container per execution of the in-container Docker engine (fixed unprivileged settings, only the mounts of the kind). Logs and `result.json` go to `/workspace/.output/exec/<execId>/`.
- `ephemeral-authz` (`cmd/ephemeral-authz`, `internal/authz`): the engine authorization plugin (SSR-AGE-007). Compiled-in default-deny policy; listens on the inherited file descriptor 3 (passed by `agent-init`) or on `--socket <path>`.
- `agent-init` (`cmd/agent-init`, `internal/agentinit`): the init of the agent container, run as root under tini (`ENTRYPOINT tini -- /usr/local/sbin/agent-init`). Binds `/run/docker/plugins/ephemeral-authz.sock` (root, 0600), starts `/usr/local/libexec/ephemeral-authz` as user `authz` with the socket as fd 3, starts `dockerd` with no flags, restarts the plugin (1 s backoff) if it exits, forwards SIGTERM and SIGINT to `dockerd`, and exits with `dockerd`'s exit code when `dockerd` exits (terminating the plugin). Logs prefixed `agent-init:` go to stderr (the container log); the plugin's stderr is inherited, so it lands there too.

Go 1.22, module `agent-exec`. Dependencies: `gopkg.in/yaml.v3` (profile parsing, unknown fields rejected), `golang.org/x/sys` (`openat2`, `flock`). The Engine API is spoken directly over the Unix socket with `net/http` (no docker CLI, no SDK).

## Build

```
CGO_ENABLED=0 go build ./cmd/...
go vet ./... && go test ./...
```

Static Linux binaries for the agent image (collected in `dist/bin` for `create_agent_image`, see [howto](../../howto/README.md) steps 2 and 3):

```
mkdir -p ../../dist/bin
for n in run_ephemeral_container ephemeral-authz agent-init; do
  CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o ../../dist/bin/$n ./cmd/$n
done
```

## Usage

```
run_ephemeral_container run     <profile> [--exec-id <name>] [--in <name>=<path>]... [--label <k>=<v>]... [--timeout <s>] -- <command> [args...]
run_ephemeral_container build   <profile> [--exec-id <name>] [--in <name>=<path>]... [--label <k>=<v>]... [--timeout <s>] [-- <driver args...>]
run_ephemeral_container install <profile> [--exec-id <name>] --manifest <path>... [--label <k>=<v>]... [--timeout <s>]
run_ephemeral_container tool    <profile> [--exec-id <name>] [--in <name>=<path>]... [--param <name>=<value>]... [--label <k>=<v>]... [--timeout <s>]
run_ephemeral_container list    [--json]
run_ephemeral_container sweep   --label <k>=<v>
```

The first stderr line of an execution is `run_ephemeral_container: exec_dir=<path>`; diagnostics are prefixed `run_ephemeral_container:`.

Environment (tests): `AGENT_EXEC_WORKSPACE` (default `/workspace`), `AGENT_EXEC_DOCKER_HOST` (default `unix:///var/run/docker.sock`).

## Kind `tool`

The caller supplies only the profile name, inputs by declared name and typed params; no `--`, argv, image, mount or option. Reference profile (`/workspace/.tools/ngspice/profile.yml`, with `image.tar`, operator-supplied):

```yaml
name: ngspice
kind: tool
image_id: sha256:<64 hex>
entry: fixed
argv: [ngspice, -b, -n, -r, "{out}/sim.raw", -o, "{out}/sim.log", "{in.netlist}"]
inputs:
  netlist: { path_under: [src/, .input/, .output/exec/], glob: "*.cir" }
  include: { path_under: [src/, .input/], glob: "*.lib", multiple: true, optional: true }
input_policy: spice-safe
params: {}            # e.g. steps: { type: int, min: 1, max: 100, default: 10 }
limits: { mem: 512m, cpus: 1, pids: 128, timeout_s: 600, out_max: 256m }
```

```
run_ephemeral_container tool ngspice --exec-id e7 --in netlist=/workspace/src/tb_amp.cir --in include=/workspace/src/models/opamp.lib
```

- Fields: as the other kinds (`name`, `image_id`, `ref`, `inputs`, `env`, `tmp_exec`, `build_dir`, `requires_build`, `limits`), plus `entry: fixed` (required), `argv` (required), `input_policy`, `params`. `env_dirs`, `fetch_argv` and unknown fields (e.g. `mounts`, `network`) are rejected. `input_policy` and `params` are rejected on other kinds.
- Placeholders in `argv` (nothing else is expanded, nothing appended): `{out}` (own `out/`), `{in.<name>}` (as a whole argument one argument per path, none for an absent optional input; inside a longer argument only for a required single input), `{param.<name>}` (canonical value). `argv[0]` is literal; every param must be used in `argv`.
- Params: `type` `int`, `number` (finite float, emitted as Go `%g`), `bool` (`true`/`false`) or `string` (needs `pattern`, matched against the whole value, or `values`); optional `min`/`max` (numbers), `default` (makes it optional). Values are at most 256 bytes without control characters. Params enter the command line only through `{param.<name>}` (passing netlist params such as `tstop` into the netlist is an open item of USS-009).
- Validation (exit 125, cause `usage`, nothing created): profile kind not `tool`, input not declared / outside `path_under` / not matching `glob` / not a regular file / symlink / repeated without `multiple` / duplicate basename, missing non-optional input, unknown, missing (no default) or mistyped param.
- Inputs under `src/` (and `build/` with `build_dir`) are used in place; others are copied to `in/<name>` or `in/<name>/<basename>` (`multiple`), 1 GiB total.
- Mounts (compiled in, `internal/kind`): `src/` ro, `build/` ro only with `build_dir: true`, own `in/` ro, own `out/` rw; tmpfs `/work`, `/tmp` (noexec unless `tmp_exec`), `/home/run`; network `none`; no env dir. `ephemeral-authz` needed no change: these binds, tmpfs and network are already on its allowlist.
- `input_policy: spice-safe` (`internal/inputpolicy`) runs on the source files of every input after validation and before the exec dir: a `.control` block (or a `*ng_script` file) with the word `shell`, `source` or `codemodel` is refused; every `.include`/`.inc*` and file form `.lib <file> <section>` must resolve to the container path of an input named `include` of the same call. Resolution: absolute as is, relative joined to the directory of the including file as the command sees it (in place `/workspace/src/...`, staged `.../in/...`); `..` components, a leading `~` and NUL are refused. Refusal: exit 125, cause `policy`, reason on stderr, nothing created.
- `list --json` adds, for kind `tool` only, `inputs` (`name`, `path_under`, `glob`, `multiple`, `optional`), `params` (schema) and `input_policy`.

## Exit codes and causes

`result.json` (`{exit, cause, exec_dir, image_id, start, end}`, install also `phase`) is authoritative.

| Exit | Cause |
|---|---|
| 0 | `ok` |
| command's own | `exit` |
| manager's own | `fetch` (install, fetch phase failed) |
| 124 | `timeout` |
| 125 | `usage`, `setup`, `denied`, `policy` (kind `tool`), `verify`, `busy` |
| 130 | `cancel` (SIGTERM, SIGINT) |
| 137 | `oom`, `out_cap` |

Errors before the exec dir exists (usage, policy, engine unreachable, invalid profile, missing folder, busy) create nothing and write no `result.json`.

## Layout

- `cmd/run_ephemeral_container`, `cmd/ephemeral-authz`, `cmd/agent-init`: thin `main`s.
- `internal/authz`: plugin policy and handler. `internal/agentinit`: init logic (socket, plugin supervision, `dockerd`).
- `internal/cli`: argument grammar. `internal/app`: wiring, signals, `list`.
- `internal/profile`: profile schema, ceilings, tarball image ID; `tool.go`: kind `tool` params and argv placeholders.
- `internal/inputpolicy`: input policies of kind `tool` (`spice-safe`).
- `internal/kind`: compiled-in kind model (mounts, network, tmpfs, env), create body, inspect verify.
- `internal/wsfs`: workspace access without symlink escape, staging, locks, exec dir, size watch.
- `internal/engine`: Engine API client, `Engine` interface, stdcopy demux.
- `internal/sequence`: steps 2-10, exit and cause contract, slot accounting, sweep.
- `internal/limits`: ceilings shared with `ephemeral-authz` (provisional values).
- `tests/`: full sequence against a fake engine.

## Not implemented

- `--manifest` form for npm and conda (TBD in USS-005): only the pip target `.venv` is supported, other install targets fail with cause `setup`.
- `spice-safe` is a line/word filter, not a SPICE parser: it is conservative (e.g. `echo source` in a `.control` block is refused) and does not inspect XSPICE `file=` model parameters. It is a guard on the command; containment stays with the mount model and `ephemeral-authz`.
- Slot accounting across concurrent tool processes is a read of the engine at start (no central process), so two simultaneous starts can race.
- A signal during `docker load` or input staging is acted on after that step returns.
- Not run against a real engine yet (verification spike: runc inside gVisor), including `agent-init` with a real `dockerd`. Unit and sequence tests (`go test ./...`) use a fake engine.
