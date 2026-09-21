// Content shown by the boundary diagram, kept apart from the component so
// the text stays in one place and doesn't drift from the post's prose. The
// IDs are the chapter's fixed vocabulary: T1–T9 vectors, L1–L7 layers,
// IP-1…IP-6 interaction points. Everything here is bundled at build time.

// Short labels for the chips on the detail panel, keyed by id.
export const REF_SHORT = {
  T1: 'Data exfiltration',
  T2: 'Supply-chain poisoning',
  T3: 'Destructive actions',
  T4: 'Lateral movement',
  T5: 'Out-of-band persistence',
  T6: 'Privilege escalation',
  T7: 'Resource exhaustion',
  T8: 'Control-plane compromise',
  T9: 'Orchestrator hijack',
  L1: 'Execution boundary',
  L2: 'Resource governance',
  L3: 'Filesystem hardening',
  L4: 'Network policy',
  L5: 'Credential brokering',
  L6: 'Action governance',
  L7: 'Control-plane scoping',
};

// One entry per Badge id: what the panel shows when it is hovered/selected.
// `iface` is the interface that carries the interaction point, `t` the
// vectors that land on it (danger chips), `l` the layers that harden the
// interface underneath it (safe chips) — same rows as the table in the post.
export const POINT_INFO = {
  'IP-1': {
    name: 'Operator I/O',
    iface: 'Terminal — stdin/stdout via the operator’s terminal emulator',
    desc: 'Operator ↔ host orchestrator. The only human-in-the-loop gate, and the one edge where sandbox-originated bytes reach an interpreter with host-side effect: the terminal emulator.',
    t: ['T1', 'T9'],
    l: ['L6'],
  },
  'IP-2': {
    name: 'Docker control plane',
    iface: 'Container runtime API — the daemon socket',
    desc: 'Host orchestrator ↔ Docker Engine. The privileged side of the boundary: whatever can reach this API can create, exec into, or tear down any container.',
    t: ['T8'],
    l: ['L7'],
  },
  'IP-3': {
    name: 'Exec channel',
    iface: 'TTY-attached exec socket (docker exec stdin/stdout)',
    desc: 'Host orchestrator ↔ process inside the container. Trusted ↔ untrusted data plane; if the harness decides what happened by reading this channel, the agent owns its own monitor.',
    t: ['T7', 'T9'],
    l: ['L6'],
  },
  'IP-4a': {
    name: 'Kernel/Sentry boundary (in)',
    iface: 'Linux syscall ABI — container → gVisor Sentry',
    desc: 'The isolation boundary proper. Under runsc the container’s syscalls are intercepted by the userspace Sentry; under bare runc this hop goes straight to the host kernel.',
    t: ['T6', 'T7'],
    l: ['L1', 'L2'],
  },
  'IP-4b': {
    name: 'Kernel/Sentry boundary (out)',
    iface: 'Linux syscall ABI — Sentry → host kernel',
    desc: 'The Sentry emulates or forwards on the container’s behalf. A kernel-level exploit here is contained to the Sentry process, not host root — T6g–l is the residual set.',
    t: ['T6'],
    l: ['L1'],
  },
  'IP-5': {
    name: 'Container filesystem',
    iface: 'Mounted filesystem — rootfs, volumes, any bind mount',
    desc: 'Container process ↔ storage, and the host if anything is mounted or exported. Ephemeral ↔ persistent: where installs land, where damage lands, and what survives a reset.',
    t: ['T2', 'T3', 'T5'],
    l: ['L3'],
  },
  'IP-6': {
    name: 'Network egress',
    iface: 'Container network namespace (bridge / veth)',
    desc: 'Container ↔ external networks and services. Sandbox ↔ the outside world: the legitimate API call, the exfiltration POST, the registry fetch and the metadata query all ride the same edge.',
    t: ['T1', 'T2', 'T4'],
    l: ['L4', 'L5'],
  },
};
