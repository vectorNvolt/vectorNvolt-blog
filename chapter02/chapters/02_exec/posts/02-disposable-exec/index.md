---
title: "Stop Letting Agents Pollute Your Runtime: Isolating Code and Tool Execution in Disposable Containers #ch02p02"
type: dev
code_tag: ch01-ch02/publication
code_package: agent-exec
code_url: https://github.com/vectorNvolt/vectorNvolt-blog/tree/ch01-ch02/publication/chapter02/packages/agent-exec
slug: exec-disposable-containers
status: published
tags: [exec, isolation, containers, go]
custom_excerpt: "I build a Go execution tool that runs agent code or each tool call in its own disposable container. The caller supplies data, never commands, and a separate authorization plugin decides what the container engine accepts."
feature_image:
links:
  howto: https://github.com/vectorNvolt/vectorNvolt-blog/blob/ch01-ch02/publication/chapter02/howto/README.md
---


<!--kg-card-begin: html-->
<style>
details.sec-more {
  margin: 0.8em 0 1.6em;
}
details.sec-more summary {
  display: inline-block;
  cursor: pointer;
  list-style: none;
  padding: 0.35em 0.95em;
  font-size: 0.85em;
  font-weight: 600;
  color: #2e86c1;
  border: 1px solid color-mix(in srgb, #2e86c1 40%, transparent);
  border-radius: 999px;
  background: color-mix(in srgb, #2e86c1 8%, transparent);
  transition: background 0.15s ease;
}
details.sec-more summary:hover {
  background: color-mix(in srgb, #2e86c1 18%, transparent);
}
details.sec-more summary::-webkit-details-marker {
  display: none;
}
details.sec-more summary::before {
  content: "▸";
  display: inline-block;
  margin-right: 0.5em;
  transition: transform 0.15s ease;
}
details.sec-more[open] summary::before {
  transform: rotate(90deg);
}
details.sec-more[open] .sec-more-show,
details.sec-more:not([open]) .sec-more-hide {
  display: none;
}
/* One line per section: add every new #more-<id> here. */
#more-decisions:not([open]) ~ :not(h2, p:has(> a[id]:empty:only-child), #more-decisions ~ h2 ~ *),
#more-arch:not([open]) ~ :not(h2, p:has(> a[id]:empty:only-child), #more-arch ~ h2 ~ *),
#more-life:not([open]) ~ :not(h2, p:has(> a[id]:empty:only-child), #more-life ~ h2 ~ *),
#more-settings:not([open]) ~ :not(h2, p:has(> a[id]:empty:only-child), #more-settings ~ h2 ~ *),
#more-policy:not([open]) ~ :not(h2, p:has(> a[id]:empty:only-child), #more-policy ~ h2 ~ *),
#more-verify:not([open]) ~ :not(h2, p:has(> a[id]:empty:only-child), #more-verify ~ h2 ~ *),
#more-change:not([open]) ~ :not(h2, p:has(> a[id]:empty:only-child), #more-change ~ h2 ~ *),
#more-scope:not([open]) ~ :not(h2, p:has(> a[id]:empty:only-child), #more-scope ~ h2 ~ *),
#more-code:not([open]) ~ :not(h2, p:has(> a[id]:empty:only-child), #more-code ~ h2 ~ *),
#more-refs:not([open]) ~ :not(h2, p:has(> a[id]:empty:only-child), #more-refs ~ h2 ~ *) {
  display: none;
}

/* ---- Inline code --------------------------------------------------------
   The theme's light chip stays light in dark mode. A tint of the text colour
   works in both modes; !important because the theme's selector is unknown. */
:not(pre) > code {
  background: color-mix(in srgb, currentColor 10%, transparent) !important;
}
</style>
<!--kg-card-end: html-->


## Why execution does not belong on the agent's runtime

Giving an AI agent the ability to write and execute code turns a passive chatbot into an autonomous worker—and turns every prompt into a potential production incident.

For most engineering teams, this is where promising AI prototypes stall before reaching production. To ship fast, teams initially run agent-generated commands directly on the application host, inside a shared CI runner, or in a long-lived sandbox container. Security and platform leads rightly block those designs from production because they introduce four unacceptable business risks:

- **Unreproducible failures:** Leftover files and package drift make debugging non-deterministic.
- **Unbounded blast radius:** A hallucinated or malicious command can corrupt shared infrastructure.
- **Credential exposure:** Unreviewed code running in the agent's environment can read environment variables, API keys, or mounted secrets.
- **Compliance and audit blind spots:** Without deterministic logs and exit states, reconstructing what happened after an incident is guesswork.

Moving an agent safely into production requires treating every tool invocation and code execution as untrusted, disposable work. Specifically, the execution layer must enforce three guarantees: every run starts clean and vanishes afterward, the model passes structured data rather than raw shell commands, and resource and security limits are enforced *outside* the caller's process.

Below are the design decisions, the architecture and the Go implementation of **agent-exec**, a reference runner I designed around those rules. It is unit-tested against an in-process mock engine and not yet run against a live daemon.

## My five core design decisions

Each architectural decision trades runtime overhead or upfront configuration for a smaller attack surface.

### 1. One disposable container per execution

When an agent writes a script, runs it, and then runs a second one, the second run should not see the first run's files or installed packages. Every invocation starts a fresh container with `AutoRemove`, and `cleanup` explicitly deletes it afterward. Execution directories are never recycled—reusing an `--exec-id` immediately fails as a usage error (`TestExecDirIsNeverReused`).

- **Alternative rejected:** Running a subprocess or reusing a long-lived sandbox container. State leaks between runs, cleanup relies on untrusted code behaving well, and one compromised step can inspect the next step's files.
- **Why this works:** Nothing survives a run, so isolation never depends on post-run cleanup.
- **Cost accepted:** Container creation and teardown overhead on every single tool call.

### 2. Data, not commands

When the model needs to run a tool such as a circuit simulator, it should choose which files and parameters to use, not which command line runs. For the `tool` kind, an operator-authored profile pins `argv`, allowed input paths, and typed parameters. The caller passes only `--in <name>=<path>` and `--param <name>=<value>`, and `ExpandToolArgv` substitutes strictly `{out}`, `{in.<name>}`, and `{param.<name>}`.

- **Alternative rejected:** Allowing the agent to pass `argv`, container images, or volume mounts, which would let the model define its own isolation boundary.
- **Why this works:** The model selects input data and parameters, never execution primitives.
- **Cost accepted:** Every new tool requires an operator-written profile before the agent can invoke it.

### 3. Policy enforced outside the caller

A bug in the agent-side client should not be enough to start a container with more privilege than the operator allowed. The `ephemeral-authz` Docker authorization plugin is default-deny, and the resource ceilings defined in `internal/limits` are compiled into both the plugin and the client.

- **Alternative rejected:** Enforcing security policy solely inside the client. A bug or compromise in the caller would widen what the container daemon accepts.
- **Why this works:** The design intent is that the daemon rejects any request outside policy, whoever sends it. The contract test covers the payloads the client generates (Unit-tested). I have not shown it on a live daemon, and the plugin only sees what the engine hook passes it.
- **Cost accepted:** Deploying and maintaining a second binary kept in sync with the client.

### 4. No network access by default

Generated code that tries to upload a file or call a remote server should find no network to do it with. `kind.Network` defaults to `none`. Host networking (`host`) is granted strictly during the fetch phase of `install` runs.

- **Alternative rejected:** Enabling networking with an egress allow-list, which creates an ongoing policy surface to maintain and potentially misconfigure.
- **Why this works:** The safe default (`none`) leaves no allow-list to maintain.
- **Cost accepted:** Any tool requiring external dependencies must fetch them upfront during an `install` run.

### 5. The result is a file

After a run, the agent, the operator and a later reviewer need one place to see what happened, even if the caller crashed. `result.json` serves as the single canonical record of every run, with exit codes mapped deterministically to failure causes: `125` for `usage`, `setup`, `denied`, `policy`, `verify`, or `busy`; `124` for `timeout`; `130` for `cancel`; and `137` for `out_cap` or `oom`.

- **Alternative rejected:** Returning execution results solely via stdout in the caller's reply.
- **Why this works:** A file on disk survives a caller crash and leaves a deterministic audit artifact.
- **Cost accepted:** Callers must read output artifacts from disk, and raw `stdout.log` and `stderr.log` files are currently not size-capped. This is a known gap (Code-read).

## The architecture: implement trust boundary around zero-trust execution

The system is split across a strict trust boundary: the untrusted agent runs as an unprivileged user (`uid 1001`), while `agent-init` (supervised by `tini`, which is PID 1) and `ephemeral-authz` form a privileged control plane around `dockerd`. The design intends the agent process not to be able to bypass it; I have not verified that on a live engine.

Click the numbered circles in the diagram to inspect the components and request flow.

{{island:exec-arch}}

<!--kg-card-begin: html-->
<details class="sec-more" id="more-arch"><summary><span class="sec-more-show">Show the full section</span><span class="sec-more-hide">Hide the section</span></summary></details>
<!--kg-card-end: html-->

- **Three binaries:**
  - `run_ephemeral_container`: Runs inside the agent environment as `uid 1001`, exposing the subcommands `run`, `build`, `install`, `tool`, `list`, and `sweep`.
  - `ephemeral-authz`: The Docker authorization plugin listening on inherited file descriptor `3`.
  - `agent-init`: The supervisor that `tini` (PID 1) starts as its child. It binds the plugin socket with mode `0600`, starts `ephemeral-authz` as user `authz`, launches `dockerd`, restarts the plugin after a 1-second backoff if it exits, and forwards `SIGTERM` and `SIGINT` to `dockerd`.
- **Direct Engine API, no CLI:** The runner speaks directly to the Docker Engine API over a Unix socket using Go's `net/http` and never invokes the `docker` CLI binary.
- **Internal packages:** `cli`, `profile`, `limits`, `kind` (mounts, network, and container create payloads), `wsfs` (all `/workspace` filesystem access), `sequence`, `engine` (the HTTP client and the `Engine` interface mocked in tests), `inputpolicy`, `authz`, `agentinit`, and `app` (holds `Main`).

## Execution lifecycle, step by step

`internal/sequence` enforces a fixed five-phase lifecycle. Validation and filesystem locking occur *before* any execution directories or containers are allocated, and event listeners attach *before* container start so short-lived processes never lose output.

Click the steps in the diagram for details.

<!--kg-card-begin: html-->
<div id="exec-life-seq"></div>
<!--kg-card-end: html-->

<!--kg-card-begin: html-->
<details class="sec-more" id="more-life"><summary><span class="sec-more-show">Show the full section</span><span class="sec-more-hide">Hide the section</span></summary></details>
<!--kg-card-end: html-->

1. **Validate:** `Execute` checks the invocation against the tool profile: execution kind, timeout bounds, declared inputs under allowed folders, and typed parameters. Usage and policy violations exit before creating any directories or containers (`TestUsageErrorsCreateNothing`, `TestToolInputPolicyRefusesBeforeExecDir`).
2. **Prepare:** `prepare` pings the engine. `removeStale` cleans up containers whose `ephemeral.pid` process is no longer alive. `checkFolders` verifies that required workspace folders exist. `takeLocks` acquires `flock` locks—exclusive on read-write folders and shared on read-only folders—with a bounded wait before returning `busy`. `checkSlot` sums active resource limits against the agent container's bounds, and `CreateExecDir` runs last.
3. **Execute:** `stage` copies inputs outside `src/` into `in/`. `ensureImage` loads `image.tar` only if the engine lacks the image, never pulling from a registry or building on the fly. `createAll` creates containers without starting them, `verifyAll` inspects the created state, and `writeMeta` writes `meta.json` with `O_EXCL`.
4. **Run:** `startAndWait` subscribes to engine events, attaches `ContainerWait` and `ContainerAttach`, and only then calls `ContainerStart`. It monitors the execution deadline, cancellation channel, and an `out_max` disk poll, sending `SIGKILL` via `ContainerKill` if any threshold fires.
5. **Finish:** `cleanup` removes the containers, filesystem locks are released, and `writeResult` writes `result.json` once.

## Container sandbox and runtime hardening

Rather than letting tool profiles assemble arbitrary container configurations, `kind.NewPlan` hardcodes a locked-down container spec in Go and restricts profiles to selecting bounded parameters within it.

- **Process isolation:** Runs as `1001:1001` with `CapDrop: ALL`, `no-new-privileges`, `ReadonlyRootfs`, an empty entrypoint, `WorkingDir: /work`, a private IPC namespace, and a fixed environment.
- **Scratch space and mount allowlist:** Size-capped `tmpfs` mounts on `/work`, `/tmp` (`noexec` unless `tmp_exec` is set), and `/home/run`, all mounted `nosuid,nodev`. For kind `tool`, bind mounts are restricted to `src/` (read-only), the run's own `in/` (read-only), its own `out/` (read-write), and `build/` (read-only, only when `build_dir` is enabled).
- **Resource ceilings:** Compiled into `internal/limits` at `2 GiB` memory, `2` CPUs, `512` PIDs, `3600 s` max runtime, and `1 GiB` for `out_max` (marked provisional pending load testing).
- **Labels and cgroups:** Containers carry `ephemeral.exec` and `ephemeral.pid` labels, and the CLI rejects a caller-supplied `--label ephemeral.*` with a usage error (exit 125) before planning. `kind.NewPlan` also drops such labels as a backstop. `CgroupnsMode` is set to `host` because gVisor (`runsc`) does not support cgroup namespaces.
- **Verify before start:** The runner creates the container, inspects the resulting daemon object, and runs `kind.Verify`. If daemon defaults or mutations cause configuration drift, the run aborts with cause `verify` before the container ever starts (`TestVerifyMismatch`).
- **Domain input filtering:** As a reference example for domain-specific workloads, `spice-safe` in `inputpolicy` rejects SPICE circuit netlists containing `.control` blocks with `shell`, `source`, or `codemodel`, as well as `include` or `lib` targets outside declared inputs (implemented as a lexical word filter rather than a full SPICE parser).
- **Safe workspace traversal:** `wsfs` opens host workspace paths via `openat2` with `RESOLVE_BENEATH`, `RESOLVE_NO_SYMLINKS`, and `RESOLVE_NO_MAGICLINKS`, falling back to an `O_NOFOLLOW` component-by-component walk when `openat2` is unavailable.

**Trade-offs:** Create, inspect, verify, and remove execute on every invocation. I have not benchmarked that latency on a live daemon yet, so the overhead is expected rather than measured. A warm container pool would reduce startup latency, but at the cost of state reuse—which violates the core isolation guarantee. The create-then-verify double check adds one inspect round-trip per call in exchange for guaranteeing a drifted container never starts, and compiling resource ceilings into both binaries means raising a limit requires rebuilding and redeploying both.

## Why the policy sits in a plugin - defense-in-depth

Because the client processes untrusted caller inputs, relying solely on client-side checks violates least privilege. Moving enforcement into `ephemeral-authz` isolates policy decisions behind the Docker daemon's authorization hook.

- **Default-deny endpoints:** `internal/authz` allows only explicitly listed routes (ping, image load and inspect, container create, start, attach, wait, kill, remove, events, and select read endpoints). `exec`, `commit`, `build`, networks, and volumes are denied.
- **Create payload inspection:** Rejects any container creation request specifying `Binds`, `Devices`, or `Privileged`. `Runtime` must be empty or `runc`. Memory, CPUs, and PIDs must fall within `internal/limits` ceilings, the container name and `ephemeral.exec` label must carry the same execution ID, and every path component of a bind mount source is checked for symlinks.
- **Contract-tested against the client:** `TestToolBodiesAreAllowedByPlugin` asserts that `ephemeral-authz` accepts payloads generated by `kind.NewPlan` and rejects the exact same payloads if `Privileged` is toggled on.
- **Intended effect:** A bug or compromise inside the caller process should not widen the set of operations `dockerd` will execute. This is design intent, covered only by the contract test against the client payloads (Unit-tested). It is not a property I have shown on a live daemon, and the plugin only sees what the engine hook passes it.

**Trade-off:** Deploying a second component introduces operational surface area. The plugin and client stay aligned because both import `internal/limits`, enforced by a contract test on the create payload—a cost worth paying over trusting the caller.

## What to take away

1. The isolation is meant to come from fixing the container body in code and the policy in a separate component, not from trusting the caller.
2. Each of the five decisions has a cost I named and accepted.
3. What is proven today is control flow against a fake engine. The claims about runtime behaviour wait for the real-engine run.

## What would change my mind

These are conditions, not findings. None has been observed.

- Tools that need the network at scale: I would move to an egress proxy instead of a per-kind network setting.
- Multi-tenant hosting: I would add a stronger runtime boundary than this package gives alone.

## What this post leaves out

- The `install` and `build` kinds, beyond where they differ from `tool`.
- Multi-host scheduling.
- A request timeout on the engine client. `internal/engine/client.go` builds an `http.Client` with no `Timeout`. Each request carries the caller's context, so a deadline comes from that context and from the run's wait and poll loop, not from the client.

## Run it yourself

The accompanying code, the `agent-exec` package, is a reference implementation of my per-execution isolation design in Go, unit-tested against a mock engine. Its `run_ephemeral_container` command runs one execution through the subcommands `run`, `build`, `install`, `tool`, `list` and `sweep`. A second binary, `ephemeral-authz`, is the engine authorization plugin. A third, `agent-init`, starts the plugin and `dockerd` in the agent container.

These steps are documented for a standard Ubuntu 26.04 "foundation" VM, which gives an immediate local sandbox to inspect the network gate without external cloud dependencies. I created the VM with Multipass (Hyper-V) on Windows. 

*Target Environment:* The commands run inside the agent container and talk to the Docker Engine API on `/var/run/docker.sock`, or on `AGENT_EXEC_DOCKER_HOST` when set. They never run `docker`, `runsc` or a shell. They read `/workspace/.tools/*/profile.yml`, `image.tar`, `src/` and the named inputs. They write only under `/workspace/.output/exec/<id>/` and never write `src/` or `.tools/`.

*Full runbook:* the [Multipass and Lima runbook]({{link:howto}}) covers building the binaries and an example profile.


<!--kg-card-begin: html-->
<div style="border-left:4px solid #b91c1c;background:color-mix(in srgb, #b91c1c 10%, transparent);padding:12px 16px;margin:1em 0;border-radius:4px;">

**Run this evaluation harness strictly within an isolated, disposable environment!** This repository is an architectural reference pattern designed to benchmark isolation boundaries, not an off-the-shelf production system. Hardening an untrusted-code sandbox for enterprise multi-tenancy—including custom seccomp profiles, network egress filtering, and tenant quotas—requires environment-specific design.

</div>
<!--kg-card-end: html-->


## Code references

These are the places in the code where the disposable-execution decision shows up. I name functions and tests, not line numbers.

<!--kg-card-begin: html-->
<details class="sec-more" id="more-code"><summary><span class="sec-more-show">Show the full section</span><span class="sec-more-hide">Hide the section</span></summary></details>
<!--kg-card-end: html-->

In this post's package, `agent-exec`:

- [sequence.go]({{link:code}}/internal/sequence/sequence.go) holds `Execute`, `validate`, `prepare`, `execute` and `cleanup`.
- [phase.go]({{link:code}}/internal/sequence/phase.go) holds `startAndWait`, `writeMeta` and `writeResult`.
- [kind.go]({{link:code}}/internal/kind/kind.go) holds `NewPlan`, `Verify` and `Network`.
- [policy.go]({{link:code}}/internal/authz/policy.go) holds the `routes` allowlist and `checkCreate`.
- [limits.go]({{link:code}}/internal/limits/limits.go) holds the ceilings shared by client and plugin.
- [wsfs.go]({{link:code}}/internal/wsfs/wsfs.go) holds `openat`, `Lock` and `CopyFile`.
- [tool.go]({{link:code}}/internal/profile/tool.go) holds `Param.Check` and `ExpandToolArgv`.
- [inputpolicy.go]({{link:code}}/internal/inputpolicy/inputpolicy.go) holds `Check` and `spiceSafe`.
- [agentinit.go]({{link:code}}/internal/agentinit/agentinit.go) holds `Init.Run`.
- [sequence_test.go]({{link:code}}/tests/sequence_test.go) holds `TestRunOK`, `TestVerifyMismatch`, `TestUsageErrorsCreateNothing` and `TestExecDirIsNeverReused`.
- [tool_test.go]({{link:code}}/tests/tool_test.go) holds `TestToolRunReference` and `TestToolInputPolicyRefusesBeforeExecDir`.
- [authz_contract_test.go]({{link:code}}/tests/authz_contract_test.go) holds `TestToolBodiesAreAllowedByPlugin`.

The other chapter 1 packages are in the same tag, shared by the six chapter 1 and chapter 2 posts, but I give no link for them here.

## References

- Source of the facts in this post: the package README and code of `agent-exec` in [the code tag]({{link:code}}).
- [Docker Engine API documentation](https://docs.docker.com/reference/api/engine/) and [Docker authorization plugin documentation](https://docs.docker.com/engine/extend/plugins_authorization/).
- Linux [`openat2(2)` manual page](https://man7.org/linux/man-pages/man2/openat2.2.html).
- [gVisor documentation](https://gvisor.dev/docs/).

To reproduce, clone the tag `ch01-ch02/publication`, which the six chapter 1 and chapter 2 posts share.

<!-- TODO(author): add 40-hex sha of ch01-ch02/publication -->
```
git clone --branch ch01-ch02/publication https://github.com/vectorNvolt/vectorNvolt-blog
```
