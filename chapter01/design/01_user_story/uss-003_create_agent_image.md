# USS-003 — Create the agent image

USS-003 is a prerequisite of USS-002: `create_agent_container` consumes the image `agent-image:latest` created here. It neither builds nor pulls it.

## Participants

| Participant | Role |
|---|---|
| Developer | Operates `agent-host` (created by USS-001) from host Windows; runs `create_agent_image.exe` |
| Image creation tool | Go executable `create_agent_image.exe`, run from host Windows only (not installed in `agent-host` or `dev-distro`; no Docker Desktop, no Windows-side Docker); reads the build inputs from its current working directory, streams them as an in-memory tar into the Docker build in `agent-host`. Runs `wsl.exe` through `os/exec`; all actions go through `wsl.exe -d agent-host -u root -e <cmd>`; it contacts no other distro |
| Build inputs | `Dockerfile.agent` and `requirements.lock` (hash-pinned lock of the Python packages: `langgraph` and its dependencies, installed with `pip install --require-hashes` into the image venv), regular files in the Windows folder the executable is run from (e.g. `C:\agent-setup`) |
| `agent-host` | WSL2 distro from USS-001; the only distro in the WSL2 VM that runs a Docker engine; builds and holds the image |
| `dev-distro` | Developer's distro: repository checkout, Go and uv (builds the binaries). Not contacted by the tool; must not run Docker (see *Design constraint*) |
| Docker engine | In `agent-host`: builds the image from the streamed build inputs and stores it locally |
| Agent image | Local Docker image `agent-image:latest` from which USS-002 creates agent containers |
| Image content sources | Base image registry (Ubuntu 26.04), `claude` CLI package source (npm), PyPI (`langgraph` and its dependencies), Ubuntu repositories via a frozen snapshot (Node 22, npm, Python 3.14, `docker.io`/`dockerd`, `tini`); reached during the build from `agent-host` |

## Goal

As a developer, I want to create the agent image from Windows, from `Dockerfile.agent`, directly in the Docker engine of `agent-host` by running a single Go executable, so that `create_agent_container` (USS-002) finds the image locally in Docker and every agent container starts from the same, known content.

Scope claim (precise):

- The image defines what is inside the agent sandbox. It does not provide isolation; isolation comes from the `runsc` runtime, the workspace mount and the agent network (USS-002, USS-004).
- The image contains no credentials. The user logs in to the `claude` CLI through the web-based authentication flow (`claude /login`) on first use inside the agent container; the resulting token is created in the container and stored there for the container's life. No credential is injected by these tools, taken from the host or baked into the image.

## Design constraint: one Docker engine, in `agent-host`

- Only `agent-host` runs Docker. `dev-distro` holds only the repository checkout, Go and uv (it builds the binaries) and must not run Docker.
- All WSL2 distros share one VM: one kernel and one network namespace. A second Docker daemon in another distro would share `docker0` and the firewall rules with `agent-host` and could disturb the host-side network controls applied by `create_agent_net arm` (USS-004). The bridges of removed distros also persist in the VM until `wsl --shutdown`.
- Building on Windows itself is impossible: the `RUN` steps need a Linux kernel.

## Invocation

The developer runs, from PowerShell 7.4+ on host Windows (no Docker Desktop, no Windows-side Docker CLI or engine), in the folder that holds the executable and the build inputs:

```
create_agent_image.exe [--tag <tag>]
```

| Argument | Required | Meaning |
|---|---|---|
| `--tag <tag>` | no | Image tag; selects `agent-image:<tag>` instead of the default `agent-image:latest` |

Image name: `agent-image`. Default tag: `latest`. `<tag>` must match the Docker tag grammar `[A-Za-z0-9_.-]{1,128}` and must not start with `.` or `-`. The tool takes no further arguments; any other flag (including `--dev-distro`) is a usage error.

Build inputs: `Dockerfile.agent` and `requirements.lock` must exist as regular files in the current working directory on Windows, the folder the executable is run from (e.g. `C:\agent-setup`). They are copied there from `dist/` together with the binaries; after changing them they must be re-copied. The image is built by the Docker engine of `agent-host`, so it is Linux.

## Build and delivery (Go)

- `create_agent_image.exe` is a native Windows Go executable. Source: `packages/agent-image/cmd/create_agent_image`. Built in `dev-distro` (bash, from `packages/agent-image`) as a windows/amd64 binary into the repo-root `dist/`, like all other executables (see USS-001, *Build in `dev-distro`*): `CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o ../../dist/create_agent_image.exe ./cmd/create_agent_image`. The file is then copied from `dist/` to the Windows folder it is run from.
- It runs on Windows only and is NOT installed into `agent-host`; USS-001 does not deliver it.
- It reads the overwrite answer (y/N) from console stdin.

The tool runs these steps, in order. Every `agent-host` action is `wsl.exe -d agent-host -u root -e <cmd>`; root is needed for the Docker socket, so `agent-operator` needs neither sudo nor `docker` group membership. The first failing step stops the run.

1. Validate the invocation and the build inputs: `[--tag <tag>]` only, tag default `latest`; `Dockerfile.agent` and `requirements.lock` must exist as regular files in the current working directory (symbolic links are followed). On success the tool prints `build context directory: <cwd>`. Failure exits 1 (a missing file is named) with no `wsl.exe` call made.
2. Check prerequisites: run `wsl.exe -d agent-host -u root -e docker version`. If it fails, exit 1 before any build.
3. Check existence in `agent-host`: `wsl.exe -d agent-host -u root -e docker image inspect agent-image:<tag>`. The image is absent only when the engine reports an explicit `no such image` result; any other failure is an engine error (step 2 proved reachability only at that moment) and exits 1 before the prompt and any build.
4. If the image exists, the tool asks `overwrite agent-image:<tag>? [y/N]` before building. Anything but `y`/`Y` exits 0, keeps the existing image and builds nothing.
5. Build as root in `agent-host`: the tool streams an in-memory, deterministic tar of exactly those two files (entries sorted, mode 0644, uid/gid 0, mtime 0) on stdin of `wsl.exe -d agent-host -u root -e docker build -f Dockerfile.agent -t agent-image:<tag> -`. The base image and packages are pulled during this build, so `agent-host` needs internet access. Failure exits 1.
6. Verify: `wsl.exe -d agent-host -u root -e docker image inspect agent-image:<tag>` must succeed. Then check the image metadata (environment, labels, build history) for credentials, without starting a container. A missing image or a finding exits 2 (state UNVERIFIED); the report names the offending item, never the matched value.
7. Report success.

Interrupt (Ctrl-C): checked before each of the 7 steps and after steps 1 to 6. The tool cancels the step in flight, terminates the `wsl.exe` child, starts no later step, reports no success, writes one interrupted line and exits 130, with no rollback. An interrupt that arrives once step 7 has started does not change the result: success and exit 0.

No Windows file is written, nothing is copied into the `agent-host` filesystem and no container is started.

## Preconditions

- USS-001 is complete: `agent-host` exists and Docker is installed and running.
- The tool does NOT create or modify the host Docker `daemon.json` of `agent-host`, install `runsc` or reload/restart the Docker daemon; USS-001 owns the `runsc` installation and registration.
- PowerShell 7.4+ (the developer's shell) is available on the Windows side. No Docker CLI or engine is needed on Windows.
- `Dockerfile.agent` and `requirements.lock` are regular files in the current working directory on Windows (copied from `dist/` together with the binaries; re-copied after a change).
- `dev-distro` is not needed at run time.
- Network access from the build in `agent-host` to the image content sources (internet access of `agent-host`).

## Interaction

1. Developer runs `create_agent_image.exe [--tag <tag>]` from host Windows, in the folder holding the build inputs.
2. Tool validates the invocation and the build inputs, then checks that the Docker engine in `agent-host` is reachable. It reports a mismatch and stops; it never repairs host configuration.
3. Tool checks whether `agent-image:<tag>` exists in `agent-host`.
4. If it does, the tool asks `overwrite agent-image:<tag>? [y/N]` before building; declining ends the run with exit 0 and nothing is built.
5. Tool builds the image in `agent-host` by streaming the build inputs (the two files as an in-memory tar) to `docker build`.
6. Tool confirms with `docker image inspect` in `agent-host` and checks the image metadata for credentials.
7. Tool reports success or failure.

## Image content (summary)

- The `claude` CLI on the session command path, so that a `claude` process can be started in the container (implied by `IP-RENDERER-CHANNEL`, proposed prerequisite). Installed from the npm package `@anthropic-ai/claude-code`, with Node from the Ubuntu repositories.
- A non-root user for the agent workload. The image creates this user with a pinned numeric UID that is the SAME as the host-side `agent-operator` UID in `agent-host` (UID 1001); the container is not created with `--user` (its init runs as root); the agent workload is started with `docker exec -u 1001` (USS-002; uid only, an explicit group would make Docker drop the supplementary group `docker`). This user is a member of the in-container group `docker` (the daemon socket group, see below) to reach `dockerd`. This enables `agent-operator` to handle git from the `agent-host` side; the container mounts the whole per-agent workspace folder at `/workspace`; `src/` inside it is the git worktree whose `.git` is masked with `/dev/null`, so git is unavailable inside; bind-mounted workspace files then have a single owner.
- `CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1` set in the image environment, so the `claude` CLI makes no telemetry, error-report or auto-update calls; only the endpoints allowed by USS-004 are needed.
- The `langgraph` Python package, installed from PyPI into a venv inside the image, which implies a Python runtime in the image (version: `3.14`). `langgraph` and its dependencies are pinned to exact versions with hashes in `requirements.lock` and installed with `pip install --require-hashes`.
- The in-container Docker daemon (`dockerd`) from the Ubuntu `docker.io` package (Ubuntu repository), plus `tini` (from the Ubuntu repository) as the container's init / PID 1, set as the image entrypoint, with `dockerd` as the default command. `tini` runs as root and starts `dockerd` as root; the non-root user (UID 1001) is used for the agent workload, not for the init. The image's `tini` is the init; containers must not be started with Docker `--init`.
- A fixed configuration of the in-container daemon at `/etc/docker/daemon.json` (`root:root`, mode 644), written by `Dockerfile.agent`; this is not the host `daemon.json` of `agent-host`. It holds the settings the ephemeral containers of USS-005 need:
  - `default-runtime: runc` and no other runtime registered; `runc` runs on the gVisor Sentry, so an ephemeral container is not a second gVisor sandbox;
  - `bridge: none`, `iptables: false`, `ip6tables: false` (no bridge, no NAT, no rules inside; matches USS-005);
  - `hosts`: the Unix socket `/var/run/docker.sock` only, no TCP listener; `group: docker`, the in-container group of the agent user (USS-005);
  - `data-root: /var/lib/docker`, inside the agent container's file system, not a host path (SSR-AGC-004);
  - `exec-opts: ["native.cgroupdriver=cgroupfs"]`, since `tini`, not systemd, is PID 1;
  - `no-new-privileges: true` as the default for every inner container; a second layer only, since the agent can override it through the socket (USS-005);
  - `storage-driver`: open item (`overlay2` vs `vfs` under gVisor, USS-005).
- Base image: Ubuntu 26.04 LTS (`resolute`; chosen independently of the `agent-host` distro release, which is also 26.04). Pinning is mandatory: the base image (dated tag and digest), every OS package (Node 22, npm, Python 3.14, `docker.io`, `tini`; apt frozen at an Ubuntu snapshot timestamp, exact `package=version`), the `claude` CLI npm package (checksums of the tarballs stored in the repo) and the Python packages are pinned to exact versions in `Dockerfile.agent` and `requirements.lock`. Pins are refreshed deliberately to pick up security updates. Integrity: every downloaded artefact is verified by checksum before installation, and the build fails when a pin or a check cannot be satisfied (SSR-IMG-004). git is NOT part of the image (git is available on `agent-host` only).
- Mount point of the workspace folder inside the image, also its working directory: `/workspace` (see `IP-AGENT-WORKSPACE-FS`).

After the build the tool confirms only that the image exists and that its metadata carries no credential (step 6); it does not verify the rest of this content, which describes what `Dockerfile.agent` produces.

## Acceptance criteria

- `create_agent_image.exe [--tag <tag>]` exits 0 and `agent-image:<tag>` (default `agent-image:latest`) exists locally in the Docker engine of `agent-host` (visible to `docker image inspect`).
- The image reference equals the one USS-002 expects (`agent-image:latest`) when the default tag is used.
- The image was built by the Docker engine in `agent-host` from the two build inputs streamed from the Windows working directory, and is a Linux image.
- If `agent-image:<tag>` already existed and the developer declined the overwrite prompt, the tool exits 0, the existing image is unchanged and nothing was built.
- The image metadata carries no credential (checked in step 6; a finding exits 2).
- The host `daemon.json` of `agent-host`, the Windows host and the configuration of `dev-distro` are unchanged, by construction (the tool issues only `docker version`, `build`, `image inspect` and `history` in `agent-host`; no before/after comparison is made); the run contacted no distro other than `agent-host`, and no file is written on Windows.
- No container is started by this run.
- The whole image is made available by one run of the Go executable.

## Failure cases

Exit codes: `0` success or overwrite declined; `1` usage, build-input, prerequisite, existence-check or build failure; `2` verification failure (image missing or credential found); `130` interrupted. No rollback.

| Case | Behaviour |
|---|---|
| Invalid `<tag>` or any other flag (including `--dev-distro`) | Usage error, exit 1; nothing is created |
| `Dockerfile.agent` or `requirements.lock` missing or not a regular file in the current working directory | Report naming the missing file, exit 1; no `wsl.exe` call is made |
| `agent-host` (or its Docker engine) unreachable | Report, exit 1 |
| Base image, `claude` CLI, `langgraph` (PyPI) or Ubuntu package (`docker.io`, `tini`, Node, Python) source unreachable (e.g. no internet in `agent-host`), or an install step fails (e.g. Python missing), during the build | Report, exit 1; the Docker error is included |
| Build fails | Report, exit 1; the Docker error is included in the report |
| Existence check fails with anything other than `no such image` | Report with the Docker error, exit 1; no prompt, nothing built, existing image kept |
| Image already exists, overwrite declined | Exit 0; existing image kept; nothing built |
| `docker image inspect` fails after the build | Report failure, exit 2, state UNVERIFIED; the Docker error is included |
| Credential-like name or value in the image environment, labels or history | Report naming the offending item (value not printed), exit 2, state UNVERIFIED; image left in place |
| Interrupt (Ctrl-C) before step 7 begins | One interrupted line, exit 130, state INTERRUPTED; no later step runs; at or before the overwrite prompt nothing is built, during or after the build the image state is whatever the engine left, unverified |
| Cleanup / rollback of partially created resources | None; report and stop |
