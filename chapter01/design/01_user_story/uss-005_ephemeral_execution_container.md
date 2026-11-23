# USS-005 — Run, build and install application code in an ephemeral execution container

USS-005 runs inside an agent container created by USS-002, from the image created by USS-003. It uses the in-container Docker daemon that those stories provide.

Design background: docs/concept_ephemeral_exec.md (concept); this story is authoritative where they differ.

Status: implemented, with fake-engine tests only. The tool `run_ephemeral_container`, the engine authorization plugin `ephemeral-authz` and `agent-init` are implemented in `packages/agent-exec` (unit and sequence tests pass against a fake engine; the `agent-init` tests use a fake starter). Verification of the acceptance criteria against a real `dockerd` is still open. `Dockerfile.agent` installs all three binaries from the build context (`bin/`, which the image builder sends, `packages/agent-image/internal/config/config.go` `BinFiles`) and fails closed if one is missing; it creates the user `authz`, writes the `authorization-plugins` key to `daemon.json` and sets `tini` -> `agent-init` as the entrypoint. The profile schema extensions of "Profile schema extensions (tool kind, USS-009)" are not implemented. The in-container daemon is provided by the image (`tini` as PID 1 -> `agent-init` -> `ephemeral-authz` and `dockerd`, see "In-container daemon configuration"; settings in `/etc/docker/daemon.json`; user `agent` uid/gid 1001, member of the group `docker`, see SSR-AGC-011).

## Participants

| Participant | Role |
|---|---|
| Caller | The agent-graph procedure runner, through `ContainerExec` (`packages/agent-graph/src/agent_graph/services/container.py`), or the agent through a guarded Bash command (USS-009). Runs as the non-root agent user (UID 1001) inside the agent container. Intent: in agent-graph every procedure step runs through the tool; there is no in-agent procedure backend for process steps. Today only steps with `isolate: true` and a `profile` do (`run_isolated`, kind `code`); every other procedure step still runs in the agent sandbox through `SandboxExec` (open item) |
| Execution tool | Go executable `run_ephemeral_container`, part of the agent image, run by the caller as the agent user (uid 1001, member of the in-container group `docker`). It talks to the in-container engine socket directly. It loads the profiles, validates the request, stages inputs, and creates, verifies, starts, waits for and removes one ephemeral container per execution. It streams the command's output to its own stdout and stderr and into the exec dir, and exits with the code of the execution |
| Execution profile | Operator-supplied manifest (`/workspace/.tools/<name>/profile.yml`) that defines one kind of execution: image ID, entry, inputs, limits, within the fixed model of its kind (see "Profile trust"). The caller selects it by name only |
| Tool pool | Folder on agent-host holding one folder per profile version (`profile.yml`, `image.tar`, `tool.json`). The operator copies folders from it into the agent's workspace (see "Profile store and supply") |
| Agent container | gVisor sandbox from USS-002; hosts the in-container Docker daemon |
| In-container Docker daemon | `dockerd` started by `agent-init` as root inside the agent container; creates the ephemeral containers. Its socket is reachable by the group `docker` (SSR-AGC-011). Every API request passes the engine authorization plugin first |
| Engine authorization plugin | `ephemeral-authz`: non-root Go process that allows or denies each engine API request by a compiled-in default-deny policy (see "Engine authorization plugin") |
| `agent-init` | Root Go program started by `tini`; creates and binds the plugin socket, starts the plugin with it, then starts `dockerd`, and supervises the plugin (`packages/agent-exec/internal/agentinit`, `cmd/agent-init`) |
| Ephemeral container | Short-lived container for one execution; removed when it ends |
| Agent workspace | `/workspace` inside the agent container (the per-agent folder from USS-002) |

Workspace layout (paths relative to `/workspace`):

| Path | Content | Visible to an ephemeral container |
|---|---|---|
| `src/` | Source code | Read-only, all kinds (install: manifests) |
| `build/` | Persistent build output folder | Per kind and profile |
| `.venv`, `.npm-env`, `.conda-env` | Dependency environment folders (hidden dot folders), for pip, npm and conda. Created empty by agent-host root (see "Preconditions"); `.venv` is not a virtualenv until `install` fills it | Per kind and profile |
| `.packages/` | Operator-supplied wheels (built or downloaded on agent-host), copied to `Workspace-Agents/<agentID>/.packages/`. Fallback for packages the fetch phase cannot get as a wheel | Read-only, install only (both phases). Never writable by any ephemeral container |
| `.input/` | Input data, put there by the developer or host. agent-graph binds selected files into `.agent/<run>/binding.json`. Read-only by intent | Never mounted. A file passed with `--in` is copied into the execution's `in/` |
| `.process/` | Process definitions and `.process/scripts/` | Never mounted |
| `.tools/` | Operator-supplied execution profiles, one folder per profile: `profile.yml`, `image.tar`, `tool.json` | Never mounted |
| `.agent/` | agent-graph run state, written only by the runtime. Each step's working dir is `.agent/<run>/work/<node>/` | Never mounted. A product passed with `--in` is copied into the execution's `in/` |
| `.output/<run>/` | Final products of a graph run, copied by agent-graph (`stage_outputs`) plus `report.md`; `.output/` is created by USS-002 at top level | Never mounted |
| `.output/exec/<execId>/` | One folder per execution, created by the tool (see "Exec dir layout"). `.output/exec/` is reserved for executions; run IDs never equal `exec` | Only its `in/` (ro; not for install), `out/` (rw) and `dl/` (install only: rw in fetch, ro in install) |
| `.git` | Git metadata | Never mounted |

Outside the workspace, `/etc/claude-code` (the agent container's policy bundle, USS-009) is never mounted into an ephemeral container; the plugin's bind allowlist never allows it.

git is not installed in the agent container.

## Goal

As a caller (the agent-graph procedure runner, or the agent), I want to run application code, build it, or install its dependencies, in a fresh, throwaway container, by calling a single Go executable with a subcommand (`run`, `build` or `install`) and a profile name, so that the container has only the access its kind needs (no network for code, build and the install phase; network only for the download phase of install, which executes no package code; no access to the agent's credentials or tools; only fixed paths writable), the logs and the result are written to `/workspace/.output/exec/<execId>/`, and nothing is left behind when it ends.

Isolation claim (precise):

- The ephemeral container separates the executed code from the agent: the agent's credentials, home directory, network route, tools and workspace (except the fixed mounts of the kind) are not visible to the code.
- The engine authorization plugin limits the agent's own use of the engine: the agent user is in the group `docker` and can reach the engine socket (SSR-AGC-011), but the engine only creates containers that match the fixed container settings of this story. Any container the engine can start runs as uid 1001 without capabilities and without host namespaces, so the group `docker` is no longer root-equivalent (see "Engine authorization plugin" for the claim and its residual risks).
- The exec tool's own validation and its inspect-verify step are not replaced by the plugin; the plugin is the hard backstop that holds even when the agent bypasses the tool.
- Which agent processes can reach the engine is further limited by USS-009 (agent tool usage and command control).
- The boundary to the host is the gVisor sandbox of the agent container (USS-002) and the host-side network controls (USS-004), which are out of the agent's reach.
- The ephemeral container runs under the in-container daemon's default runtime (`runc`) inside gVisor, with the default seccomp profile. It is not a second gVisor sandbox.

## Invocation

```
run_ephemeral_container run     <profile> [--exec-id <name>] [--in <name>=<path>]... [--label <k>=<v>]... [--timeout <s>] -- <command> [args...]
run_ephemeral_container build   <profile> [--exec-id <name>] [--in <name>=<path>]... [--label <k>=<v>]... [--timeout <s>] [-- <driver args...>]
run_ephemeral_container install <profile> [--exec-id <name>] --manifest <path>... [--label <k>=<v>]... [--timeout <s>]
run_ephemeral_container tool    <profile> ...   # reserved, USS-009
run_ephemeral_container list    [--json]
run_ephemeral_container sweep   --label <k>=<v>
```

Run by the caller (agent user) inside the agent container. The tool has no `--image`, mount or flag argument; the profile decides all of that.

Test-only environment overrides: `AGENT_EXEC_WORKSPACE` (production: `/workspace`) and `AGENT_EXEC_DOCKER_HOST` (production: `unix:///var/run/docker.sock`).

Principle: the kind (compiled into the tool) defines every mount and its mode; the operator-supplied profile only selects within it. No argument adds, removes or changes a mount. Arguments only supply data: the command, driver args, manifests, input files to stage, the timeout, labels and an exec ID name.

| Argument | Meaning |
|---|---|
| `<profile>` | Profile name. The subcommand must match the profile's `kind`, else usage error 125 |
| `--exec-id <name>` | Name of the execution folder. Pattern `[a-z0-9][a-z0-9-]{0,63}`; the folder must not exist. If absent, the tool generates `e<base36 unix seconds>-<8 hex>`. Lets agent-graph know the folder in advance. Container name is `ephemeral-<execId>`; for install the fetch phase container is `ephemeral-<execId>-fetch` and the install phase container is `ephemeral-<execId>`. Both carry the label `ephemeral.exec=<execId>` |
| `--in <name>=<path>` | Declared input. `<name>` matches `[a-z][a-z0-9_]{0,63}`. The path must be under `/workspace`, readable by the agent user, lie under one of the declaration's `path_under` folders, match its `glob` (against the base name) if any, and the profile must declare `<name>`; a declared input that is not `optional` is required, and a name given twice needs `multiple: true`; each violation is exit 125, cause `usage`. A path under `src/` (or `build/` when mounted) is used in place, not copied (already mounted: read-only, except `build/` for the build kind, which is read-write). Any other path must be a regular file and is copied by the tool: a single input to `in/<name>`; for `multiple: true`, to `in/<name>/<basename>`, and a duplicate basename for the same name is exit 125, cause `usage`. The copies of one call may total at most 1 GiB (provisional); more is exit 125, cause `setup`. Staging copies data; it never adds a mount. `{in.<name>}` in the command arguments (`run`, and the driver arguments of `build`) expands to the staged path, the list of staged paths (`multiple`, one argument each, when the placeholder is a whole argument), or the in-place path; embedded in a longer argument only a name with a single path is replaced. `meta.json` records `inputs: [{name, source, path, sha256}]`. Typical sources: `.input/` files, products in `.agent/<run>/work/<node>/`, an earlier execution's `.output/exec/<prev>/out/<file>`, files under `src/`. Not for `install` |
| `--manifest <path>` | `install` only; repeatable; required at least once. Path under `src/`, resolved like other paths. The tool builds both argvs of the install (see "Install in detail") from the profile plus the manifests (pip: `-r <manifest>`; the form for npm and conda is `TBD`: today an install profile whose target is not `.venv` fails with cause `setup`). There are no free manager args: args appended to the argv could switch the no-execution flags off |
| `--label <k>=<v>` | Extra container label. Key `[A-Za-z0-9][A-Za-z0-9._-]{0,127}`, value up to 256 characters; keys starting with `ephemeral.` are refused (usage). Labels are `ephemeral.exec=<execId>`, `ephemeral.pid=<tool PID>` plus the caller's (agent-graph sets `agent-graph.run` and `agent-graph.node`; the attempt is not a label, it is part of the exec ID) |
| `--timeout <s>` | Wall-clock limit, a positive integer; cannot exceed the profile's `timeout_s`; default is the profile's `timeout_s`. For install it is one limit covering both phases together |
| `-- <args...>` | `run`: the command (required), run with `/work` as working directory. `build`: arguments to the fixed driver. Not used by `install` |

The tool resolves all paths itself without following symlinks out of `/workspace` (`openat2` with `RESOLVE_BENEATH` and `RESOLVE_NO_SYMLINKS`, with a component-wise `O_NOFOLLOW` walk where `openat2` is missing), because the agent controls the workspace and can plant symlinks.

The execution folder is `/workspace/.output/exec/<execId>/`. The tool prints `run_ephemeral_container: exec_dir=<path>` as its first stderr line. It never writes `.output/<run>/`: final products reach it through agent-graph's existing staging.

`list` reads `.tools/` and reports, per profile: name, kind, version (from `tool.json`; the profile itself carries no version), `image_id`, whether `image.tar` is present, whether the tarball's image ID matches the profile's `image_id` (read from the tarball's manifest, without loading it), whether the image is loaded, and whether the profile is valid (schema and ceilings; an invalid profile is listed with its error). The default output is a table, `--json` gives the same as JSON, with the keys `name`, `kind`, `version`, `image_id`, `image_tar_present`, `image_tar_matches`, `image_loaded`, `valid`, `error`; for kind `tool` also `inputs` (name, glob, multiple, optional) and `params` (the param schema), not yet in code. Per-profile errors are also printed to stderr. It creates nothing and needs no engine (an unreachable engine only means "not loaded"). It is used by the agent-graph static check (USS-008) and by the agent (USS-009).

`sweep --label <k>=<v>` takes exactly one label (not an `ephemeral.*` key), removes only containers that carry exactly that label (force-removes each: kills it if running, then removes it; it pings the engine first) and prints one `swept <name>` line per container on stderr. agent-graph runs it at the start of a run that has an isolated step (see "Failure cases", rows "Tool killed" and "Cleanup / rollback").

### Kinds

| Kind | Subcommand | What it does | Read-only | Read-write | Not visible |
|---|---|---|---|---|---|
| code | `run` | Runs the caller's command on `src/` with dependencies from the env dirs | `src/`, profile env dirs, `build/` (if declared), `in/` | `out/` | `.input/`, `.process/`, `.tools/`, `.agent/`, `.output/` (except own `in/` and `out/`), other exec dirs, `.git`, logs |
| build | `build` | Runs a fixed driver (e.g. `make`) on `src/`; writes `build/` | `src/`, profile env dirs, `in/` | `build/`, `out/` | same as code |
| install | `install` | Two containers in one call: fetch (network, downloads only) then install (offline, runs a fixed dependency manager e.g. `pip`; writes one env dir). No package code is executed | `src/` (manifests), `.packages/`, other profile env dirs, `dl/` (install phase) | target env dir (install phase), `dl/` (fetch phase), `out/` | same as code, and `in/` |

Order for code from the workspace: `install` (fills env dirs) → `build` (fills `build/`, compiled languages only) → `run`. `run` never builds implicitly: an implicit build would give the executed code write access to `build/`, which later runs read. `build` is the only way `build/` gets written. Interpreted code (e.g. Python) needs no `build`. A code profile with `requires_build: true` (which needs `build_dir: true`) fails with cause `setup` if `build/` is missing or empty. Combining the steps (e.g. a build-and-run step) is a matter for the process definition in agent-graph, which issues separate calls.

### Install (`install`) in detail

`install` runs two phases, each an ephemeral container with the fixed settings. The fetch container is `ephemeral-<execId>-fetch`, the install container is `ephemeral-<execId>`; both carry the label `ephemeral.exec=<execId>`.

- fetch: network `host` (shares the agent container's network namespace, so the host-side controls of USS-004 still filter it); mounts `src/` ro, `.packages/` ro, `/workspace/.output/exec/<execId>/dl` rw, `out/` rw; command = the profile's `fetch_argv` with no-execution flags, e.g. `[pip, download, --only-binary=:all:, --find-links, /workspace/.packages, -d, /workspace/.output/exec/<execId>/dl]`.
- install: network `none`; mounts `src/` ro, `.packages/` ro, `dl/` ro, target env dir rw, other profile env dirs ro, `out/` rw; command = the profile's `argv`, offline from `dl/` and `.packages/`, e.g. `--no-index --find-links .../dl --find-links /workspace/.packages`.

No package code is executed in either phase: pip only wheels (`--only-binary=:all:`), npm `--ignore-scripts`, conda `TBD`. The caller supplies no free manager args; the tool builds both argvs from the profile plus the `--manifest` paths.

Timeout and cancel: one wall-clock limit (`--timeout`, default and ceiling the profile's `timeout_s`) covers both phases together. On timeout or cancel the tool kills the container of the running phase and makes sure both containers are removed (the install container is created but not started while fetch runs). Exit and cause are as for the other kinds (124 `timeout`, 130 `cancel`). `result.json` records which phase was running (`phase: fetch|install`).

Limitation: a package without a prebuilt wheel cannot be fetched. The fetch phase then fails with cause `fetch` (see "Exit codes and causes"); the operator builds or downloads the wheel on agent-host and copies it to `Workspace-Agents/<agentID>/.packages/`.

For pip the target is `.venv`, which starts empty. The install-python image ships a wrapper `install-into-venv`, used as `argv`: it runs `python -m venv /workspace/.venv` if `/workspace/.venv/pyvenv.cfg` is missing, then runs `/workspace/.venv/bin/pip` with its arguments.

### Reserved: kind `tool`

The subcommand `tool` and the kind `tool` are reserved for engineering tools. They use the same exec tool, exec dir, fixed container settings, profile store and engine authorization policy as the kinds above, and are defined in USS-009. Nothing else about them is specified here.

### Code run (`run`) in detail

- It executes the caller's argv in exec form. There is no shell unless the caller passes one (e.g. `sh -c`, if the image has it).
- Working directory is `/work`: a size-limited tmpfs (`noexec`), scratch, lost at the end. `src` is at `/workspace/src`, so the caller uses absolute paths for it. Products must be written to the folder given to the command as the environment variable `OUT=/workspace/.output/exec/<execId>/out`. Staged inputs are at `/workspace/.output/exec/<execId>/in/`, given as `IN=/workspace/.output/exec/<execId>/in`.
- `PATH` comes from the profile's `env` (see "Environment"), e.g. `/workspace/.venv/bin` first, or `/workspace/build/bin` when `build_dir` is declared; without it the image's own `PATH` applies.
- Interpreters and runtimes come from the image. Dependencies come from the env dirs (read-only). Compiled binaries come from `build/` (read-only, bound without `noexec`).
- Stdin is closed; there is no interaction.
- It sees only `src/`, the env dirs, `build/` if declared, `in/` and `out/`. It writes only `out/`, `/work`, `/tmp` and `HOME`. It has no network.
- It never installs or builds. Missing dependencies make the command fail; `requires_build` with no build gives exit 125, cause `setup`, before start.
- The exit code is the command's.

## Execution profiles

Profiles replace compiled-in image names. A profile is an operator-supplied manifest in `/workspace/.tools/<name>/profile.yml`. The caller selects it by name only; never an image, mount or flag. The store holds execution profiles of all kinds (code, build, install, and the reserved tool kind).

Why profiles: the caller may be LLM-driven, so it chooses only a name. Image, entry, mounts, limits and checks belong together, and the tool verifies the inspected container against exactly one profile as one unit. A new language or toolchain is a new profile plus an image, with no recompile.

Location: profiles in `/workspace/.tools/*/profile.yml`, image tarballs in `/workspace/.tools/<name>/image.tar`, both supplied by the operator. See "Profile store and supply" and "Profile trust".

| Field | Meaning |
|---|---|
| `name` | Profile name, `[a-z0-9][a-z0-9._-]{0,63}`; must equal the folder name in `.tools/` |
| `kind` | `code`, `build`, `install` (`tool` is reserved and refused, USS-009) |
| `image_id` | The engine's image ID, `sha256:<64 hex>`; pins the image (not a tag) |
| `ref` | Optional, informational `name:tag`; never used for lookup |
| `entry` | `agent` (code: the caller supplies the command); `fixed-args` (build: `argv` is the driver, the caller's driver args are appended; install: `argv` is the manager, the tool appends the `--manifest` arguments, no caller args); `fixed` (tool kind only, not implemented: `argv` is the whole command, caller args are not appended, unlike `fixed-args`; see "Profile schema extensions") |
| `argv` | Required and non-empty for `fixed-args`; absent for `agent`. Install: the install phase, offline. Exec form, no shell: `$VAR` is not expanded; entries are non-empty strings. The literal placeholder `<execId>` is substituted by the tool |
| `fetch_argv` | Install only (required there); fixed command line of the fetch phase, with no-execution flags (see "Install in detail"). Same exec form and `<execId>` substitution as `argv` |
| `env_dirs` | Map of env dir (`.venv`, `.npm-env`, `.conda-env`) to `ro` or `rw`. Read-only, except the one install target |
| `build_dir` | Whether the profile uses `build/`: read-write for build, read-only for a code profile that declares it |
| `requires_build` | Code only; bool, default false; needs `build_dir: true`. When true, `run` fails with cause `setup` if `build/` is missing or empty |
| `inputs` | Map `name` (`[a-z][a-z0-9_]{0,63}`) to `path_under` (list of workspace-relative folders, required, non-empty: each at or below `src/`, `build/`, `.input/`, `.agent/` or `.output/`; never `.tools/`, `.process/`, `.git`) and an optional `glob` (matched against the base name); `multiple: true` allows the name to be repeated with `--in`, `optional: true` allows it to be absent |
| `env` | Extra environment variables of the container (variable names `[A-Z_][A-Z0-9_]{0,63}`; `OUT`, `IN` and `HOME` are set by the tool and refused). Not an allowlist of names: every variable the profile names is set |
| `tmp_exec` | Bool, default false; `/tmp` is mounted without `noexec` when true |
| `limits` | All five required: `mem` and `out_max` as a size (`512m`, `2g`, `64k` or plain bytes, optional trailing `b`, powers of 1024), `cpus` (number, may be fractional), `pids` and `timeout_s` (integers), each above 0 and at most the ceiling of the tool (see "Profile trust") |

Stricter than the field list alone (all enforced by the profile parser; a violation makes the profile invalid, cause `setup`, and unknown fields are rejected):

- Every limit is required; there are no defaults.
- By kind: `code` needs `entry: agent` and has no `argv`/`fetch_argv`; `build` needs `entry: fixed-args`, `argv` and `build_dir: true`, and has no `fetch_argv` or `requires_build`; `install` needs `entry: fixed-args`, `argv` and `fetch_argv`, has no `build_dir`, `requires_build` or `inputs`, and needs exactly one env dir with `rw` (the target).
- Env dirs of a `code` or `build` profile are all `ro`.
- `env` may not set `OUT`, `IN` or `HOME`.
- A profile folder in `.tools/` whose name starts with `.` or that is not a directory is ignored; `profile.yml` is read up to 256 KiB.

The values in the examples are example values within the provisional ceilings, not defaults.

Example profiles:

```yaml
name: python
kind: code
image_id: sha256:…                # same base image and Python path as install-python
ref: exec-python:…
entry: agent
env_dirs: { .venv: ro }
env: { PATH: /workspace/.venv/bin:/usr/local/bin:/usr/bin:/bin }   # .venv first: python and pip resolve to the venv
build_dir: false
inputs: { data: { path_under: [.input/, .agent/, .output/exec/, src/], multiple: true } }   # e.g. a file from .input/
limits: { mem: 512m, cpus: 1, pids: 128, timeout_s: 600, out_max: 256m }
```

```yaml
name: c-make
kind: build
image_id: sha256:…
ref: exec-c:…
entry: fixed-args
argv: [make]                      # caller supplies only driver args
build_dir: true                   # writes /workspace/build
tmp_exec: true                    # compilers, try_compile
limits: { mem: 1g, cpus: 2, pids: 256, timeout_s: 1800, out_max: 512m }
```

```yaml
name: install-python
kind: install
image_id: sha256:…                # same base image and Python path as python: .venv holds absolute interpreter symlinks
ref: exec-python:…
entry: fixed-args
# fetch phase (network host): wheels only, nothing executed; <execId> is filled in by the tool
fetch_argv: [pip, download, --only-binary=:all:, --find-links, /workspace/.packages, -d, "/workspace/.output/exec/<execId>/dl"]
# install phase (network none): install-into-venv is a wrapper shipped in the image; it runs
# `python -m venv /workspace/.venv` if /workspace/.venv/pyvenv.cfg is missing (.venv starts empty),
# then /workspace/.venv/bin/pip with the arguments. The tool appends `-r <manifest>` per --manifest
argv: [install-into-venv, install, --no-index, --find-links, "/workspace/.output/exec/<execId>/dl", --find-links, /workspace/.packages]
env_dirs: { .venv: rw }
env: { PATH: /workspace/.venv/bin:/usr/local/bin:/usr/bin:/bin }   # .venv first: pip resolves to /workspace/.venv/bin/pip, so it installs into .venv
tmp_exec: true                    # build backends
limits: { mem: 1g, cpus: 2, pids: 256, timeout_s: 900, out_max: 512m }
```

Adding a language or toolchain means a new profile plus an image, with no code change in the tool, except for the `--manifest` form of an install target other than `.venv` (open).

### Profile store and supply

Profiles and their images are supplied by the operator manually, from a tool pool on agent-host, the same way process definitions are transferred (USC-008 step 1). No exec image or profile is baked into the agent image, and there is no extra mount.

- Tool pool on agent-host: location `TBD` (candidate `/var/lib/agent-sandbox/tool-pool/<name>/<version>/`). One folder per profile version, holding `profile.yml`, `image.tar` (from `docker save`) and `tool.json` (name, version, `image_id`, the registry digest if any for provenance, source or build reference). How images get into the pool (e.g. a host-side `create_exec_image` with the same pinning and credscan as `create_agent_image`) is an open item.
- Transfer: the operator copies a folder from the pool into the agent's workspace on the host, `Workspace-Agents/<agentID>/.tools/<name>/`, like process definitions into `.process/`. Inside the agent container it is `/workspace/.tools/<name>/`. Adding or updating a profile needs no image rebuild and no container recreate; the next call sees it.
- Profiles are read from `/workspace/.tools/*/profile.yml` on every call (no cache).
- Image: lazy `docker load` of `/workspace/.tools/<name>/image.tar` when the image is not loaded. Before the load the tool reads the tarball's manifest and refuses (cause `setup`) a tarball whose image ID differs from the profile's `image_id` (it also accepts the digests listed in `index.json` when present, containerd image store); after the load, `GET /images/<image_id>/json` must succeed and report that ID. The container is created with `Image: <image_id>`. The data root is a tmpfs, so images are reloaded after every agent container restart.
- `.tools/` is never mounted into an ephemeral container.

### Profile trust

`.tools/` lies in the workspace and is writable by uid 1001 at the file-system level, like `.process/`. Four controls:

- (a) Kind model compiled in. The mount model, capabilities, network, user and seccomp of each kind are compiled into `run_ephemeral_container`. A profile can only choose: `image_id`, entry, `argv` and `fetch_argv`, declared inputs, env dirs from the fixed set (`.venv`, `.npm-env`, `.conda-env`), `build_dir`, `requires_build`, `tmp_exec`, profile `env`, for the tool kind also `input_policy` and `params` (not implemented, see "Profile schema extensions"), and limits up to ceilings compiled into the tool and into the plugin from one shared package, so they are equal. Provisional values: memory 2 GiB, 2 CPUs, 512 pids, `timeout_s` 3600, `out_max` 1 GiB; the sizes of `/work` (512 MiB), `/tmp` (512 MiB) and `/home/run` (128 MiB) are fixed, not profile-selectable. A profile cannot add a mount, a capability, a network or privileged mode; the network of each kind is compiled into the tool (code, build: none; install: fetch phase `host`, install phase `none`); the tool rejects unknown fields (exit 125, cause `setup`). So a changed profile cannot widen what an ephemeral container sees.
- (b) The agent's own tools cannot write `.tools/` (USS-009).
- (c) Provenance: `tool.json` records the pool version and `image_id`. Checking it against the pool record on agent-host is an open item.
- (d) The engine authorization plugin enforces the container invariants independent of any profile (see "Engine authorization plugin"). Even a changed profile or a bypassed tool cannot produce a container outside them.

Residual risk: a changed profile could change a profile's argv or point to another image in `.tools/`, still inside the kind's fixed model and the plugin's invariants.

### Profile schema extensions (tool kind, USS-009)

Needed by USS-009 for the kind `tool`. These are new fields, not implemented: the profile parser rejects them today (`KnownFields(true)`, `internal/profile/profile.go`), so a profile that uses them is invalid (cause `setup`). The other kinds keep `<execId>` and `$OUT` as before.

- `entry: fixed` (tool kind only): the argv comes entirely from the profile. Caller args are not appended, unlike `fixed-args`.
- argv placeholders (tool kind): `{out}`, `{work}`, `{in.<name>}`, `{param.<k>}`. `{in.<name>}` and `{param.<k>}` each substitute as one whole argv element; `{out}` and `{work}` may also prefix a path in an element (`{out}/sim.raw`). No splitting and no shell. An element whose `{in.<name>}` refers to an optional input that is absent is dropped.
- `input_policy: <name>`: the name of a filter compiled into `run_ephemeral_container`. An unknown name makes the profile invalid (cause `setup`). A file that fails the filter ends the call with exit 125, cause `policy`, and nothing is created.
- `params: {<k>: {type, required, default, ...}}`. Types: `string` (optional `pattern`), `int` and `number` (optional `min`/`max`), `bool`, `enum` (`values`). An unknown key, a wrong type or a missing required param is exit 125, cause `usage`. Open: a unit-aware numeric type (e.g. SPICE suffixes).
- `inputs[].glob` stays a single string. A tool that needs two patterns declares two inputs.

### Invocation examples

One per kind, for agent-graph run `r1`. The working directory is `/work`, so `src/` is addressed as `/workspace/src`.

```
run_ephemeral_container install install-python --exec-id e1 --manifest /workspace/src/requirements.txt
run_ephemeral_container build c-make --exec-id e2 -- -C /workspace/src BUILD_DIR=/workspace/build
run_ephemeral_container run python --exec-id e3 --in data=/workspace/.input/measurements.csv -- python /workspace/src/analyse.py /workspace/.output/exec/e3/in/data
```

The folders are `/workspace/.output/exec/e1` to `e3`. The `analyse.py` products go to `$OUT`, i.e. `/workspace/.output/exec/e3/out/`.

Process YAML step that produces the `run` call (implemented form; only `isolate` and `profile` are step fields for this, `run` is one string that agent-graph splits with `shlex` into an argument list, with the usual required step fields `id`, `name`, `type` and `instruction`):

```yaml
- id: 2
  name: analyse
  type: procedure
  reads: ["input.source[Data]"]
  produces: [{name: report, file: report.json}]
  instruction: |
    ...
  run: "python src/analyse.py {input.Data} {report}"
  isolate: true
  profile: python
```

agent-graph turns it into this call (`ContainerExec.run_isolated`; the tool is `AGENT_GRAPH_EXEC_TOOL`, else `run_ephemeral_container` on `PATH`, else `/usr/local/bin/run_ephemeral_container`):

```
run_ephemeral_container run python --exec-id ag-analyse-1-1-3f9a1c20 --in data=/workspace/.input/measurements.csv --label agent-graph.run=<runID> --label agent-graph.node=analyse -- python /workspace/src/analyse.py {in.data} /workspace/.output/exec/ag-analyse-1-1-3f9a1c20/out/report.json
```

- Exec ID: `ag-<node>-<visit>-<attempt>-<8 hex>`, within the tool's pattern. Labels: `agent-graph.run` and `agent-graph.node` only.
- Placeholders: a bound input or a previous step's product given as a whole argument becomes `{in.<name>}` plus one `--in` per file; the tool input name is the process name in lowercase with `-` turned into `_` and must match `[a-z][a-z0-9_]*` (static check `isolate_input_name`). Embedded in a longer argument, such a placeholder is accepted only for a file under `src/` (visible in place). `{work}` and the step's own product names become paths in the execution's `out/` (`$OUT`).
- Script: the first non-option argument of an interpreter command must lie under `src/` (static check `isolate_script_outside_src`; the `.process/scripts/` rule of non-isolated steps does not apply), and is made absolute against the workspace root. The executable allowlist and the refusal of inline code stay (today `python`, `python3`, `/opt/venv/bin/python`).
- After the run, the files of `out/` are copied into the step's work folder (`.agent/<runID>/work/<node>/`, symlinks skipped), so the command writes `$OUT/.outcome` and its products to `$OUT`, and the usual product checks and staging follow.
- Result: `result.json` of the exec dir decides exit code and cause; if `result.json` is missing and the exit code is non-zero, the cause is `setup` (unless the tool's stderr names an early cause); a cause other than `ok` or `exit` on exit 0 still fails the step. `usage`, `setup`, `denied` and `verify` are not retried (retrying cannot help); every other failure follows the step's `retry`.
- Timeout and cancel: agent-graph enforces the step timeout itself (it does not pass `--timeout`): it sends SIGTERM to the tool, waits 10 s, then kills it.
- Static check (S3): for an isolated step, `profile` is required, a `profile` without `isolate: true` is an error, and, when the tool can be run, the profile must exist, be valid and have kind `code` (`isolate_profile_missing`, `isolate_profile_invalid`, `isolate_profile_unknown`). Without the tool the check is skipped. It does not check `image.tar`, and the process schema has no `runtime.profiles` list to check against.
- At the start of a run that has an isolated step, agent-graph runs `sweep --label agent-graph.run=<runID>` (best effort, errors ignored).

Open: `build` and `install` are not reachable from a process (the schema has no `args`, `manifests` or `inputs` step fields, and `run_isolated` only calls `run`). The richer form is only an idea, for example a `deps` step with the install profile and `manifests: [...]`, or a build step with the build profile and `args`.

Notes:

- Build: building executes code from `src/` (Makefiles, build scripts, compiler plugins). Build is treated like a code run for isolation.
- Install: no package code is executed in either phase (pip only wheels, npm `--ignore-scripts`, conda `TBD`), and the install phase has no network. A package without a prebuilt wheel fails the fetch phase (cause `fetch`); see "Install in detail" and the open items.

## Build and delivery (Go)

- Three Go programs, static binaries for `linux/amd64`: `run_ephemeral_container`, the engine authorization plugin `ephemeral-authz` and `agent-init` (same Go module or separate, `TBD`). All three programs are in one Go module, `packages/agent-exec`. They are built there with the static build command in `packages/agent-exec/README.md`, staged to `bin/` in howto step P8, and sent in the build context by `create_agent_image`. A CI build location is still `TBD`.
- Delivery is through USS-003: `Dockerfile.agent` installs `/usr/local/bin/run_ephemeral_container`, `/usr/local/libexec/ephemeral-authz` and `/usr/local/sbin/agent-init` from `bin/` (`root:root`, mode 755; the build fails if one is missing), creates the system user `authz`, adds the `authorization-plugins` key to `daemon.json`, and sets `ENTRYPOINT`/`CMD` to `tini` → `agent-init`. The image builder sends `bin/agent-init`, `bin/ephemeral-authz` and `bin/run_ephemeral_container` (`config.BinFiles`). No exec images or profiles are in the agent image (see "Profile store and supply"). The agent is already in the group `docker` (SSR-AGC-011); no other group is needed.
- Where the exec images are built into the tool pool is open. Candidate: a host-side `create_exec_image` alongside `create_agent_image`, with the same pinning and credscan.
- `run_ephemeral_container` is not setuid; `no-new-privileges` (SSR-AGC-010) would block that. It runs as the agent user and reaches the in-container Docker daemon through the engine socket by the group `docker`.

## Preconditions

- USS-002 is complete: the agent container is running and armed.
- The in-container Docker daemon is running and its socket `/var/run/docker.sock` is reachable by the agent user (group `docker`, SSR-AGC-011). The engine authorization plugin is running (otherwise the daemon fails every request).
- The daemon is configured as described in "In-container daemon configuration": no bridge and no iptables management, and its data root `/var/lib/docker` lives inside the agent container, on a size-bounded `tmpfs` (2g, USS-002), not on a host path (SSR-AGC-004).
- The profile exists in `/workspace/.tools/<name>/` with its `image.tar` (or the image is already loaded). The tool neither pulls nor builds images.
- agent-host root has created `build/`, `.venv`, `.npm-env`, `.conda-env` and `.packages/` (empty, owned by uid 1001) together with the other workspace folders when the agent container was created (USS-002). The tool never creates them; a missing one is exit 125, cause `setup`. It checks only the folders the call needs: `src/`, `build/` when the kind mounts it, the profile's env dirs, and `.packages/` for install. See USS-002, USC-002 and SFR-AGC-004.
- For `install`: the target env dir exists (see above) and each `--manifest` path exists under `src/`.

## In-container daemon configuration

`/etc/docker/daemon.json` is written at image build by `Dockerfile.agent` (`RUN printf`, `root:root`, mode 644). `dockerd` is started by `agent-init` without flags, so every setting is in this file (a setting given both as a flag and in the file stops `dockerd` from starting). Process tree: `tini` (PID 1) → `agent-init` (root) → `ephemeral-authz` (user `authz`) and `dockerd` (root). `Dockerfile.agent` sets `tini` → `agent-init` as the entrypoint and writes the content below, the `authorization-plugins` key included; `agent-init` is implemented but not yet run with a real `dockerd`. The file as written:

```json
{
  "default-runtime": "runc",
  "authorization-plugins": ["ephemeral-authz"],
  "bridge": "none", "iptables": false, "ip6tables": false,
  "hosts": ["unix:///var/run/docker.sock"], "group": "docker",
  "data-root": "/var/lib/docker",
  "exec-opts": ["native.cgroupdriver=cgroupfs"],
  "no-new-privileges": true
}
```

| Key | Value | Why |
|---|---|---|
| `default-runtime` | `runc` | Ephemeral containers run under `runc` inside gVisor, not a second gVisor |
| `bridge` | `none` | No bridge network inside the agent container |
| `iptables`, `ip6tables` | `false` | The daemon does not manage firewall rules; no bridge or NAT inside, so `NET_RAW` is not needed |
| `hosts` | `unix:///var/run/docker.sock` | Local engine socket only |
| `group` | `docker` | Socket group; the agent user is a member (SSR-AGC-011) |
| `data-root` | `/var/lib/docker` | Inside the agent container, on the 2g `tmpfs` (USS-002, SSR-AGC-004) |
| `exec-opts` | `native.cgroupdriver=cgroupfs` | cgroup driver for a daemon started without systemd |
| `no-new-privileges` | `true` | Default for every container; the tool still sets and verifies it per container |
| `authorization-plugins` | `["ephemeral-authz"]` | Every API request is checked by the plugin (default deny). `dockerd` reaches it at `/run/docker/plugins/ephemeral-authz.sock`; `Dockerfile.agent` does not create that directory (`/run` is runtime), `agent-init` does |

To change it: edit `Dockerfile.agent`, rebuild the agent image (USS-003) and recreate the agent containers. Never edit it in a running container: the file is root-owned and the agent user cannot. The engine comes from the Ubuntu package `docker.io`, pinned to an exact version in `Dockerfile.agent`; that it contains the CVE-2024-41110 fix is still to be checked.

Not in the file yet (candidates, `TBD` until the spike; `Dockerfile.agent` carries a `TODO` for the storage driver only):

- `storage-driver`: `overlay2` or `vfs` under gVisor.
- `log-driver` / `log-opts`, bounded (e.g. `json-file` with `max-size`), because the data root is a memory-backed tmpfs.
- `default-ulimits` (`nofile`, `nproc`).
- Pin the image store explicitly (`features.containerd-snapshotter`). The meaning of `image_id` depends on it (classic store: config digest; containerd store: manifest digest), so the pool-side build must compute `image_id` with the same store and engine version.

## Engine authorization plugin

Purpose: the engine socket stays reachable by the agent user (group `docker`, SSR-AGC-011), but the engine only does what the defined scenarios need (code, build, install, and the reserved tool kind).

| Item | Value |
|---|---|
| Name | `ephemeral-authz` |
| Binary | Go static binary `/usr/local/libexec/ephemeral-authz`, `root:root` 755, in the agent image (USS-003) |
| Type | Legacy (non-managed) Docker authorization plugin |
| Socket | `/run/docker/plugins/ephemeral-authz.sock`, created and bound by `agent-init` (root): owner root, mode 0600; the directory is root-owned. `agent-init` passes the listening socket to the plugin as an inherited file descriptor (fd 3), so the plugin needs no write access to the directory. The plugin's `--socket <path>` option binds a socket itself (mode 0600); it is for tests and development only |
| User | Dedicated non-root system user and group `authz`, created by `Dockerfile.agent` (no home, no login shell; uid from the system range, picked at image build; not 1001, not in the group `docker`). The agent (uid 1001) sends every request to the engine socket `/var/run/docker.sock`; only `dockerd` (root) connects to the plugin socket and forwards each request to it. uid 1001 cannot connect to the plugin socket, replace it or signal the plugin |
| Configuration | `"authorization-plugins": ["ephemeral-authz"]` in `daemon.json` |
| Policy source | Compiled in (rules and ceilings). The plugin reads nothing from the workspace |

Start order (`agent-init`, implemented): `tini` cannot supervise two children. `tini` starts `agent-init` (root, Go, `/usr/local/sbin/agent-init`), which creates and binds the plugin socket, starts the plugin with the listening socket as an inherited file descriptor, then starts `dockerd`.

- If `dockerd` exits, `agent-init` exits (as today when `dockerd` dies).
- If the plugin exits, `agent-init` waits 1 s (backoff) and restarts it with the same socket. On shutdown it sends the plugin SIGTERM, then SIGKILL after 3 s. Meanwhile `dockerd` fails every request (fail closed, to verify with the pinned engine version).

Policy: default deny. A Unix socket gives no caller identity, so the policy applies to every client, including the exec tool itself.

Endpoints (API version prefix normalised). Everything not listed is denied:

| Group | Allowed |
|---|---|
| Engine info | `GET /_ping`, `HEAD /_ping`, `GET /version`, `GET /info` |
| Images | `GET /images/json`, `GET /images/{id}/json`, `POST /images/load` |
| Create | `POST /containers/create` (body checked, see below) |
| Inspect, list | `GET /containers/json`, `GET /containers/{id}/json` |
| Lifecycle | `POST /containers/{id}/start` (empty body only), `POST /containers/{id}/attach`, `GET /containers/{id}/logs`, `POST /containers/{id}/wait`, `POST /containers/{id}/kill`, `DELETE /containers/{id}` |
| Events | `GET /events` |

Explicitly denied (examples): exec (`/containers/{id}/exec`, `/exec/*`), `archive` (docker cp), export, commit, update, build, pull (`/images/create`), push, tag, networks, volumes, plugins, swarm, session, grpc.

Request normalisation, before any rule: the URI path must be canonical (no `.`, `..` or empty segments) and must not contain a percent-encoded character, and the query must parse, else the request is denied; an optional API version prefix `/v<major>.<minor>` is stripped. The decision is made on the request only; the plugin always allows the response. Any panic is a deny. Each deny is logged on the plugin's stderr (one line, with the reason).

Kill and delete need no label check: every container that exists was created under the policy.

Create invariants (`Config`/`HostConfig`). Any violation, or a missing or unparsable body, is denied:

| Area | Required |
|---|---|
| User | exactly `1001:1001`; `GroupAdd` empty |
| Privilege | `Privileged` false; `CapAdd` empty; `CapDrop` contains `ALL` (the tool sets exactly `[ALL]` and verifies it) |
| Security options | `SecurityOpt` contains `no-new-privileges` (value true) and no option with key `seccomp`, `label` or `systempaths` at all, and no `apparmor=unconfined` (`:` and `=` forms alike). Stricter than "unconfined only": the tool sets no other option, so any such option is a deviation |
| Root file system | `ReadonlyRootfs` true; `MaskedPaths` and `ReadonlyPaths` not set |
| Network | `NetworkMode` `none` or `host`. A `host` container has no more network than the agent user already has; keeping executed code offline is enforced by the tool per kind (only the fetch phase of install uses `host`) |
| Namespaces | Pid, Ipc, UTS, Userns modes empty or `private` (no `host`, no `container:`); `CgroupnsMode` exactly `host` (gVisor has no cgroup namespaces; inside the agent container `host` is the agent container's own cgroup view, never agent-host's; decision 2026-11-12) |
| Forbidden fields | no `Devices`, `DeviceRequests`, `DeviceCgroupRules`, `Sysctls`, `CgroupParent`, `PortBindings`, `PublishAllPorts`, `Links`, `VolumesFrom`, anonymous `Volumes`, volume driver; no `Binds` (binds only as `Mounts`, see below); `LogConfig` driver only `json-file`, `local` or `none` (or unset) |
| Runtime | unset or `runc` only |
| Limits | `OomKillDisable` not true; `Memory`, `NanoCpus`, `PidsLimit` set (above 0) and at most the ceilings compiled into the plugin |
| Identity | exactly one `name` query parameter, `ephemeral-<id>` or `ephemeral-<id>-fetch`, and label `ephemeral.exec=<id>` present (`<id>` matching `[a-z0-9][a-z0-9-]{0,63}`), with `<id>` equal in both |
| Removal | `AutoRemove` true |
| Mounts | only through `HostConfig.Mounts`, types bind and tmpfs. Bind sources only: `/workspace/src` (ro), `/workspace/build` (ro or rw), `/workspace/.venv`, `/workspace/.npm-env`, `/workspace/.conda-env` (ro or rw), `/workspace/.packages` (ro only), `/workspace/.output/exec/<id>/in` (ro), `/workspace/.output/exec/<id>/dl` (ro or rw), `/workspace/.output/exec/<id>/out` (rw), with `<id>` equal to the label; the target equals the source; bind propagation unset or `rprivate`; no `CreateMountpoint`, volume or image options; no duplicate target; no source or target containing `docker.sock`. Each component of a bind source is `Lstat`ed at create: a symlink or a missing component denies the create. Tmpfs targets only `/work`, `/tmp`, `/home/run`, each with a size within its ceiling (512 MiB, 512 MiB, 128 MiB, provisional), either as the `HostConfig.Tmpfs` map, which the tool uses (options accepted: `rw`, `ro`, `exec`, `noexec`, `suid`, `nosuid`, `dev`, `nodev`, `mode=<octal>`, and `size=` exactly once, required), or as tmpfs `Mounts` with a size; a target in both is denied. The plugin does not require the three tmpfs to be present. Never the engine socket, `/`, `/etc`, `/home/agent`, `/var/lib/docker` (none is on the allowlist) |

Not examined by the plugin: `Image`, `Cmd`, `Entrypoint`, `Env`, `WorkingDir`, labels other than `ephemeral.exec`. These stay with the tool's inspect-verify step (the container still runs as uid 1001 without capabilities and sees only the allowlisted paths, whatever they are).

Claim: every container the engine can start runs as uid 1001 without capabilities and without host namespaces, so it can never do more than the agent user itself, and it sees only the allowlisted paths. The group `docker` is then no longer root-equivalent.

Residual risks:

- Body forwarding: `dockerd` forwards request bodies to the plugin only up to a size limit and only as JSON. A create without a body, or one that cannot be parsed, is denied. This is the class of CVE-2024-41110 (authorization bypass); pin an engine version that contains the fix.
- TOCTOU on bind sources: the agent can swap a checked directory for a symlink between create and start. Low impact for this reason: a container as uid 1001 without capabilities cannot write what uid 1001 cannot write. The symlink check at create still matters: the workspace folder is owned by uid 1001, so the agent can replace e.g. `build/` with a symlink, and a bind resolved by root skips the permissions of the parent directories.
- No caller identity: the policy cannot tell the exec tool from any other uid 1001 process; it holds for all of them.
- Image content is not checked by the plugin (the `image_id` check stays with the exec tool). `POST /images/load` can fill the 2g tmpfs (memory denial of service, accepted).

Relation to the exec tool: the plugin does not replace the tool's own validation and its inspect-verify step (Interaction, step 7). It is the hard backstop that holds when the agent bypasses the tool, e.g. `docker run --privileged -v /:/host` from any uid 1001 process is denied.

Fail-closed behaviour (to verify): when the plugin is not running, `dockerd` must deny every request, not allow it. To verify with the pinned engine version.

Verification (agent-sandbox final verification; follow-up, not implemented by this story):

- `docker info` as uid 1001 lists the authorization plugin.
- A privileged create as uid 1001 is denied.

## Interaction

All steps are done by the tool, as the agent user.

1. The tool parses the arguments, loads the profiles from `/workspace/.tools/*/profile.yml` and validates the request: subcommand, profile, kind match, `--exec-id` (pattern, not existing; a generated one is retried until free), `--in` against the profile (declared name, `path_under`, `glob`, required inputs present, for `multiple` names no duplicate basename), `--manifest` paths (install: under `src/`, existing regular files), `--timeout` against `timeout_s`. It resolves every path without following symlinks out of `/workspace`. It builds the argv of every container here (placeholders expanded, `<execId>` substituted). Nothing is written yet.
2. The tool checks that the daemon is reachable (`GET /_ping`), removes stale containers (every container labelled `ephemeral.exec` whose recorded tool PID, label `ephemeral.pid`, no longer exists; silently, so that the `exec_dir` line stays the first stderr line), checks that the workspace folders the call needs exist, takes the locks (see "Persistent folders and concurrency") and a slot.
3. The tool creates `/workspace/.output/exec/<execId>/` with `in/` (not for install), `out/` and, for install, `dl/` (uid 1001) and prints `run_ephemeral_container: exec_dir=<path>` as its first stderr line. Failures before this step create nothing and write no `result.json`.
4. The tool stages the inputs: a path under `src/` (or `build/` when mounted) is used in place (not copied, but hashed when it is a file); any other path is copied into `in/<name>` (single input) or `in/<name>/<basename>` (`multiple`), within the 1 GiB total staging cap, and hashed. `{in.<name>}` expands to the staged path, the list of staged paths, or the in-place path. A failure exits 125 with cause `setup`. Install has no inputs; its manifests are read in place from `src/`.
5. The tool ensures the image. If it is absent, it runs `docker load` of `/workspace/.tools/<name>/image.tar`; `GET /images/<image_id>/json` must then succeed. It never pulls and never builds.
6. The tool creates (does not start) `ephemeral-<execId>` with `Image: <image_id>`, the fixed settings and the labels. For install it creates both containers, `ephemeral-<execId>-fetch` (fetch phase) and `ephemeral-<execId>` (install phase), before starting either. If the engine denies a request (authorization: HTTP 403 or a message "authorization denied"), the tool exits 125 with cause `denied`; any other engine error is cause `setup`.
7. The tool inspects the created container (for install, both) and verifies on the inspected container, not from its own request: name, capabilities, no-new-privileges, read-only root file system, user 1001:1001, network, private namespaces, default runtime, none of the forbidden fields, exact mount allowlist and tmpfs (size and exec/noexec), the container's `Image` equal to `image_id`, limits, `AutoRemove` true, an empty entrypoint, `/work` as working directory, no stdin or tty, the command, the exec label, environment (the variables the tool sets with their values, otherwise only the image's `ENV` or the engine's default `PATH`). On a mismatch it removes the container and exits 125 with cause `verify`. Inspect-verify catches faulty settings and profile mismatches; it is not a boundary against the agent. The hard backstop is the engine authorization plugin, which denies any container outside the invariants even if the agent bypasses the tool.
8. The tool writes `meta.json` once, before start, and never changes it (see "Exec dir layout"), then starts the container, streams stdout and stderr to its own output and into `stdout.log` and `stderr.log` (created just before the first start), and watches the timeout and `out_max` (a poll every second). Before the start it subscribes to the container's events, registers the wait and attaches to the output. The timeout is one limit for the whole execution (install: both phases together). Because the container may be gone (AutoRemove) before it can be inspected after exit, the tool detects OOM from the engine's `oom` event (`GET /events`, filtered on its container) and, if the wait fails, takes the exit code from the `die` event. For install, the tool starts the fetch container first; when it ends with a non-zero exit, the tool removes the install container and exits with the manager's exit code, cause `fetch`. Otherwise it starts the install container. On timeout the tool kills the container of the running phase (install: fetch or install) and exits 124, cause `timeout`.
9. On SIGTERM or SIGINT the tool kills the running container and removes it (install: kills the running phase's container and removes both); cause `cancel`, exit 130.
10. The tool makes sure the container is removed (install: both, `ephemeral-<execId>-fetch` and `ephemeral-<execId>`; AutoRemove; removes it itself if still present), releases the locks, writes `result.json` and exits with the execution's exit code.

If the tool itself is killed (`kill -9`), the container may remain. It is removed by the next tool start (step 2) or by `sweep --label <k>=<v>`.

## Ephemeral container settings (fixed)

Applied by the tool to every execution; not selectable by argument. The plugin enforces the same invariants at the engine (see "Engine authorization plugin").

Common to all kinds:

- capabilities: all dropped (`--cap-drop ALL`), plus `no-new-privileges`;
- default seccomp profile;
- user: the agent UID/GID 1001, so files written to the workspace have the same single owner as the rest of the workspace (USS-003);
- root file system read-only, with size-limited `tmpfs` at `/work` (512 MiB), `/tmp` (512 MiB) and `/home/run` (128 MiB, `HOME`), sizes fixed and provisional, options `rw`, `nosuid`, `nodev`, mode 1777; `/work` and `/home/run` are always `noexec`, `/tmp` is `noexec` unless the profile sets `tmp_exec`. The tool passes them as the `HostConfig.Tmpfs` map and the binds as `HostConfig.Mounts` of type bind (no `Binds`, no `noexec` on binds);
- the image's entrypoint is reset (empty `Entrypoint`; the command is the tool's argv), working directory `/work`, stdin closed, no tty, IPC namespace private, cgroup namespace `host` (gVisor, the agent container's runtime, has no cgroup namespaces, so runc refuses a private one with "cgroup namespaces aren't enabled in the kernel"; `host` there is the agent container's own cgroup view, read-only for the executed code; the memory, CPU and process limits still apply; decision 2026-11-12), default runtime;
- no credential mount, no Docker socket;
- bounded memory, CPU and process count from the profile; they must fit inside the agent container's own bounds (SSR-AGC-012), which the tool checks by the slot rule of "Persistent folders and concurrency";
- removed when the execution ends (`--rm` semantics): the tool creates the container with AutoRemove set and still removes it itself if it remains (e.g. after a failed start or a kill).

Environment: the tool sets only `HOME=/home/run`, `LANG=C.UTF-8`, `TZ=UTC`, `OUT` (all kinds) and `IN` (code and build; the `out/` and `in/` folders of the execution; install has no `in/` mount and no `--in`), plus the profile's `env`. Nothing from the agent environment. The image's own `ENV` is inherited as part of the pinned image (the image default `PATH` is one of them). The tool sets `PATH` only through the profile's `env` (e.g. `PATH: /workspace/.venv/bin:/usr/local/bin:/usr/bin:/bin`); without it `PATH` is the image's, or the engine's default when the image has none. The caller cannot set any variable. A profile `env` may not name `OUT`, `IN` or `HOME`; it can override `LANG` and `TZ`.

### Mounts (allowlist, not mask)

The kind and the profile define every mount and its mode; no argument changes them. Mount only what the kind needs. Anything not listed is invisible, including folders added to the workspace later. Paths are identical inside and outside the container, except `/work`. Interpreters and toolchains come from the image, not the workspace.

| Path in container | code | build | install |
|---|---|---|---|
| `/workspace/src` | ro | ro | ro (manifests), both phases |
| `/workspace/build` | ro if profile declares | rw | not mounted |
| Env dirs named in the profile | ro | ro | install phase: target rw, others ro; fetch phase: not mounted |
| `/workspace/.packages` | not mounted | not mounted | ro, both phases |
| `/workspace/.output/exec/<execId>/in` | ro | ro | not mounted |
| `/workspace/.output/exec/<execId>/dl` | not mounted | not mounted | fetch rw, install ro |
| `/workspace/.output/exec/<execId>/out` | rw | rw | rw, both phases |
| `/work` | tmpfs, size-limited (512 MiB), `noexec`, working directory | same | same |
| `/tmp` | tmpfs (512 MiB); `noexec` unless `tmp_exec: true` | same; `tmp_exec` expected true | same; `tmp_exec` expected true |
| `/home/run` (HOME) | tmpfs (128 MiB), `noexec` | same | same |

Never mounted: `.input/`, `.process/`, `.tools/`, `.agent/`, `.output/` outside the own exec dir's `in/` and `out/`, other exec dirs, `.git`, `/etc/claude-code` (the agent container's policy bundle, USS-009; the plugin's bind allowlist never allows it), credentials, the engine socket. `build/` and `src/` are read-only for every kind, except that `build` writes `build/`.

Besides `out/` (and `/work`, `/tmp`, `HOME`), the only writable workspace paths are the install target env dir (install phase), `dl/` (fetch phase) and the build dir. `.packages/` is never writable.

- `.agent/` holds agent-graph run state (`checkpoint.sqlite`, `history.jsonl`, `graph.json`, `binding.json`, other nodes' results). Executed code never needs it; products it needs are copied into `in/`.
- `.git`: in a worktree it is a one-line file (`gitdir: <host path>`) whose target is not mounted (dangling; leaks only a host path). In a plain clone it holds the full history, remote URLs, possible credential helpers and hooks. It is not mounted either way. git is not in the agent container. If a step needs version info, the caller passes it as a parameter or argument.

### Per kind

| Setting | code | build | install |
|---|---|---|---|
| Image | Profile image | Profile image | Profile image (same base and Python path as the matching code profile) |
| Entry | The caller's `<command>` | Fixed driver of the profile plus the caller's driver args | Fixed manager of the profile: `fetch_argv` (fetch phase) and `argv` (install phase), plus the arguments built from `--manifest`; no caller args |
| Mounts | Table above | Table above | Table above |
| Network | none (`--network none`) | none; offline (see below) | fetch phase `host`, install phase `none` |
| Input check | none (code is untrusted by definition) | none | none |

Build is offline. Dependencies come only from env dirs filled by `install` (e.g. `cargo --offline`, `npm` with no registry).

### Persistent folders and concurrency

Env dirs and `build/` outlive the run.

- A run holding one of them rw takes an exclusive `flock` on the folder (`build/` for the build kind, the target env dir for install). A run mounting it ro takes a shared lock (`build/` for a code profile that declares it, every other env dir). Locks are taken in sorted folder order, before the exec dir is created, held for the whole call (both install phases) and released at its end.
- The tool refuses (exit 125, cause `busy`) rather than waiting beyond 2 s (provisional; it retries every 20 ms).
- Slot rule: the tool lists the containers labelled `ephemeral.exec` (after removing the stale ones), keeps those in state `running` or `created`, groups them by exec ID (an install pair counts once, with the larger value of each limit), adds its own profile's limits and refuses (`busy`) if the sum of memory, CPUs or pids exceeds the agent container's bounds (SSR-AGC-012; compiled in, provisional: 4 GiB, 4 CPUs, 2048 pids). There is no central process: the count is a read of the engine at start, so two tool processes that start at the same moment can both pass it (the first has not created its containers yet), and the sum can then exceed the bounds. Open.

## Exec dir layout

```
/workspace/.output/exec/<execId>/
  meta.json    written once, before start, never changed: request (profile, kind, exec_id, args, manifests, labels, timeout_s), inputs, image_id, and per container (phase) its name, argv and inspected settings (user, caps, security options, network, mounts, tmpfs, limits, labels)
  result.json  {exit, cause, exec_dir, image_id, start, end}; install also {phase: fetch|install}, the phase running when the execution ended
  stdout.log   streamed by the tool (agent side)
  stderr.log   streamed by the tool (agent side)
  in/          staged inputs (mounted ro; not for install)
  dl/          downloaded packages (install only; rw in fetch, ro in install)
  out/         the only workspace path the code writes (mounted rw), besides build/ for build and the target env dir for install
```

- Created by the tool (as uid 1001) for each execution. `.output/exec/` is reserved for executions; agent-graph final products stay in `.output/<run>/`.
- An execution that fails after the folder exists but before the start (staging, image, create, verify) leaves `result.json` and what was created so far, but no `meta.json` and no logs: both are written only when the start is reached (`meta.json` just before, the logs at the start).
- `meta.json` carries `inputs: [{name, source, path, sha256}]` (`path` is the path inside the container; `sha256` is empty for an in-place directory). For install it records both phases (fetch and install: argv and inspected settings of each); `result.json` records which phase was running when the execution ended (`phase: fetch|install`, install kind only).
- Only `in/`, `out/` and `dl/` are mounted. Logs, `meta.json` and `result.json` are never mounted, so executed code cannot see, overwrite or forge them.
- The tool's own stdout and stderr carry only the streamed command output, except the first stderr line `run_ephemeral_container: exec_dir=<path>`. Tool diagnostics go to stderr, prefixed `run_ephemeral_container:`. The machine-readable result is `result.json`, not a stdout line.
- Output cap: the tool watches `out/` (for build also `build/`, for install also `dl/` and the target env dir) and kills the run when the growth exceeds `out_max`: it sums the sizes of the regular files of those folders once before the first start, and compares the growth since then (one sum over all watched folders, not one per folder) every second. Content that was there before the run (a filled `build/`, an env dir) does not count. Symlinks are not followed. `in/` is not watched. A size-limited tmpfs with copy-back is an open item.

## Exit codes and causes

`result.json` is authoritative. It is always written once the exec dir was created; a failure before that (usage, daemon unreachable, invalid profile, missing folder, busy) creates nothing and writes none. A command's own exit code can equal 124, 125, 130 or 137; only `cause` distinguishes these.

| Exit | Cause | Meaning |
|---|---|---|
| 0 | `ok` | Completed; command, driver or manager exited 0 |
| its own | `exit` | Completed; the command, driver or manager's own non-zero exit code (install: the install phase) |
| the manager's | `fetch` | Install fetch phase failed (e.g. a package without a prebuilt wheel). The tool exits with the manager's exit code and prints on stderr: `run_ephemeral_container: fetch failed: packages without a prebuilt wheel must be built or downloaded as a wheel on agent-host and copied by the operator to Workspace-Agents/<agentID>/.packages/`. agent-graph surfaces this cause and message to the operator |
| 124 | `timeout` | Timeout reached |
| 125 | `usage` | Bad subcommand, unknown profile name, kind mismatch, `--exec-id`, `--in` (incl. duplicate basename), `--manifest` or `--timeout` |
| 125 | `setup` | Daemon unreachable, invalid profile, image load or `image_id` failure, exec dir creation or input staging failure, missing workspace folder (`build/`, env dir, `.packages/`) |
| 125 | `denied` | The engine refused a request (authorization plugin) |
| 125 | `verify` | Inspected settings differ from the fixed settings |
| 125 | `busy` | Lock or slot not available within the bound |
| 125 | `policy` | An input file was rejected by the profile's `input_policy`; nothing is created. Tool kind (USS-009); not yet in code (`internal/sequence/sequence.go` has no `policy` cause) |
| 130 | `cancel` | SIGTERM or SIGINT to the tool |
| 137 | `oom` or `out_cap` | Killed with SIGKILL; `cause` tells which |

## Agent container dependency (summary)

Running a Docker daemon inside the agent container needs `SYS_ADMIN` (mounts, namespaces, cgroups) on the agent container's capability allowlist (SSR-AGC-009). It is held by the gVisor Sentry, not by the `agent-host` kernel. `NET_ADMIN` (creating the network namespace and bringing up the loopback interface of a `--network none` container) was dropped from the allowlist (decision 2026-10-29) and added back on 2026-11-12 after measurement on the real host: the agent container's CapEff `a92405fb` lacked `NET_ADMIN`, and `runc` failed to start any container with a new network namespace (`runc create failed: unable to start container process: error during container init: operation not permitted`). A `runc` spec matrix (`howto/manual_test/agent-exec/diag/runc_matrix.sh`) showed that with the network namespace removed every hardening setting works (user 1001, all capabilities dropped, no-new-privileges, read-only root, noexec tmpfs, masked and read-only paths, cgroup mount, seccomp, memory, CPU and pids limits). It is acceptable because the capability is held by the gVisor Sentry, not by the `agent-host` kernel; it affects only the sandbox's own network stack; the egress gate (agent-net, on `agent-host`) is outside its reach; the agent workload runs as uid 1001 with no-new-privileges and has no effective capability (only root processes inside, `dockerd` and `runc`, use it); and ephemeral containers keep `NetworkMode none` (fetch phase of install: `host`) and `CapDrop ALL`. `NET_RAW` stays absent; no bridge or NAT is used inside, so runsc's `--net-raw` is not needed and the host runtime configuration stays unchanged. The minimum allowlist is not yet measured (open item, shared with USS-002).

## Acceptance criteria

Code (`run`):

- The command runs and the tool exits with the command's exit code.
- `src/` is read-only; env dirs named in the profile are read-only; `build/` is visible read-only to a code run of a profile that declares it, after a `build`.
- Products written to `$OUT` are present in `.output/exec/<execId>/out/` after the run.
- A file passed with `--in data=<path>` from `.input/` is readable under `$IN` and is not otherwise visible.

Build (`build`):

- The driver's exit code is the tool's exit code.
- Build output is present in `build/` after the build.
- A write to `src/`, to any env dir or to `in/` fails.

Install (`install`):

- The install phase's exit code is the tool's exit code; a failed fetch phase exits with the manager's exit code, cause `fetch`, with the stderr message of "Exit codes and causes".
- A package installed is present in the target env dir after the run; `--manifest` is the only way to name packages (no free manager args).
- The fetch phase has network `host`; the install phase has none. No package code is executed in either phase (wheels only; a package without a wheel fails the fetch).
- A write to `src/`, `build/`, `.packages/`, another env dir or `in/` fails; the only writable workspace paths are `dl/` and `out/` (fetch phase) and the target env dir and `out/` (install phase).

Engine authorization plugin:

- As uid 1001, each of these is denied by the engine: `docker run --privileged`, `docker run -u 0`, `docker run -v /:/host`, `docker exec`, `docker cp`, `docker pull`.
- The exec tool's own creates (all three kinds) pass the plugin.
- A create without a body, or with a body violating any invariant, is denied.
- uid 1001 cannot connect to the plugin socket (only `dockerd` does; the agent uses `/var/run/docker.sock`), replace it or signal the plugin process.
- When the plugin is not running, the engine denies every request (fail closed, to verify).
- `docker info` as uid 1001 lists the authorization plugin: deferred follow-up, not part of this story (agent-sandbox verification, see "Engine authorization plugin").

Common:

- A write to `src/`, `build/` (except for the build kind), env dirs (except the install target) or `in/` fails; a write outside the declared rw paths fails.
- `docker inspect` of the ephemeral container, taken before start, shows: capabilities dropped, `no-new-privileges`, default seccomp, read-only root file system, user 1001:1001, the network mode of the kind, the exact mount allowlist of the kind, `Image` equal to the profile's `image_id`, the profile's limits, and no environment variable beyond the image's `ENV` and the allowed set.
- Any network access fails (code, build, install phase of install).
- `--in` with `multiple: true`: staged as `in/<name>/<basename>`; a duplicate basename is exit 125, cause `usage`. `meta.json` lists every input with name, source, path and sha256.
- `meta.json` is written once before start and not changed; `result.json` is `{exit, cause, exec_dir, image_id, start, end}` (install also `phase: fetch|install`).
- `.input/`, `.process/`, `.tools/`, `.agent/`, other exec dirs and `.git` are not visible inside the container; `.output/` shows only the own `in/` and `out/`.
- `stdout.log`, `stderr.log`, `meta.json` and `result.json` are present in the exec dir and are not visible inside the container at all.
- `--exec-id` given: the folder is `/workspace/.output/exec/<name>/`; the same name again exits 125, cause `usage`.
- After any outcome (normal exit, failure, timeout, cancel by SIGTERM or SIGINT) no container `ephemeral-<execId>` (install: also no `ephemeral-<execId>-fetch`) exists. After a `kill -9` of the tool the container is removed by the next tool start or by `sweep`.
- The agent container's settings, network arming and the host are unchanged by the execution.

## Failure cases

| Case | Behaviour |
|---|---|
| Missing or invalid arguments; unknown subcommand or profile; subcommand does not match the profile kind | Exit 125, cause `usage`; nothing is created |
| `--timeout` above the profile's `timeout_s` | Exit 125, cause `usage`; nothing is created |
| `--exec-id` invalid (pattern) or the exec dir already exists | Exit 125, cause `usage`; nothing is created |
| `--in` path outside `/workspace`, unreadable by the agent user, not a regular file (outside `src/`/`build/`), or not matching the profile (undeclared name, `path_under`, `glob`, a required input missing, a name given twice without `multiple`) | Exit 125, cause `usage`; nothing is created |
| `--in` with `multiple: true` stages two files with the same basename for one name | Exit 125, cause `usage`; nothing is created |
| `install` without `--manifest`, or a `--manifest` path outside `src/` or missing; `--manifest` or free manager args on another kind | Exit 125, cause `usage`; nothing is created |
| In-container Docker daemon unreachable (engine socket missing or not reachable) | Exit 125, cause `setup` |
| Profile invalid (unknown field, missing or invalid limit, value above a ceiling, a rule of "Execution profiles"), `image.tar` missing, not loadable or holding another image ID than the profile's `image_id`, `GET /images/<image_id>/json` fails after the load, or an install profile whose target is not `.venv` (the `--manifest` form is open) | Exit 125, cause `setup`; the tool neither pulls nor builds |
| Exec dir cannot be created | Exit 125, cause `setup`; nothing is started |
| Input staging fails (copy error, the 1 GiB total cap exceeded) | Exit 125, cause `setup`; nothing is started |
| A folder agent-host root should have created is missing (`build/`, env dir, `.packages/`) | Exit 125, cause `setup`; nothing is started. The tool never creates them |
| `build/` missing or empty for a code profile with `requires_build: true` | Exit 125, cause `setup`; nothing is started |
| Engine denied a request (authorization plugin) | Exit 125, cause `denied`; a container already created is removed; nothing is started. Indicates a profile or tool setting outside the plugin's invariants (or a plugin ceiling lower than the profile's limits) |
| Plugin not running | The engine fails every request; the tool reports the failure as cause `setup` or `denied` (to verify). `agent-init` restarts the plugin |
| Inspected settings differ from the fixed settings | Remove the container, exit 125, cause `verify`; the command is never started |
| Lock held for more than 2 s, or slot unavailable (summed limits would exceed the bounds of SSR-AGC-012) | Exit 125, cause `busy`; before the exec dir is created, so nothing is created and no `result.json` is written |
| Timeout reached (install: one limit for both phases together) | Kill and remove the container (install: kill the running phase's container, remove both), exit 124, cause `timeout`; output written so far is kept; install: `result.json` records `phase` |
| `out_max` exceeded (growth of the watched folders since the start, see "Exec dir layout") | Kill and remove the container, exit 137, cause `out_cap`; output is kept |
| Memory limit exceeded (OOM kill) | Remove the container, exit 137, cause `oom`; output is kept |
| SIGTERM or SIGINT to the tool | Kill and remove the container (install: kill the running phase's container, remove both); exit 130, cause `cancel`; install: `result.json` records `phase`. The signal is checked between the steps before the start and during the run; a signal during `docker load` or input staging is acted on when that step returns |
| Tool killed (`kill -9`) | The container may remain. The next tool start removes stale `ephemeral.exec` containers whose `ephemeral.pid` no longer exists; `sweep --label <k>=<v>` removes labelled ones (agent-graph runs it at start) |
| Install fetch phase fails (e.g. package without a prebuilt wheel) | Remove both containers, exit with the manager's exit code, cause `fetch`; stderr: `run_ephemeral_container: fetch failed: packages without a prebuilt wheel must be built or downloaded as a wheel on agent-host and copied by the operator to Workspace-Agents/<agentID>/.packages/`; agent-graph surfaces cause and message to the operator; output is kept |
| Command, driver or manager fails (install: the install phase) | Remove the container, exit with its exit code, cause `exit`; output is kept |
| Cleanup / rollback | A normal run removes only the container it created and releases its locks; the exec dir is always kept |

## Open items

- Install network (decided): two phases. The fetch phase has network `host` (the host-side controls of USS-004 still filter it) and downloads wheels only; the install phase is offline. Packages without a prebuilt wheel use the fallback `.packages/`, supplied by the operator. The network of each phase is compiled into the tool as part of the install kind, not chosen by a profile. Conda: `TBD`.
- `--manifest` form for npm and conda (pip: `-r <manifest>`): `TBD`. Today only an install profile with the target `.venv` works; any other target fails with cause `setup`.
- Tool pool: location on agent-host (`TBD`) and how images are built into it (host-side `create_exec_image`, pinning, credscan; `image_id` computed with the same image store and engine version as the agent container).
- Provenance check: compare `tool.json` (pool version, `image_id`, registry digest if any) with the pool record on agent-host (see "Profile trust").
- Limit values are decided provisionally in code (`internal/limits`, one package for the tool and the plugin, so equal): ceilings memory 2 GiB, 2 CPUs, 512 pids, `timeout_s` 3600, `out_max` 1 GiB; tmpfs `/work` 512 MiB, `/tmp` 512 MiB, `/home/run` 128 MiB; staging cap 1 GiB; lock wait 2 s. There are no default limits (a profile states all five). Open: confirm or change these values, and the per-profile values of the real profiles.
- Slot bounds: the tool compiles in 4 GiB, 4 CPUs and 2048 pids as the agent container's bounds (provisional), but `create_agent_container` (`packages/agent-sandbox/internal/config/config.go`) sets 4g, 2 CPUs and 512 pids, and the 2g data-root tmpfs counts against the memory. Open: align the bounds with SSR-AGC-012.
- Slot accounting across concurrent tool processes (no central process): implemented as a read of the engine at start, which two simultaneous starts can race (see "Persistent folders and concurrency"). Open: a race-free form.
- Whether gVisor enforces memory, pids and CPU limits on the inner `runc` container; if not, limits are best-effort.
- In-container daemon settings still to add (`storage-driver`, bounded `log-driver`/`log-opts`, `default-ulimits`, explicit image store `features.containerd-snapshotter`): see "In-container daemon configuration". The data root is the 2g `tmpfs` at `/var/lib/docker`, which counts against the agent container's memory bound.
- Resolved 2026-11-12: `--network none` does not work without `NET_ADMIN` on the agent container (`runc create failed: ... operation not permitted`, CapEff `a92405fb`); `NET_ADMIN` is added back to SSR-AGC-009 (see "Agent container dependency"). Ephemeral containers use `CgroupnsMode host` because gVisor has no cgroup namespaces.
- Multiple read-only binds plus read-write binds (`out/`, `build/`) over the gofer-backed host mount; files written to `out/` must appear on the host owned by 1001.
- Kind code: any command, or a per-profile command allowlist. Today the tool runs any argv the caller gives; the agent-graph side of an isolated step is limited to the executables of `PROCEDURE_ALLOWED_EXECUTABLES` (`python`, `python3`, `/opt/venv/bin/python`; `packages/agent-graph/src/agent_graph/config.py`, checked in `process/validate.py`).
- Build caches (ccache, cargo, go): none persisted in phase 1. Candidate: a read-write cache folder.
- `out_max` alternative: size-limited tmpfs for `out/` copied back after the run (exact cap, counts against the agent container's memory).
- Install-time code execution: mitigated by the no-execution flags of `fetch_argv`/`argv` (`pip --only-binary=:all:`, `npm --ignore-scripts`); conda post-link scripts `TBD`.
- agent-graph: every procedure step runs through the tool. Done for steps with `isolate: true` and a `profile` (`ContainerExec.run_isolated` calls `run_ephemeral_container run`; the isolate checks in `process/validate.py`; products copied from `out/` into the step's work dir and `.outcome` read from `out/`); the agent-graph tests for this have not been run yet. Open: non-isolated steps still run through `SandboxExec` (`dynamic/step_procedure.py`; `isolate` defaults to false, so isolation is opt-in per step, and whether it becomes the default is open, also in USS-008), and the process YAML schema has only `isolate` and `profile` (`run` is a string), not `args`, `manifests` or `inputs`, so `build` and `install` are not reachable from a process. The schema has no `runtime.profiles`, so a step `profile` is not checked against a process-level list; the static check does not look at `image.tar`.
- Unit-aware numeric `params` type (e.g. SPICE suffixes) for the tool kind: open (see "Profile schema extensions").
- Cognitive-step write reach and the agent's own command control: defined in USS-009 (agent tool usage and command control).
- Measured minimum capability allowlist of the agent container (shared with USS-002).
- Confirm the agent workspace form (worktree expected: `.git` is a dangling one-line file). `.git` is unmounted either way.
- Verification spike (runc inside gVisor): the items in section 9 of docs/concept_ephemeral_exec.md (dockerd and runc under runsc, loopback, bind mounts, seccomp, limit enforcement, `docker load` and reload after restart, cleanup after `kill -9`) are not verified yet.
- Real-dockerd verification of the acceptance criteria (tool, plugin and `agent-init` are tested against a fake engine or fake starter only). The profiles and exec images as tool-pool entries are not yet implemented.
- CI build location of the three binaries (today built by hand per `packages/agent-exec/README.md` and staged to `bin/`, howto step P8).
- Profile schema extensions for the tool kind (USS-009): not implemented, see "Profile schema extensions (tool kind, USS-009)".
- Engine authorization plugin follow-ups (the plugin is implemented in `packages/agent-exec`, tested against a fake engine only):
  - `Dockerfile.agent` already has the plugin install, user `authz` (uid: system range, picked at build), the `daemon.json` key and the `ENTRYPOINT` change.
  - SSR-AGC-011: update (group `docker` no longer root-equivalent, plugin dependency); updated separately.
  - agent-sandbox final verification (`s08_verify`): checks for the plugin listed in `docker info` and a denied privileged create as uid 1001.
  - Engine version pin (CVE-2024-41110 fix) and fail-closed behaviour of the pinned engine when the plugin is down: to verify.
  - Confirm the provisional ceilings for `Memory`, `NanoCpus`, `PidsLimit` (see the limits item above). The uid of `authz` is not fixed: it comes from the system range at image build.
