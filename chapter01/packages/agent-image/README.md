# agent-image (`create_agent_image`)

Go CLI that builds the `agent-image` Docker image from `Dockerfile.agent` directly in the Docker engine of `agent-host`, so `create_agent_container` finds `agent-image:latest` locally. It runs on Windows only (PowerShell 7.4+) and is not installed into `agent-host`; every action is a `wsl.exe -d agent-host -u root -e <cmd>` call. No other WSL distro is contacted: all WSL2 distros share one VM network stack, so only `agent-host` may run a Docker daemon. The build context is sent as a tar on stdin (no Windows temp file, nothing copied into `agent-host`). It never touches `daemon.json`, installs `runsc` or restarts a Docker daemon. The image contains no credentials. Spec: `design/02_use_case/usc-003_create_agent_image.yml`.

## Steps

Keep in sync with `internal/steps/sNN_*.go`. Pulling the base image and installing packages run inside the `docker build` of step 5. The `docker build` output (plain progress) is shown live on stderr during step 5. The first failing step stops the run.

| Step | File | Exit code on failure | Purpose |
|---|---|---|---|
| 1 | `s01_validate_tag.go` | 1 | Tag must match `^[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}$`; `Dockerfile.agent` and `requirements.lock` must exist in the current directory. |
| 2 | `s02_check_engines.go` | 1 | `docker version` must succeed in `agent-host`. |
| 3 | `s03_check_existing.go` | 1 if the check cannot run | `docker image inspect` in `agent-host`; the image is absent only when the engine explicitly reports `no such image`. Any other failure (other non-zero exit, `wsl.exe` error) is an engine error: exit 1, no prompt, nothing built. |
| 4 | `s04_confirm_overwrite.go` | 0 if declined | If the image exists, prompt `overwrite agent-image:<tag>? [y/N]` before building; anything but `y`/`Y` keeps the old image, builds nothing and exits 0. |
| 5 | `s05_build_image.go` | 1 | `docker build` of `agent-image:<tag>` in `agent-host`; the build context is a tar of the build files, sent on stdin (`docker build -f Dockerfile.agent -`). |
| 6 | `s06_verify_image.go` | 2 | `docker image inspect` in `agent-host` must succeed. Then a credential check without starting a container: `docker image inspect` (Config.Env, Config.Labels) and `docker history --no-trunc` must show no env var named like `TOKEN`/`SECRET`/`PASSWORD`/`PASSWD`/`API_KEY`/`APIKEY`/`CREDENTIAL`/`PRIVATE_KEY` and no `ghp_`, `gho_`, `ghs_`, `github_pat_`, `sk-ant-`, `AKIA...` or private-key block in any env value, label value or history line. A finding (or an engine error during the check) exits 2 `UNVERIFIED`; the message names the env var or history step, never the matched value. |
| 7 | `s07_report.go` | none (exit 0) | Print the success report. |

No rollback.

Ctrl-C at the overwrite prompt is neither a yes nor a no: nothing is built and the existing image is kept. Ctrl-C before the build leaves everything unchanged. Ctrl-C during or after the build (steps 5, 6) leaves `agent-image:<tag>` as the engine left it (old image kept, replaced, or none), unverified, with no cleanup.

## How to build / run

From `packages\agent-image` on Windows:

```
go build -o dist\create_agent_image.exe .\cmd\create_agent_image
```

```
.\dist\create_agent_image.exe [--tag <tag>]
```

Run it from the directory that holds `create_agent_image.exe`, `Dockerfile.agent` and `requirements.lock` (e.g. `C:\agent-setup`): the build inputs are read from the current directory. A missing file exits 1 and names the file and the directory checked.

Default tag is `latest`. `create_agent_container` only consumes `agent-image:latest`.

| Exit code | Meaning |
|---|---|
| 0 | Image built and verified, or overwrite declined (existing image unchanged) |
| 1 | Usage error or prerequisite/build failure (steps 1-5) |
| 2 | Runtime failure (step 6: image not inspectable after the build, or credential check failed or could not run) |
| 130 | Interrupted (Ctrl-C) at any step: the in-flight wsl.exe is terminated, no later step runs, no success is reported; one line on stderr names the step (exit_state INTERRUPTED) |

## Configuration

There are no tunables; everything is fixed in `internal/config/config.go`:

| Item | Value |
|---|---|
| Build files (current directory on Windows) | `Dockerfile.agent`, `requirements.lock` |

## Tests

```
go test ./...
```

## Layout

| Here | Role |
|---|---|
| `cmd/create_agent_image/` | entry point |
| `internal/config/` | constants |
| `internal/services/` (`wsl`, `docker`, `buildcontext`, `render`) | `wsl.exe` runner, docker adapter, build-context tar, console output |
| `internal/steps/` | one file per step |
| `internal/sequence/` | numbered step list, first failure sets the exit code |
| `tests/` | sequence tests |
| `Dockerfile.agent`, `requirements.lock` | image definition and build inputs (copied next to the exe to run it) |

## Refreshing the pins

`Dockerfile.agent` pins every component to an exact version and checks every download by hash (SSR-IMG-004). Base: Ubuntu 26.04 `resolute`, Python 3.14, Node.js 22 and npm from Ubuntu. To move the pins forward, change all of them together and build once:

1. Base image: pick the newest dated tag (`resolute-YYYYMMDD[.N]`) and take the `Docker-Content-Digest` of its OCI index (`docker buildx imagetools inspect ubuntu:<tag>`, or a registry `HEAD` on `/v2/library/ubuntu/manifests/<tag>` with `Accept: application/vnd.oci.image.index.v1+json`). Update the `FROM` line (tag and `@sha256`).
2. Ubuntu packages: set `UBUNTU_SNAPSHOT` to a new snapshot ID (`YYYYMMDDTHHMMSSZ`, UTC). For each package in the `--snapshot` install, take the highest version across `resolute`, `resolute-updates` and `resolute-security` (main and universe, amd64) from `https://snapshot.ubuntu.com/ubuntu/<ID>/dists/<suite>/<component>/binary-amd64/Packages.xz` and update its `=version`. The bootstrap pins (`ca-certificates`, `openssl` from the release pocket) only change with a new Ubuntu release.
3. claude CLI: set `CLAUDE_CODE_VERSION` (`npm view @anthropic-ai/claude-code version`). Download `@anthropic-ai/claude-code` and `@anthropic-ai/claude-code-linux-x64` at that version from `registry.npmjs.org`, run `sha512sum` on both tarballs (it must match `npm view <pkg>@<version> dist.integrity`) and replace both checksums in the claude CLI step.
4. Python packages: edit `requirements.in`, delete `requirements.lock` (uv keeps the pins of an existing output file) and run, from this directory:

   ```
   uv pip compile requirements.in --python-version 3.14 --python-platform x86_64-manylinux_2_39 --generate-hashes -o requirements.lock
   ```

   The build installs with `--require-hashes --only-binary :all:`, so every locked package needs a wheel for CPython 3.14 on manylinux x86_64.
