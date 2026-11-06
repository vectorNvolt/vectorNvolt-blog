// Seven isolation alternatives, one block diagram each, drawn by the generic
// BlockDiagram.jsx (hand laid out, viewBox units, orthogonal connectors only):
//   bare*   → bare Linux process (fork/exec)
//   runc*   → Docker / runc (namespaces + cgroups, shared host kernel)
//   wasm*   → WebAssembly / WASI runtime (Wasmtime, Wasmer; capabilities)
//   gvisor* → gVisor / runsc (Sentry + Gofer)
//   microvm* → microVM (Firecracker: minimal VMM + KVM)
//   kata*   → Kata Containers (OCI shim + VMM + KVM, guest kernel)
//   vm*     → standard VM (QEMU/KVM, full guest OS)
// Geometry (`*Block`) first, every word the panels show (`*Text`) second.
// Layout: the layers sit in a column at x = 60 (top: untrusted agent code,
// bottom: the shared host kernel); side paths run to the right.
// Colour meaning: danger = untrusted code, safe = the component that holds
// the boundary, solid-dark = shared host infrastructure.
// The panel text states trade-offs against my own threat model (an agent
// framework executing LLM-generated, untrusted scripts).

// ---------------------------------------------------------------- 1. Bare process

export const bareBlock = {
  viewH: 324,
  outlines: [{ x: 40, y: 24, w: 400, h: 200, label: 'Same user, same view of the host', variant: 'danger' }],
  nodes: {
    code: { x: 60, y: 64, w: 360, h: 52, label: 'Untrusted agent code', sub: 'LLM-generated bash / scripts', variant: 'solid-danger' },
    proc: { x: 60, y: 156, w: 360, h: 52, label: 'Child process', sub: 'fork/exec, same user' },
    resources: { x: 560, y: 156, w: 320, h: 52, label: 'Host files, network, secrets', sub: 'whatever the user can reach', variant: 'solid-dark' },
    kernel: { x: 60, y: 248, w: 820, h: 52, label: 'Host Linux kernel', sub: 'full syscall interface', variant: 'solid-dark' },
  },
  links: [],
  ips: [
    { id: 'IP-1', label: '1', points: [[240, 116], [240, 156]], dir: 'end', at: [292, 136] },
    { id: 'IP-2', label: '2', points: [[420, 182], [560, 182]], dir: 'end', at: [490, 182] },
    { id: 'IP-3', label: '3', points: [[240, 208], [240, 248]], dir: 'end', at: [292, 228] },
  ],
  assets: [{ id: 'AS-1', label: '1', at: [880, 156] }],
  threats: [{ id: 'T1', label: '1', at: [188, 228] }],
};

export const bareText = {
  label: 'Block diagram of a bare Linux process: untrusted code runs as a child process with the same user rights and reaches host files, network and the full syscall interface directly',
  hint: 'Circles are interaction points, the diamond is an asset, the square is a threat. Hover or click one to see what it connects to.',
  items: {
    'IP-1': {
      name: 'Spawn: the agent runs the code as a child process',
      desc: 'The framework starts the script with fork/exec. The child gets the same user, working directory and environment unless the framework restricts them itself.',
    },
    'IP-2': {
      name: 'Direct access to host files, network and secrets',
      desc: 'Nothing sits between the process and the host’s resources. It sees what the user it runs as can see.',
    },
    'IP-3': {
      name: 'System calls straight to the host kernel',
      desc: 'The process calls the host kernel directly, with the full syscall interface available to it.',
    },
    'AS-1': {
      name: 'Host data and credentials',
      desc: 'Files, environment variables, network access and other processes of the user that runs the agent framework. This is what I want to protect.',
    },
    T1: {
      name: 'Untrusted code reads or alters host data',
      desc: 'Generated code that is wrong, or steered by a malicious prompt, can do whatever that user can do. For my threat model this is why I would not use this boundary for LLM-generated code.',
      links: ['IP-2', 'IP-3', 'AS-1'],
    },
  },
};

// ---------------------------------------------------------------- 2. Docker / runc

export const runcBlock = {
  viewH: 324,
  outlines: [{ x: 40, y: 24, w: 400, h: 200, label: 'Container', variant: 'danger' }],
  nodes: {
    code: { x: 60, y: 64, w: 360, h: 52, label: 'Untrusted agent code', sub: 'LLM-generated bash / scripts', variant: 'solid-danger' },
    container: { x: 60, y: 156, w: 360, h: 52, label: 'Namespaces + cgroups', sub: 'own PID / mount / net view, resource limits' },
    mounts: { x: 560, y: 156, w: 320, h: 52, label: 'Mounted host paths, devices', sub: 'only what I choose to give it' },
    kernel: { x: 60, y: 248, w: 820, h: 52, label: 'Shared host Linux kernel', sub: 'the same kernel as the host and other containers', variant: 'solid-dark' },
  },
  links: [],
  ips: [
    { id: 'IP-1', label: '1', points: [[240, 116], [240, 156]], dir: 'end', at: [292, 136] },
    { id: 'IP-2', label: '2', points: [[420, 182], [560, 182]], dir: 'end', at: [490, 182] },
    { id: 'IP-3', label: '3', points: [[240, 208], [240, 248]], dir: 'end', at: [292, 228] },
  ],
  assets: [{ id: 'AS-1', label: '1', at: [880, 248] }],
  threats: [{ id: 'T1', label: '1', at: [188, 228] }],
};

export const runcText = {
  label: 'Block diagram of a Docker/runc container: untrusted code runs in namespaces and cgroups, and its system calls are handled by the shared host kernel',
  hint: 'Circles are interaction points, the diamond is an asset, the square is a threat. Hover or click one to see what it connects to.',
  items: {
    'IP-1': {
      name: 'Process starts inside namespaces and cgroups',
      desc: 'The runtime sets up PID, mount, network and other namespaces and cgroup limits, then starts the process. The code gets a restricted view of the host, not a separate kernel.',
    },
    'IP-2': {
      name: 'Mounts and devices: only what I give it',
      desc: 'Bind mounts, volumes and devices are how host data reaches the container. Mounting less is the lever I control.',
    },
    'IP-3': {
      name: 'System calls go to the shared host kernel',
      desc: 'The container’s system calls are handled by the kernel that runs the host. Dropped capabilities and a seccomp profile narrow what is allowed. The calls that remain allowed still run in the host kernel.',
    },
    'AS-1': {
      name: 'Host kernel and the node',
      desc: 'Everything that depends on the host kernel: the host itself and the other containers on it.',
    },
    T1: {
      name: 'Kernel or runtime bug reachable through an allowed call',
      desc: 'Kernel and container-runtime vulnerabilities that allowed container escapes have been published in the past. A bug in a call the profile still permits would cross this boundary. It is the residual risk I weigh.',
      links: ['IP-3', 'AS-1'],
    },
  },
};

// ---------------------------------------------------------------- 3. WebAssembly / WASI runtime

export const wasmBlock = {
  viewH: 324,
  outlines: [{ x: 40, y: 24, w: 400, h: 200, label: 'Wasm sandbox (runtime)', variant: 'safe' }],
  nodes: {
    code: { x: 60, y: 64, w: 360, h: 52, label: 'Untrusted agent code', sub: 'must be compiled to, or run inside, Wasm', variant: 'solid-danger' },
    runtime: { x: 60, y: 156, w: 360, h: 52, label: 'Wasm runtime (Wasmtime / Wasmer)', sub: 'a normal host process, imports only', variant: 'solid-safe' },
    caps: { x: 560, y: 156, w: 320, h: 52, label: 'Granted WASI capabilities', sub: 'preopened directories, sockets I choose' },
    kernel: { x: 60, y: 248, w: 820, h: 52, label: 'Host Linux kernel', sub: 'reached by the runtime’s own syscalls', variant: 'solid-dark' },
  },
  links: [],
  ips: [
    { id: 'IP-1', label: '1', points: [[240, 116], [240, 156]], dir: 'end', at: [292, 136] },
    { id: 'IP-2', label: '2', points: [[240, 208], [240, 248]], dir: 'end', at: [292, 228] },
    { id: 'IP-3', label: '3', points: [[420, 182], [560, 182]], dir: 'end', at: [490, 182] },
    { id: 'IP-4', label: '4', points: [[720, 208], [720, 248]], dir: 'end', at: [772, 228] },
  ],
  assets: [{ id: 'AS-1', label: '1', at: [880, 248] }],
  threats: [{ id: 'T1', label: '1', at: [188, 228] }],
};

export const wasmText = {
  label: 'Block diagram of a WebAssembly runtime: untrusted code runs as a Wasm module with no direct system calls, the runtime provides only the granted WASI capabilities, and the runtime itself is a normal host process on the host kernel',
  hint: 'Circles are interaction points, the diamond is an asset, the square is a threat. Hover or click one to see what it connects to.',
  items: {
    'IP-1': {
      name: 'The module calls only imported functions',
      desc: 'A Wasm module has no instruction for a system call. It can only call functions the runtime imports into it. The code first has to be compiled to Wasm, or an interpreter for it has to run as a Wasm module.',
    },
    'IP-2': {
      name: 'Runtime to host: the runtime’s own syscalls',
      desc: 'The runtime (for example Wasmtime or Wasmer) is an ordinary process on the host kernel. It makes host system calls to implement what it grants. It can be confined further with the usual host mechanisms. The runtime is the trust boundary, so a bug in it matters.',
    },
    'IP-3': {
      name: 'Capabilities: only what I grant',
      desc: 'WASI is capability based. A module gets the directories, sockets, environment and clocks the host hands it, and nothing else. Which of these exist depends on the runtime and the WASI version.',
    },
    'IP-4': {
      name: 'Granted resources on the host',
      desc: 'A preopened directory or a socket is a real host resource that the runtime opens on the module’s behalf. Granting less is the lever I control.',
    },
    'AS-1': {
      name: 'Host kernel and the node',
      desc: 'The same asset as before. The module cannot issue a host system call itself. The runtime issues them on its behalf, so its own syscalls reach the host kernel.',
    },
    T1: {
      name: 'Flaw in the runtime or in a granted capability',
      desc: 'An escape needs a bug in the runtime’s sandboxing, or a capability that I granted too widely. The larger cost for me is the workload: arbitrary bash and installed tools would not run unchanged.',
      links: ['IP-2', 'IP-3', 'IP-4', 'AS-1'],
    },
  },
};

// ---------------------------------------------------------------- 4. gVisor / runsc

export const gvisorBlock = {
  viewH: 324,
  outlines: [{ x: 40, y: 24, w: 400, h: 200, label: 'gVisor sandbox (runsc)', variant: 'safe' }],
  nodes: {
    code: { x: 60, y: 64, w: 360, h: 52, label: 'Untrusted agent code', sub: 'LLM-generated bash / scripts', variant: 'solid-danger' },
    sentry: { x: 60, y: 156, w: 360, h: 52, label: 'Sentry (user-space kernel)', sub: 'implements Linux syscalls, written in Go', variant: 'solid-safe' },
    gofer: { x: 560, y: 156, w: 320, h: 52, label: 'Gofer', sub: 'separate process for file access (or directfs)', variant: 'solid-safe' },
    kernel: { x: 60, y: 248, w: 820, h: 52, label: 'Host Linux kernel', sub: 'reached through a restricted syscall set', variant: 'solid-dark' },
  },
  links: [],
  ips: [
    { id: 'IP-1', label: '1', points: [[240, 116], [240, 156]], dir: 'end', at: [292, 136] },
    { id: 'IP-2', label: '2', points: [[240, 208], [240, 248]], dir: 'end', at: [292, 228] },
    { id: 'IP-3', label: '3', points: [[420, 182], [560, 182]], dir: 'both', at: [490, 182] },
    { id: 'IP-4', label: '4', points: [[720, 208], [720, 248]], dir: 'end', at: [772, 228] },
  ],
  assets: [{ id: 'AS-1', label: '1', at: [880, 248] }],
  threats: [{ id: 'T1', label: '1', at: [188, 228] }],
};

export const gvisorText = {
  label: 'Block diagram of gVisor with runsc: untrusted code talks to the Sentry, a user-space kernel, which narrows the host syscall surface to a restricted set of calls and mediates file access through a Gofer process or directfs',
  hint: 'Circles are interaction points, the diamond is an asset, the square is a threat. Hover or click one to see what it connects to.',
  items: {
    'IP-1': {
      name: 'System calls are handled by the Sentry first',
      desc: 'The Sentry is a user-space kernel, written in Go, that implements a large part of the Linux syscall interface itself. The sandboxed code talks to it.',
    },
    'IP-2': {
      name: 'Sentry to host: a restricted syscall set',
      desc: 'The Sentry runs with a narrow filter and uses a limited set of host syscalls. This set is much smaller than the full Linux interface.',
    },
    'IP-3': {
      name: 'File requests between the Sentry and the Gofer',
      desc: 'File access from the sandbox is mediated by the Sentry. It goes through the Gofer, which performs the file operations on the host, or through directfs.',
    },
    'IP-4': {
      name: 'Gofer to host: file operations',
      desc: 'The Gofer opens and reads files on the host. It is a separate process with its own restricted privileges.',
    },
    'AS-1': {
      name: 'Host kernel and the node',
      desc: 'The same asset as in the container case. What changes is how much of the kernel interface the untrusted code can reach.',
    },
    T1: {
      name: 'Bug in the Sentry or in an allowed host call',
      desc: 'The boundary moves from the full kernel interface to the Sentry and its restricted host interface. It is still software, so an escape needs a bug there. I rely on the gVisor project for the quality of that code.',
      links: ['IP-2', 'IP-4', 'AS-1'],
    },
  },
};

// ---------------------------------------------------------------- 5. microVM (Firecracker)

export const microvmBlock = {
  viewH: 416,
  outlines: [{ x: 40, y: 24, w: 400, h: 200, label: 'microVM guest', variant: 'safe' }],
  nodes: {
    code: { x: 60, y: 64, w: 360, h: 52, label: 'Untrusted agent code', sub: 'LLM-generated bash / scripts', variant: 'solid-danger' },
    guest: { x: 60, y: 156, w: 360, h: 52, label: 'Guest Linux kernel', sub: 'its own kernel, inside the microVM' },
    vmm: { x: 60, y: 248, w: 360, h: 52, label: 'Firecracker VMM', sub: 'minimal device model, a host process', variant: 'solid-safe' },
    kernel: { x: 60, y: 340, w: 820, h: 52, label: 'Host Linux kernel + KVM', sub: 'hardware virtualization needs /dev/kvm', variant: 'solid-dark' },
  },
  links: [],
  ips: [
    { id: 'IP-1', label: '1', points: [[240, 116], [240, 156]], dir: 'end', at: [292, 136] },
    { id: 'IP-2', label: '2', points: [[240, 208], [240, 248]], dir: 'both', at: [292, 228] },
    { id: 'IP-3', label: '3', points: [[420, 182], [800, 182], [800, 340]], dir: 'both', at: [610, 182] },
    { id: 'IP-4', label: '4', points: [[240, 300], [240, 340]], dir: 'end', at: [292, 320] },
  ],
  assets: [{ id: 'AS-1', label: '1', at: [880, 340] }],
  threats: [{ id: 'T1', label: '1', at: [188, 320] }],
};

export const microvmText = {
  label: 'Block diagram of a Firecracker microVM: untrusted code runs on a guest kernel, and the guest reaches the host through virtual devices in a minimal VMM and through hardware virtualization in KVM',
  hint: 'Circles are interaction points, the diamond is an asset, the square is a threat. Hover or click one to see what it connects to.',
  items: {
    'IP-1': {
      name: 'System calls go to the guest kernel',
      desc: 'The code’s system calls are handled by a kernel that lives inside the microVM, not by the host kernel.',
    },
    'IP-2': {
      name: 'Virtual devices through the VMM',
      desc: 'Block, network and similar devices are emulated by the Firecracker VMM. The Firecracker device model is kept minimal on purpose.',
    },
    'IP-3': {
      name: 'CPU and memory under hardware virtualization (KVM)',
      desc: 'The guest runs on the CPU under KVM. Privileged operations cause VM exits that KVM and the VMM handle. This needs /dev/kvm on the host, either bare metal or nested virtualization.',
    },
    'IP-4': {
      name: 'VMM to host: its own syscalls',
      desc: 'The VMM is an ordinary host process that uses KVM (/dev/kvm) and the host kernel to run the guest, so the host kernel stays reachable through KVM and the VMM’s own syscalls. Firecracker sandboxes it further, for example with its jailer and seccomp filters.',
    },
    'AS-1': {
      name: 'Host kernel and the node',
      desc: 'The same asset as before. The untrusted code now faces a hypervisor boundary instead of the syscall interface.',
    },
    T1: {
      name: 'Flaw in the VMM device model or in KVM',
      desc: 'An escape needs a flaw in the VMM’s device emulation or in the hypervisor path. The surface is different in kind from the syscall interface.',
      links: ['IP-2', 'IP-3', 'IP-4', 'AS-1'],
    },
  },
};

// ---------------------------------------------------------------- 6. Kata Containers

export const kataBlock = {
  viewH: 416,
  outlines: [{ x: 40, y: 24, w: 400, h: 200, label: 'Kata pod VM', variant: 'safe' }],
  nodes: {
    code: { x: 60, y: 64, w: 360, h: 52, label: 'Untrusted agent code', sub: 'LLM-generated bash / scripts', variant: 'solid-danger' },
    guest: { x: 60, y: 156, w: 360, h: 52, label: 'Guest Linux kernel', sub: 'its own kernel, inside the pod VM' },
    vmm: { x: 60, y: 248, w: 360, h: 52, label: 'VMM (QEMU / Cloud Hypervisor / Firecracker)', sub: 'a host process, started through the Kata shim', variant: 'solid-safe' },
    shim: { x: 560, y: 248, w: 200, h: 52, label: 'OCI runtime shim', sub: 'Docker / containerd / Kubernetes', variant: 'solid-dark' },
    kernel: { x: 60, y: 340, w: 820, h: 52, label: 'Host Linux kernel + KVM', sub: 'hardware virtualization, or nested virtualization', variant: 'solid-dark' },
  },
  links: [],
  ips: [
    { id: 'IP-1', label: '1', points: [[240, 116], [240, 156]], dir: 'end', at: [292, 136] },
    { id: 'IP-2', label: '2', points: [[240, 208], [240, 248]], dir: 'both', at: [292, 228] },
    { id: 'IP-3', label: '3', points: [[420, 182], [800, 182], [800, 340]], dir: 'both', at: [610, 182] },
    { id: 'IP-4', label: '4', points: [[240, 300], [240, 340]], dir: 'end', at: [292, 320] },
    { id: 'IP-5', label: '5', points: [[560, 274], [420, 274]], dir: 'end', at: [490, 274] },
  ],
  assets: [{ id: 'AS-1', label: '1', at: [880, 340] }],
  threats: [{ id: 'T1', label: '1', at: [188, 320] }],
};

export const kataText = {
  label: 'Block diagram of Kata Containers: the container manager starts the workload through an OCI shim, the untrusted code runs on a guest kernel inside a lightweight VM, and the VM reaches the host through a VMM and hardware virtualization in KVM',
  hint: 'Circles are interaction points, the diamond is an asset, the square is a threat. Hover or click one to see what it connects to.',
  items: {
    'IP-1': {
      name: 'System calls go to the guest kernel',
      desc: 'The container’s processes run on a guest kernel inside a lightweight VM, not on the host kernel.',
    },
    'IP-2': {
      name: 'Virtual devices through the VMM',
      desc: 'Kata can use different VMMs, among them QEMU, Cloud Hypervisor and Firecracker. The device surface depends on the one chosen and how it is configured.',
    },
    'IP-3': {
      name: 'CPU and memory under hardware virtualization (KVM)',
      desc: 'The guest runs under KVM, so the host needs hardware virtualization or nested virtualization. This is the same requirement that I weigh against the microVM option.',
    },
    'IP-4': {
      name: 'VMM to host: its own syscalls',
      desc: 'The VMM is a host process (QEMU, Cloud Hypervisor or Firecracker) that uses KVM (/dev/kvm) and the host kernel to run the guest, so the host kernel stays reachable through KVM and the VMM’s own syscalls. It can be confined further with host mechanisms.',
    },
    'IP-5': {
      name: 'The container manager starts it through a shim',
      desc: 'Kata plugs in as an OCI-compatible runtime through a shim, so Docker, containerd or Kubernetes can start a pod or container the usual way and the runtime puts it inside a VM. This is the container ergonomics that a bare microVM lacks.',
    },
    'AS-1': {
      name: 'Host kernel and the node',
      desc: 'The same asset as before. The boundary is a hypervisor boundary, as with a microVM.',
    },
    T1: {
      name: 'Flaw in the VMM or in KVM',
      desc: 'Same class of risk as in the microVM case, with the VMM as a choice.',
      links: ['IP-2', 'IP-3', 'IP-4', 'AS-1'],
    },
  },
};

// ---------------------------------------------------------------- 7. Standard VM (QEMU/KVM)

export const vmBlock = {
  viewH: 416,
  outlines: [{ x: 40, y: 24, w: 400, h: 200, label: 'Guest VM', variant: 'safe' }],
  nodes: {
    code: { x: 60, y: 64, w: 360, h: 52, label: 'Untrusted agent code', sub: 'LLM-generated bash / scripts', variant: 'solid-danger' },
    guest: { x: 60, y: 156, w: 360, h: 52, label: 'Full guest OS', sub: 'own kernel, init, services, userland' },
    qemu: { x: 60, y: 248, w: 360, h: 52, label: 'QEMU', sub: 'broad device emulation, a host process', variant: 'solid-safe' },
    kernel: { x: 60, y: 340, w: 820, h: 52, label: 'Host Linux kernel + KVM', sub: 'hardware virtualization', variant: 'solid-dark' },
  },
  links: [],
  ips: [
    { id: 'IP-1', label: '1', points: [[240, 116], [240, 156]], dir: 'end', at: [292, 136] },
    { id: 'IP-2', label: '2', points: [[240, 208], [240, 248]], dir: 'both', at: [292, 228] },
    { id: 'IP-3', label: '3', points: [[420, 182], [800, 182], [800, 340]], dir: 'both', at: [610, 182] },
    { id: 'IP-4', label: '4', points: [[240, 300], [240, 340]], dir: 'end', at: [292, 320] },
  ],
  assets: [{ id: 'AS-1', label: '1', at: [880, 340] }],
  threats: [{ id: 'T1', label: '1', at: [188, 320] }],
};

export const vmText = {
  label: 'Block diagram of a standard QEMU/KVM virtual machine: untrusted code runs in a full guest operating system, and the guest reaches the host through emulated devices in QEMU and through hardware virtualization in KVM',
  hint: 'Circles are interaction points, the diamond is an asset, the square is a threat. Hover or click one to see what it connects to.',
  items: {
    'IP-1': {
      name: 'System calls go to the guest OS',
      desc: 'The code runs inside a complete guest operating system with its own kernel and services.',
    },
    'IP-2': {
      name: 'Emulated devices through QEMU',
      desc: 'QEMU emulates a broad range of devices for the guest. This is a larger device model than a minimal VMM, which is part of the cost of a general-purpose VM.',
    },
    'IP-3': {
      name: 'CPU and memory under hardware virtualization (KVM)',
      desc: 'The guest runs on the CPU under KVM, as in the microVM case. The same host requirement applies: access to /dev/kvm.',
    },
    'IP-4': {
      name: 'QEMU to host: its own syscalls',
      desc: 'QEMU is a host process and calls the host kernel. It can be confined further with the usual host mechanisms.',
    },
    'AS-1': {
      name: 'Host kernel and the node',
      desc: 'The same asset as before. The boundary is the hypervisor, as with a microVM.',
    },
    T1: {
      name: 'Flaw in QEMU device emulation or in KVM',
      desc: 'Same class of risk as in the microVM case.',
      links: ['IP-2', 'IP-3', 'IP-4', 'AS-1'],
    },
  },
};
