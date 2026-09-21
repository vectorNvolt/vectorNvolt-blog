// Content shown by the two diagram islands of this post, kept apart from the
// components so the text stays in one place and doesn't drift from the prose.
// Everything here is bundled at build time (no runtime fetches).
//
// IDs are the chapter's: IP-1…IP-6 interaction points, T-cases from the
// overview's inventory. UC-1…UC-3 and N1…N6 are specific to this harness.

// Short labels for chips, keyed by id.
export const REF_SHORT = {
  'UC-1': 'Provision',
  'UC-2': 'Authenticate',
  'UC-3': 'Task exchange',

  'IP-1': 'Operator I/O',
  'IP-2': 'Docker control plane',
  'IP-3': 'Exec channel',
  'IP-4': 'Kernel boundary',
  'IP-4a': 'Container → Sentry',
  'IP-4b': 'Sentry → host kernel',
  'IP-5': 'Container filesystem',
  'IP-6': 'Network egress',

  'T1a': 'outbound POST',
  'T1b': 'DNS tunnelling',
  'T1d': 'ambient credential',
  'T1e': 'terminal query → stdin',
  'T2a–d': 'supply chain',
  'T2c': 'install-time hooks',
  'T3b': 'irreversible remote action',
  'T3c': 'destructive retry loop',
  'T4a–c': 'lateral movement',
  'T5b': 'PATH / build-file backdoor',
  'T5d': 'reused sandbox state',
  'T6a': 'shared kernel surface',
  'T6b': 'runc binary overwrite',
  'T6c': 'leaked-fd escape',
  'T6d': 'privileged / misconfigured',
  'T6e': 'shared-hardware side channels',
  'T6f': 'no cgroup caps',
  'T6g–j': 'Sentry / Gofer residual',
  'T6k': 'platform exposure',
  'T6l': 'env flips runtime',
  'T7a–c': 'no PID / mem / disk caps',
  'T7d': 'Sentry DoS',
  'T7e': 'unbounded read_until · no exec timeout',
  'T8b': 'unscoped docker.from_env()',
  'T9a': 'marker-driven state',
  'T9b': 'unsupervised tool loop',
  'T9c': 'no per-action gate',
  'T9d': 'raw print() to terminal',
  'T9e': 'CLI output is the prompt',
};

// --- system block diagram ---------------------------------------------------
// One entry per interaction point badge. `code` is where it lives in
// src/agent_sandbox; `uc` → info chips, `open` → danger chips, `closed` →
// safe chips.
const COMMON_POINTS = {
  'IP-1': {
    name: 'Operator I/O',
    code: 'gateway.ask_user() → interrupt(), answered by input() in main.py; gateway.notify_user() → print()',
    desc: 'The sole human-in-the-loop gate, at turn granularity only. Agent text reaches the terminal emulator unfiltered — the _clean regex runs on login output alone.',
    uc: ['UC-1', 'UC-2', 'UC-3'],
    open: ['T9c', 'T9d', 'T9e', 'T1e'],
    closed: [],
  },
  'IP-2': {
    name: 'Docker control plane',
    code: 'container.client() — module-level docker.from_env(); create(), find(), containers.get() in nodes 2–4, 6',
    desc: 'Full daemon access from the harness process. No socket is mounted into the container and the agent cannot spawn containers, so only the orchestrator side is live.',
    uc: ['UC-1', 'UC-2', 'UC-3'],
    open: ['T8b', 'T5d'],
    closed: [],
  },
  'IP-3': {
    name: 'Exec channel',
    code: 'ExecSession (exec_create stdin+tty, attached socket) for the login relay; ct.exec() → exec_run(demux=False) for auth status and claude -p',
    desc: 'Free text in both directions. Login transitions on literal markers; read_until grows its buffer unbounded; the timeout ct.exec() accepts is never passed on.',
    uc: ['UC-2', 'UC-3'],
    open: ['T9a', 'T9b', 'T7e', 'T3c'],
    closed: [],
  },
  'IP-5': {
    name: 'Container filesystem',
    code: 'create(): read_only=False; no volumes, mounts, or tmpfs',
    desc: 'The whole rootfs is writable — including the CLI binary on PATH and the OAuth session claude auth login persists — and node 2 reuses it across runs. No host path is mounted, so the blast radius stops at the container.',
    uc: ['UC-3'],
    open: ['T1d', 'T2c', 'T5b', 'T5d', 'T7a–c'],
    closed: [],
  },
  'IP-6': {
    name: 'Network egress',
    code: 'create(): network_mode="bridge"; no egress or DNS policy',
    desc: 'The CLI reaches the Anthropic API because everything is reachable: registries, link-local metadata, host services, and every other agent container on the same default bridge.',
    uc: ['UC-2', 'UC-3'],
    open: ['T1a', 'T1b', 'T2a–d', 'T4a–c', 'T3b'],
    closed: [],
  },
};

export const POINT_INFO = {
  runc: {
    ...COMMON_POINTS,
    'IP-4': {
      name: 'Kernel boundary',
      code: 'CONTAINER_RUNTIME=runc in the environment; cap_drop=["ALL"], security_opt=["no-new-privileges"] still apply',
      desc: 'Raw syscalls to the shared host kernel. cap_drop and no mounts still close the misconfiguration class, but every kernel and runc CVE is one hop from the orchestrator.',
      uc: ['UC-1', 'UC-3'],
      open: ['T6a', 'T6b', 'T6c', 'T6e', 'T6f', 'T6l'],
      closed: ['T6d'],
    },
  },
  runsc: {
    ...COMMON_POINTS,
    'IP-4a': {
      name: 'Container → Sentry',
      code: 'create(): runtime=config.CONTAINER_RUNTIME (os.getenv default "runsc"), cap_drop=["ALL"], security_opt=["no-new-privileges"]',
      desc: 'Syscalls are intercepted by gVisor’s userspace Sentry before they would ever reach the host kernel. Closes the shared-kernel and runc-CVE cases; the kwargs close the misconfiguration class.',
      uc: ['UC-1', 'UC-3'],
      open: ['T6g–j', 'T6l'],
      closed: ['T6a', 'T6b', 'T6c', 'T6d'],
    },
    'IP-4b': {
      name: 'Sentry → host kernel',
      code: 'whatever the daemon’s runsc platform is (ptrace / KVM / systrap); no mem_limit, pids_limit, or CPU quota',
      desc: 'The Sentry’s own narrow, seccomp-filtered call set. A Sentry compromise lands here, contained to that process — but nothing caps what it may consume.',
      uc: ['UC-1', 'UC-3'],
      open: ['T6e', 'T6f', 'T6k', 'T7a–c', 'T7d'],
      closed: [],
    },
  },
};

// --- state machine diagram --------------------------------------------------
// One entry per graph node badge. `ip` → info chips, `t` → danger chips.
export const NODE_INFO = {
  N1: {
    name: 'select_agent',
    desc: 'ask_user("Which agent do you want to work with?") → interrupt(). The name, lower-cased and hyphenated, becomes the container name via CONTAINER_PREFIX.',
    ip: ['IP-1'],
    t: [],
  },
  N2: {
    name: 'check_agent',
    desc: 'ct.find(name) → containers.get(). If the container exists, ensure_running() (re)starts it and the graph skips creation — the previous run’s login and writes come along.',
    ip: ['IP-2'],
    t: ['T5d'],
  },
  N3: {
    name: 'create_container',
    desc: 'ct.create(name) → containers.run(image, runtime=CONTAINER_RUNTIME, network_mode="bridge", read_only=False, security_opt=["no-new-privileges"], cap_drop=["ALL"]). notify_user() prints "runtime=runsc" as a literal, whatever the env resolved.',
    ip: ['IP-2', 'IP-4', 'IP-5', 'IP-6'],
    t: ['T6l', 'T7a–c', 'T8b'],
  },
  N4: {
    name: 'auth_gateway (loop)',
    desc: 'check: is_authenticated() runs claude auth status; else start_login() opens a TTY ExecSession on claude auth login and read_until("Paste code here"). relay: interrupt() with that output as the prompt, then finish_login() writes the reply back, read_until("Invalid code"), and sets authenticated from exit code + auth status. Capped at MAX_AUTH_ATTEMPTS.',
    ip: ['IP-1', 'IP-2', 'IP-3'],
    t: ['T9a', 'T9e', 'T7e', 'T1d'],
  },
  N5: {
    name: 'ask_task',
    desc: 'ask_user("… What do you want to do?") → interrupt(). Seeds the transcript with the first user turn.',
    ip: ['IP-1'],
    t: [],
  },
  N6: {
    name: 'exchange (loop)',
    desc: 'agent turn: send_prompt() runs claude -p --output-format text via exec_run, piping the last user message through sh -c "printf %s … | claude -p"; the answer goes to notify_user() → print(). user turn: interrupt(); an EXIT_WORDS entry sets done. Only the operator can end the loop.',
    ip: ['IP-1', 'IP-3', 'IP-5', 'IP-6'],
    t: ['T9b', 'T9c', 'T9d', 'T1e', 'T3c'],
  },
};
