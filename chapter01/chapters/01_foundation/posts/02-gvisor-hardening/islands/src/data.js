// Short labels for the chips on the detail panel, keyed by id. Every id a
// panel shows as a chip (related hotspots) needs one here; the three diagrams
// of this post (groups/gvisor-hardening.js) share this table, so their ids
// are distinct: IP-1..6 / AS-1..2 / T1a..T1b are the runtime diagram, IP-R* /
// T-R* the runc capability diagram, IP-G* / T-G* the gVisor one.
export const REF_SHORT = {
  'IP-1': 'Syscalls',
  'IP-2': 'File access',
  'IP-3': 'Network',
  'IP-4': 'Gofer to host',
  'IP-5': 'Netstack to host',
  'IP-6': 'Runtime registration',
  'AS-1': 'Rootfs overlay',
  'AS-2': 'daemon.json',
  T1a: 'Sentry escape',
  T1b: 'Memory pressure',

  'IP-R1': 'Syscalls to host kernel',
  'T-R1': 'Kernel attack surface',
  'T-R2': 'Escape reaches the host',

  'IP-G1': 'Syscalls to Sentry',
  'IP-G2': 'Sentry to Gofer / Netstack',
  'IP-G3': 'Remaining host syscalls',
  'T-G1': 'Sentry bug',
  'T-G2': 'Residual host exposure',
};
