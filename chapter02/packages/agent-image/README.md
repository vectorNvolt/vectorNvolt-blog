# agent-image (`create_agent_image`)

Go CLI that builds the `agent-image` Docker image from `Dockerfile.agent` in the Docker engine of `agent-host`, so `create_agent_container` finds `agent-image:latest` locally. It runs as root inside the agent-host VM and calls `docker` directly (there is no other runner). The build context is the current directory: the build files are read from it and sent as a tar on stdin to `docker build`. It never touches `daemon.json`, installs `runsc` or restarts a Docker daemon. The image contains no credentials.

Besides the `claude` CLI, `dockerd`/`tini` and the hash-pinned Python venv (`/opt/venv`, from `requirements.lock`), the image carries the `agent_graph` runtime (USC-008): the package source at `/opt/agent_graph/agent_graph` (root-owned, read-only for the `agent` user), put on the venv's `sys.path` by `/opt/venv/lib/python3.14/site-packages/agent_graph.pth`, and started with `agent_graph ...` (wrapper `/usr/local/bin/agent_graph` = `/opt/venv/bin/python -m agent_graph`). It is not pip-installed (no build backend); its dependencies `langgraph` and `pyyaml` are pinned in `requirements.in`/`requirements.lock`. The build stops if `import agent_graph` or `agent_graph --help` fails. The image also carries three static Go binaries from `packages/agent-exec`, supplied in `bin/` of the current directory: `agent-init` (`/usr/local/sbin/agent-init`, the entrypoint's child under tini; starts the authorization plugin, then `dockerd`), `ephemeral-authz` (`/usr/local/libexec/ephemeral-authz`) and `run_ephemeral_container` (`/usr/local/bin/run_ephemeral_container`, root:root 0755). Spec: `design/02_use_case/usc-003_create_agent_image.yml`.

## Steps

Keep in sync with `internal/steps/sNN_*.go`. Progress is printed as `[n/8]`. Installing the Ubuntu packages runs inside the `docker build` of the base image (step 5, see "Base image"), the rest inside the agent build (step 6). The `docker build` output (plain progress) is shown live on stderr during steps 5 and 6. The first failing step stops the run.

| Step | File | Exit code on failure | Purpose |
|---|---|---|---|
| 1 | `s01_validate_tag.go` | 1 | Tag must match `^[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}$`; `Dockerfile.base`, `Dockerfile.agent` and `requirements.lock` must exist in the current directory, and `agent_graph/` as a directory holding at least `__init__.py` and `__main__.py`; `bin/` must hold `agent-init`, `ephemeral-authz` and `run_ephemeral_container`. A missing one exits 1 before any engine call and names it. |
| 2 | `s02_check_engines.go` | 1 | `docker version` must succeed. |
| 3 | `s03_check_existing.go` | 1 if the check cannot run | `docker image inspect`; the image is absent only when the engine explicitly reports `no such image`. Any other failure (other non-zero exit, `docker` not startable) is an engine error: exit 1, no prompt, nothing built. |
| 4 | `s04_confirm_overwrite.go` | 0 if declined | If the image exists, prompt `overwrite agent-image:<tag>? [y/N]` before building; anything but `y`/`Y` keeps the old image, builds nothing and exits 0. |
| 5 | `s05_ensure_base.go` | 1 (exit_state `NOT_CREATED`) | Ensure `agent-base:<hash>` exists: reuse, `docker load` of the archive, or `docker build` of `Dockerfile.base`. See "Base image". |
| 6 | `s06_build_image.go` | 1 | `docker build` of `agent-image:<tag>` with `--build-arg BASE_IMAGE=agent-base:<hash>`; the build context is a tar of the build files and the selected `agent_graph/` files, sent on stdin (`docker build -f Dockerfile.agent -`). |
| 7 | `s07_verify_image.go` | 2 | `docker image inspect` must succeed. Then a credential check without starting a container: `docker image inspect` (Config.Env, Config.Labels) and `docker history --no-trunc` must show no env var named like `TOKEN`/`SECRET`/`PASSWORD`/`PASSWD`/`API_KEY`/`APIKEY`/`CREDENTIAL`/`PRIVATE_KEY` and no `ghp_`, `gho_`, `ghs_`, `github_pat_`, `sk-ant-`, `AKIA...` or private-key block in any env value, label value or history line. A finding (or an engine error during the check) exits 2 `UNVERIFIED`; the message names the env var or history step, never the matched value. |
| 8 | `s08_report.go` | none (exit 0) | Print the success report. |

No rollback.

Ctrl-C at the overwrite prompt is neither a yes nor a no: nothing is built and the existing image is kept. Ctrl-C before the build leaves everything unchanged. Ctrl-C during or after the build (steps 5 to 7) leaves `agent-image:<tag>` as the engine left it (old image kept, replaced, or none), unverified, with no cleanup.

## How to build / run

Inside the foundation VM, from the repo root (Go is in `/usr/local/go`; the build is native):

```
export PATH="$PATH:/usr/local/go/bin"
mkdir -p dist
(cd packages/agent-image && CGO_ENABLED=0 go build -o ../../dist/create_agent_image ./cmd/create_agent_image)
```

Run it as root from the directory that holds `Dockerfile.base`, `Dockerfile.agent`, `requirements.lock`, `agent_graph/` and `bin/` (in the howto: `dist/`, assembled from the repo checkout):

```
cd dist
sudo ./create_agent_image [--tag <tag>] [--rebuild-base]
```

`bin/` holds the three Linux binaries `agent-init`, `ephemeral-authz` and `run_ephemeral_container`; they go into the build context tar with mode 0755. Build them from `packages/agent-exec` (`CGO_ENABLED=0 go build -o <dir>/bin/<name> ./cmd/<name>`, see the howto, steps 2 and 3) into `bin/`. `agent_graph/` is a copy of `packages/agent-graph/src/agent_graph`; re-copy it after changing the package. Only its `*.py` files (any depth) and `schemas/*.yml` go into the build context; `__pycache__/` and `*.egg-info/` are skipped and every other file is left out. A missing file or directory exits 1 and names it and the directory checked.

Default tag is `latest`. `create_agent_container` only consumes `agent-image:latest`.

## Base image

The Ubuntu base digest and the snapshot-pinned apt installs (`apt-get --snapshot`, 10 retries, max delay 60 s) live in `Dockerfile.base`, not in `Dockerfile.agent`. `Dockerfile.agent` starts with `ARG BASE_IMAGE` / `FROM ${BASE_IMAGE}`.

- Tag: `agent-base:<hash>`, `<hash>` = first 16 hex characters of the SHA-256 of `Dockerfile.base` (CRLF normalized to LF). The image is built with the label `vectornvolt.agent-base.hash=<hash>`. Any change to `Dockerfile.base` (package pins, snapshot date, base digest) changes the tag and so triggers a new base build.
- `Dockerfile.base` must be in the current directory. Missing or empty, step 1 exits 1 before any engine call: `build file Dockerfile.base not found as a non-empty regular file in <dir>`. It is not sent in the agent build context.
- Step 5 (after the overwrite confirmation, before the agent build):
  1. `docker image inspect --format {{json .Config.Labels}} agent-base:<hash>`: the image exists and carries `vectornvolt.agent-base.hash=<hash>`: reuse it, no network access to `snapshot.ubuntu.com` (`base image agent-base:<hash> present`). An existing tag with a wrong label counts as absent.
  2. Else `agent-base-<hash>.tar` is in the current directory: `docker load` with the archive on stdin (`base image loaded from agent-base-<hash>.tar`).
  3. Else `docker build --progress=plain -f Dockerfile.base --label vectornvolt.agent-base.hash=<hash> -t agent-base:<hash> -` with a tar holding only `Dockerfile.base` on stdin (`building base image agent-base:<hash> (needs snapshot.ubuntu.com)`, then `base image agent-base:<hash> built`). A `503 Service Unavailable` from the snapshot service is an upstream outage: wait and rerun, nothing to clean up.
- Failures of step 5 exit 1 with exit_state `NOT_CREATED`:
  - Base build failed: `docker build of the base image agent-base:<hash> failed: ...`, then `the base build needs snapshot.ubuntu.com; run again later to retry the agent build, or load the base from agent-base-<hash>.tar in this folder`.
  - `docker load of <archive> failed`.
  - Label missing or wrong after the load or build: `base image ... is missing or does not carry label vectornvolt.agent-base.hash=<hash> after docker load of <archive>` (or `after docker build`).
- `--rebuild-base` skips cases 1 and 2 and forces case 3 even if the image exists.
- Step 6 then runs `docker build --progress=plain -f Dockerfile.agent --build-arg BASE_IMAGE=agent-base:<hash> -t agent-image:<tag> -`.
- The tool never writes the archive and never deletes images (old `agent-base:*` images stay until removed by hand).
- Optional archive, so a recreated VM needs no network access to the snapshot service for the base. Save it once, keep it outside the VM (for example `limactl copy` or `multipass transfer`, see the howto platform sections), and put it back into the build directory before the next run (inferred, not in repo; the howto does not cover it):

  ```
  sudo docker image ls agent-base                                   # find <hash>
  sudo docker save -o agent-base-<hash>.tar agent-base:<hash>
  ```

  A tar whose hash differs from the current `Dockerfile.base` is not used for it; the label is verified after the load.

| Exit code | Meaning |
|---|---|
| 0 | Image built and verified, or overwrite declined (existing image unchanged) |
| 1 | Usage error or prerequisite/build failure (steps 1-6) |
| 2 | Runtime failure (step 7: image not inspectable after the build, or credential check failed or could not run) |
| 130 | Interrupted (Ctrl-C) at any step: the in-flight `docker` command is terminated, no later step runs, no success is reported; one line on stderr names the step (exit_state INTERRUPTED) |

## Configuration

There are no tunables; everything is fixed in `internal/config/config.go`:

| Item | Value |
|---|---|
| Build files (current directory) | `Dockerfile.agent`, `requirements.lock` |
| Base file (current directory; hashed, not in the build context) | `Dockerfile.base` |
| Optional base archive (current directory) | `agent-base-<hash>.tar` |
| Package directory (current directory) | `agent_graph/` (`*.py`, `schemas/*.yml`; must hold `__init__.py`, `__main__.py`) |
| Binaries (current directory) | `bin/agent-init`, `bin/ephemeral-authz`, `bin/run_ephemeral_container` (tar mode 0755) |

## Tests

```
go test ./...
```

## Layout

| Here | Role |
|---|---|
| `cmd/create_agent_image/` | entry point |
| `internal/config/` | constants |
| `internal/services/` (`runner`, `host`, `docker`, `buildcontext`, `render`) | direct command runner, docker adapter, build-context tar, console output |
| `internal/steps/` | one file per step |
| `internal/sequence/` | numbered step list, first failure sets the exit code |
| `tests/` | sequence tests |
| `Dockerfile.base` | apt/snapshot/base-digest layer, built as `agent-base:<hash>` |
| `Dockerfile.agent`, `requirements.lock` | image definition and build inputs (copied next to the binary to run it, together with `agent_graph/` from `packages/agent-graph/src`) |

## Refreshing the pins

`Dockerfile.base` and `Dockerfile.agent` together pin every component to an exact version and check every download by hash (SSR-IMG-004). Base: Ubuntu 26.04 `resolute`, Python 3.14, Node.js 22 and npm from Ubuntu. The apt, snapshot and base-digest pins (steps 1 and 2) are edited in `Dockerfile.base`; changing it changes the `agent-base` tag, so the next run builds a new base (needs `snapshot.ubuntu.com`). The claude CLI pin (step 3) is in `Dockerfile.agent`; the Python pins (step 4) are in `requirements.in`/`requirements.lock`. To move the pins forward, change all of them together and build once:

1. Base image (`Dockerfile.base`): pick the newest dated tag (`resolute-YYYYMMDD[.N]`) and take the `Docker-Content-Digest` of its OCI index (`docker buildx imagetools inspect ubuntu:<tag>`, or a registry `HEAD` on `/v2/library/ubuntu/manifests/<tag>` with `Accept: application/vnd.oci.image.index.v1+json`). Update the `FROM` line (tag and `@sha256`).
2. Ubuntu packages (`Dockerfile.base`): set `UBUNTU_SNAPSHOT` to a new snapshot ID (`YYYYMMDDTHHMMSSZ`, UTC). For each package in the `--snapshot` install, take the highest version across `resolute`, `resolute-updates` and `resolute-security` (main and universe, amd64) from `https://snapshot.ubuntu.com/ubuntu/<ID>/dists/<suite>/<component>/binary-amd64/Packages.xz` and update its `=version`. The bootstrap pins (`ca-certificates`, `openssl` from the release pocket) only change with a new Ubuntu release.
3. claude CLI (`Dockerfile.agent`): set `CLAUDE_CODE_VERSION` (`npm view @anthropic-ai/claude-code version`). Download `@anthropic-ai/claude-code`, `@anthropic-ai/claude-code-linux-x64` and `@anthropic-ai/claude-code-linux-arm64` at that version from `registry.npmjs.org`, run `sha512sum` on all three tarballs (each must match `npm view <pkg>@<version> dist.integrity`, converted to hex) and replace the wrapper checksum and both native checksums (x64 and arm64) in the claude CLI step.
4. Python packages: edit `requirements.in`, delete `requirements.lock` (uv keeps the pins of an existing output file) and run, from this directory:

   ```
   uv pip compile requirements.in --python-version 3.14 --python-platform x86_64-manylinux_2_39 --generate-hashes -o requirements.lock
   ```

   The build installs with `--require-hashes --only-binary :all:`, so every locked package needs a wheel for CPython 3.14 on manylinux x86_64.

## Known gaps: arm64

The foundation VM supports amd64 and arm64. `Dockerfile.agent` selects the claude CLI native package by `dpkg --print-architecture` and pins both (`claude-code-linux-x64` and `claude-code-linux-arm64`), so that step is no longer a gap. Still open, so the agent image is not verified on arm64:

- `requirements.lock` is compiled for `x86_64-manylinux_2_39`; the arm64 wheel hashes are not in it and unverified.
- The `Dockerfile.base` digest must be the multi-arch index digest of the Ubuntu tag (unverified).
