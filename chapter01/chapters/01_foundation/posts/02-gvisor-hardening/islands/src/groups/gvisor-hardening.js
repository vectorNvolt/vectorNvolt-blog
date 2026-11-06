// Three block diagrams for the gVisor hardening post, all drawn by the generic
// BlockDiagram.jsx:
//   runtime → the runtime boundary and the flags that shape it
//   runc    → SYS_ADMIN checked by the host kernel (shared kernel)
//   gvisor  → SYS_ADMIN checked by the Sentry (emulated credentials)
// The runc and gvisor drawings share viewH and row positions so they read as
// alternatives. Geometry first (`block`), every word the panels show second
// (`blockText`); chip labels come from REF_SHORT in ../data.js.

const runtime = {
  block: {
    // Layout as in post 01: agent top-left, host kernel as the full-width bottom
    // row, every connector straight (no bends).
    viewH: 450,
    outlines: [
      { x: 20, y: 20, w: 920, h: 410, label: 'agent-host' },
      { x: 40, y: 50, w: 670, h: 260, label: 'gVisor sandbox (runsc)', variant: 'safe' },
    ],
    nodes: {
      agent: { x: 60, y: 90, w: 140, h: 64, label: 'Agent process', sub: ['untrusted', 'root inside'], variant: 'solid-danger' },
      sentry: { x: 230, y: 90, w: 270, h: 64, label: 'Sentry', sub: ['user-space kernel', 'systrap'], variant: 'solid-safe' },
      overlay: { x: 540, y: 90, w: 150, h: 64, label: 'Rootfs overlay', sub: ['in host RAM', '--overlay2=root:memory'] },
      gofer: { x: 230, y: 214, w: 150, h: 64, label: 'Gofer', sub: ['--directfs=false'] },
      netstack: { x: 400, y: 214, w: 170, h: 64, label: 'Netstack', sub: ['--network=sandbox', '--net-raw=false'] },
      daemon: { x: 740, y: 214, w: 180, h: 64, label: 'daemon.json', sub: ['runtime registration', '--allow-flag-override=false'] },
      kernel: { x: 40, y: 350, w: 880, h: 64, label: 'Host kernel', sub: 'reduced syscall surface', variant: 'solid-dark' },
    },
    links: [{ points: [[500, 122], [540, 122]], dir: 'end' }],
    ips: [
      { id: 'IP-1', label: '1', points: [[200, 122], [230, 122]], dir: 'both', at: [215, 122] },
      { id: 'IP-2', label: '2', points: [[305, 154], [305, 214]], dir: 'both', at: [305, 184] },
      { id: 'IP-3', label: '3', points: [[485, 154], [485, 214]], dir: 'both', at: [485, 184] },
      { id: 'IP-4', label: '4', points: [[305, 278], [305, 350]], dir: 'end', at: [305, 330] },
      { id: 'IP-5', label: '5', points: [[485, 278], [485, 350]], dir: 'end', at: [485, 330] },
      { id: 'IP-6', label: '6', points: [[740, 246], [710, 246]], dir: 'end', at: [725, 246] },
      { id: 'IP-7', label: '7', points: [[390, 154], [390, 350]], dir: 'end', at: [390, 184] },
    ],
    assets: [
      { id: 'AS-1', label: '1', at: [690, 154] },
      { id: 'AS-2', label: '2', at: [740, 278] },
    ],
    threats: [
      { id: 'T1a', label: '1a', at: [500, 90] },
      { id: 'T1b', label: '1b', at: [540, 154] },
    ],
  },
  blockText: {
    label: 'Block diagram of the gVisor runtime boundary: the agent process talks only to the Sentry, which reaches the host kernel through the Gofer and Netstack and through a narrow set of its own host syscalls; the rootfs overlay lives in host RAM and the runtime is registered in daemon.json with flag override disabled',
    hint: 'Circles are interaction points, diamonds assets, squares threats; hover or click one to highlight what it connects to. The flags in each box are the settings this post applies.',
    items: {
      'IP-1': {
        name: 'Syscalls from the agent process',
        desc: 'The agent’s syscalls are handled by the Sentry, a kernel implemented in user space, instead of going straight to the host kernel. With the systrap platform they are intercepted and passed to the Sentry; the exact mechanism depends on the platform in use.',
        code: '--platform=systrap',
        links: ['IP-2', 'IP-3', 'IP-7', 'T1a'],
      },
      'IP-2': {
        name: 'File access through the Gofer',
        desc: 'The Sentry is intended to reach host files only through the Gofer, which brokers the access. Turning off directfs keeps that brokering in place rather than handing the Sentry direct host file descriptors.',
        code: '--directfs=false',
        links: ['IP-4', 'AS-1'],
      },
      'IP-3': {
        name: 'Network through Netstack',
        desc: 'Network traffic is handled by Netstack, the network stack that runs inside the sandbox. Raw sockets are switched off, which is meant to narrow what a root process in the sandbox can craft.',
        code: '--network=sandbox --net-raw=false',
        links: ['IP-5'],
      },
      'IP-4': {
        name: 'Gofer to host kernel',
        desc: 'The Gofer performs file operations on the host on the sandbox’s behalf. These are ordinary host syscalls, but a smaller and more predictable set than an arbitrary container process could issue.',
        links: ['IP-7', 'T1a'],
      },
      'IP-5': {
        name: 'Netstack to host kernel',
        desc: 'Packets leave the sandbox through a small set of host interfaces rather than through the agent’s own socket calls.',
        links: ['IP-7', 'T1a'],
      },
      'IP-7': {
        name: 'Sentry’s own host syscalls',
        desc: 'The Sentry still calls the host kernel for its own operation (memory mapping, futex, timers, epoll, I/O on host file descriptors), through a narrow set of syscalls filtered by seccomp. With --directfs enabled it also uses host file descriptors directly; this post turns it off.',
        links: ['IP-4', 'IP-5', 'T1a'],
      },
      'IP-6': {
        name: 'Runtime registration',
        desc: 'The runtime is registered with Docker in daemon.json, together with its flags. Setting --allow-flag-override=false is meant to stop a container from supplying its own runtime flags and loosening these settings.',
        code: 'daemon.json::runtimes.runsc.runtimeArgs',
        links: ['AS-2'],
      },
      'AS-1': {
        name: 'Rootfs overlay in host RAM',
        desc: 'With --overlay2=root:memory the container’s root filesystem writes go to an overlay held in memory instead of the image layers on disk. Changes are expected to disappear when the container stops.',
        code: '--overlay2=root:memory',
        links: ['T1b'],
      },
      'AS-2': {
        name: 'daemon.json runtime entry',
        desc: 'The file that tells Docker how to launch the sandboxed runtime. Anyone who can edit it could change which flags apply, so it is treated as part of the trusted host configuration.',
        code: '--allow-flag-override=false',
      },
      T1a: {
        name: 'Sentry escape',
        desc: 'A bug in the Sentry could let a process break out of the sandbox. This is the main residual risk of the design. The Sentry is intended to be a small, memory-safe component, but that reduces the likelihood rather than ruling it out.',
      },
      T1b: {
        name: 'Memory pressure from the overlay',
        desc: 'Because the root overlay is held in host RAM, a process that writes a lot of data can consume memory. A memory limit on the container is the usual way to bound this; whether it is sufficient depends on the workload.',
      },
    },
  },
};

// Two alternatives drawn on the same grid: the same agent process asks for
// SYS_ADMIN-gated operations; who decides differs.
const GRID = {
  viewH: 410,
  host: { x: 20, y: 20, w: 920, h: 370, label: 'Host' },
  kernel: { x: 40, y: 310, w: 880, h: 64 },
};

const runc = {
  block: {
    viewH: GRID.viewH,
    outlines: [GRID.host, { x: 40, y: 60, w: 200, h: 130, label: 'runc container', variant: 'danger' }],
    nodes: {
      agent: { x: 60, y: 100, w: 160, h: 64, label: 'Agent process', sub: ['root inside', 'SYS_ADMIN granted'], variant: 'solid-danger' },
      check: { x: 720, y: 100, w: 200, h: 64, label: 'Capability check', sub: ['host kernel, not seccomp', 'process capability sets'] },
      kernel: { ...GRID.kernel, label: 'Host kernel', sub: 'shared with container', variant: 'solid-danger' },
    },
    links: [],
    ips: [
      { id: 'IP-R1', label: 'R1', points: [[140, 164], [140, 310]], dir: 'end', at: [140, 250] },
      { id: 'IP-R2', label: 'R2', points: [[820, 310], [820, 164]], dir: 'end', at: [820, 250] },
    ],
    assets: [],
    threats: [
      { id: 'T-R1', label: 'R1', at: [300, 310] },
      { id: 'T-R2', label: 'R2', at: [900, 310] },
    ],
  },
  blockText: {
    label: 'Block diagram of SYS_ADMIN under runc: the agent process’s mount and namespace syscalls go directly to the host kernel, whose own capability check evaluates them against the container process’s capability sets',
    hint: 'Circles are interaction points, squares threats; hover or click one to see what it connects to.',
    items: {
      'IP-R1': {
        name: 'Mount and namespace syscalls reach the host kernel',
        desc: 'Under runc the container is a set of namespaces and cgroups on the host. Calls such as mount, unshare and setns are handled by the host kernel itself. Whether one succeeds depends on the capability check there, which is evaluated against the host’s user namespace rules.',
        code: 'mount(2), unshare(2), setns(2)',
        links: ['IP-R2', 'T-R1', 'T-R2'],
      },
      'IP-R2': {
        name: 'Host kernel checks the capability',
        desc: 'The check is the host kernel’s own, not seccomp and not a separate component. For each privileged call the kernel evaluates the capability against the container process’s credentials and capability sets, which are bounded by the --cap-add and --cap-drop set Docker applied. If the capability is present, the operation runs in the host kernel.',
        code: 'capable(CAP_SYS_ADMIN) in the host kernel',
        links: ['T-R2'],
      },
      'T-R1': {
        name: 'Kernel attack surface',
        desc: 'The whole host syscall interface is reachable from the container. A flaw in any code path it can reach is a flaw in the kernel the host and every other container depend on.',
      },
      'T-R2': {
        name: 'Container escape is host access',
        desc: 'Because the kernel is shared, an escape does not cross a further boundary: it lands in the host kernel’s context. Granting SYS_ADMIN widens the set of operations an attacker can attempt.',
      },
    },
  },
};

const gvisor = {
  block: {
    viewH: GRID.viewH,
    outlines: [GRID.host, { x: 40, y: 60, w: 660, h: 220, label: 'gVisor sandbox (runsc)', variant: 'safe' }],
    nodes: {
      agent: { x: 60, y: 100, w: 160, h: 64, label: 'Agent process', sub: ['root inside', 'SYS_ADMIN granted'], variant: 'solid-danger' },
      sentry: { x: 290, y: 100, w: 180, h: 64, label: 'Sentry', sub: ['user-space kernel'], variant: 'solid-safe' },
      io: { x: 540, y: 100, w: 150, h: 64, label: 'Gofer · Netstack', sub: ['brokered I/O'] },
      check: { x: 340, y: 204, w: 180, h: 64, label: 'Capability check', sub: ['inside Sentry, not seccomp', 'emulated credentials'] },
      kernel: { ...GRID.kernel, label: 'Host kernel', sub: 'reduced syscall surface', variant: 'solid-dark' },
    },
    links: [],
    ips: [
      { id: 'IP-G1', label: 'G1', points: [[220, 132], [290, 132]], dir: 'end', at: [255, 132] },
      { id: 'IP-G2', label: 'G2', points: [[470, 132], [540, 132]], dir: 'both', at: [505, 132] },
      { id: 'IP-G3', label: 'G3', points: [[615, 164], [615, 310]], dir: 'end', at: [615, 296] },
      { id: 'IP-G4', label: 'G4', points: [[380, 164], [380, 204]], dir: 'end', at: [380, 184] },
      { id: 'IP-G5', label: 'G5', points: [[310, 164], [310, 310]], dir: 'end', at: [310, 250] },
    ],
    assets: [],
    threats: [
      { id: 'T-G1', label: 'G1', at: [470, 100] },
      { id: 'T-G2', label: 'G2', at: [880, 310] },
    ],
  },
  blockText: {
    label: 'Block diagram of SYS_ADMIN under gVisor: the agent process’s syscalls go to the Sentry, which runs its own emulated capability check against the sandbox’s emulated credentials for other syscalls; the host kernel is reached by the Gofer and Netstack and by a narrow seccomp-filtered set of the Sentry’s own host syscalls, but not by the capability check',
    hint: 'Circles are interaction points, squares threats; hover or click one to see what it connects to.',
    items: {
      'IP-G1': {
        name: 'Syscalls arrive at the Sentry',
        desc: 'The same mount, unshare and setns calls are answered by the Sentry. The capability check runs against the credentials the Sentry keeps for the sandbox, so SYS_ADMIN here is a property of the emulated kernel, not of the host.',
        code: 'mount(2), unshare(2), setns(2)',
        links: ['IP-G2', 'IP-G4', 'IP-G5', 'T-G1'],
      },
      'IP-G4': {
        name: 'Other syscalls, handled inside the Sentry',
        desc: 'Syscalls that are neither file nor network I/O, such as mount or a capability-gated call, are answered inside the Sentry. Its capability check is its own emulation, run against the sandbox’s emulated credentials: it is not the host kernel and not seccomp, and these calls do not reach the host kernel.',
        code: 'mount(2), unshare(2), setns(2)',
        links: ['T-G1'],
      },
      'IP-G2': {
        name: 'Sentry to Gofer and Netstack',
        desc: 'File and network work is delegated to the Gofer and to Netstack, which are the components that touch host resources.',
        links: ['IP-G3'],
      },
      'IP-G3': {
        name: 'Remaining host syscalls',
        desc: 'The host kernel is still reached, but through a restricted set of syscalls issued by the sandbox’s own components rather than by the agent process. The Sentry makes some host syscalls of its own as well (see the Sentry’s own host syscalls).',
        links: ['IP-G5', 'T-G2'],
      },
      'IP-G5': {
        name: 'Sentry’s own host syscalls',
        desc: 'The Sentry still calls the host kernel for its own operation (memory mapping, futex, timers, epoll, I/O on host file descriptors), through a narrow set of syscalls filtered by seccomp. With --directfs enabled it also uses host file descriptors directly. The agent’s syscalls that the Sentry answers itself, including the capability check, do not appear here.',
        links: ['IP-G3', 'T-G2'],
      },
      'T-G1': {
        name: 'Sentry bug',
        desc: 'A flaw in the Sentry’s implementation of a syscall could let a process act beyond its emulated privileges. The boundary moves from the host kernel to a smaller, separate component; it does not disappear.',
      },
      'T-G2': {
        name: 'Residual host exposure',
        desc: 'The host kernel is still part of the path. The aim is a smaller reachable surface than under runc, not zero exposure.',
      },
    },
  },
};

export const variants = { runtime, runc, gvisor };
