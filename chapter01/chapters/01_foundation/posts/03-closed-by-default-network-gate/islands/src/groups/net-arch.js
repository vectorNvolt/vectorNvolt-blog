// The isolation architecture of an agent container's network path, drawn once
// by the generic BlockDiagram (top to bottom):
//   `block` / `blockText` → BlockDiagram.jsx (hand laid out, viewBox units)
// Geometry and structure first, every word the panels show second. Chip
// labels for the ids named in `links` come from REF_SHORT in ../data.js.
// Layers are told apart by box variant (see the "Layer key" in the drawing):
//   solid-dark = kernel netdev hooks (nftables)
//   solid-safe = bridge L2 port lock (iproute2)
//   default    = L3/L4 filtering (iptables)
// A numbered circle sits on the connector that leads into each stage and
// carries that stage's text.

export const block = {
  viewH: 912,
  outlines: [
    { x: 200, y: 16, w: 360, h: 96, label: 'Agent container · untrusted', variant: 'danger' },
    { x: 610, y: 16, w: 320, h: 134, label: 'Layer key' },
    { x: 20, y: 160, w: 920, h: 660, label: 'Host · kernel and network namespace' },
    { x: 40, y: 400, w: 880, h: 230, label: 'Bridge agents-net · 172.30.0.0/24' },
  ],
  nodes: {
    keyNetdev: { x: 630, y: 42, w: 280, h: 30, label: 'Kernel netdev hooks (nftables)', variant: 'solid-dark', small: true },
    keyBridge: { x: 630, y: 78, w: 280, h: 30, label: 'Bridge L2 port lock (iproute2)', variant: 'solid-safe', small: true },
    keyIptables: { x: 630, y: 114, w: 280, h: 30, label: 'L3/L4 filtering (iptables)', small: true },
    agent: { x: 230, y: 46, w: 300, h: 54, label: 'Agent (gVisor guest)', sub: 'guest eth0', variant: 'solid-danger' },
    veth: { x: 230, y: 190, w: 300, h: 44, label: 'Host veth', sub: 'bridge port of the container' },
    port: {
      x: 230,
      y: 298,
      w: 300,
      h: 84,
      label: 'Per-port table as_port_<id>',
      sub: ['netdev nftables · policy drop', 'ingress + egress hooks on the veth', 'ingress accepts set mark 0x10000000'],
      variant: 'solid-dark',
    },
    lock: {
      x: 230,
      y: 470,
      w: 300,
      h: 104,
      label: 'Port controls and neighbour entry',
      sub: ['veth: isolated on · learning off · locked on', 'veth: static FDB entry', 'bridge: permanent neighbour entry'],
      variant: 'solid-safe',
    },
    gate: {
      x: 620,
      y: 470,
      w: 280,
      h: 104,
      label: 'agents_gate (netdev)',
      sub: ['policy drop · released = MAC + name', 'ingress: saddr in released + mark', 'then the gate clears the mark bit', 'egress: daddr in released'],
      variant: 'solid-dark',
    },
    iptables: {
      x: 590,
      y: 680,
      w: 340,
      h: 104,
      label: 'iptables DOCKER-USER / INPUT',
      sub: [
        'rules keyed by container IP, tag agent-net:<name>',
        'ACCEPT DNS 1.1.1.1 and 9.9.9.9 · model IP tcp/443',
        'DROP all other traffic of the container IP',
        'agent-net:gate: physdev DROP between ports',
      ],
    },
    internet: { x: 610, y: 840, w: 300, h: 52, label: 'Public internet', sub: 'outside the host', variant: 'solid-danger' },
  },
  links: [],
  ips: [
    { id: 'IP-1', label: '1', points: [[380, 100], [380, 190]], dir: 'end', at: [380, 130] },
    { id: 'IP-2', label: '2', points: [[380, 234], [380, 298]], dir: 'end', at: [380, 266] },
    { id: 'IP-3', label: '3', points: [[380, 382], [380, 470]], dir: 'end', at: [380, 436] },
    { id: 'IP-4', label: '4', points: [[530, 522], [620, 522]], dir: 'end', at: [575, 522] },
    { id: 'IP-5', label: '5', points: [[760, 574], [760, 680]], dir: 'end', at: [760, 655] },
    { id: 'IP-6', label: '6', points: [[760, 784], [760, 840]], dir: 'end', at: [760, 815] },
  ],
  assets: [],
  threats: [],
};

export const blockText = {
  label:
    'Block diagram of the network isolation architecture, top to bottom: the agent container, the host boundary and veth, the per-port netdev table, the agents-net bridge with its port controls, neighbour entry and agents_gate table, the iptables rules in DOCKER-USER and INPUT, and the public internet',
  hint: 'Numbered circles mark each stage of the path from the agent to the public internet; hover or click one to see what the stage is configured to do. Box colour shows the layer: kernel netdev hooks (dark), bridge L2 port lock (green), iptables L3/L4 filtering (white).',
  items: {
    'IP-1': {
      name: 'Host boundary: guest eth0 to host veth',
      desc: [
        '• Where: the agent runs in a gVisor container whose only network interface is the guest eth0.',
        '• Link: eth0 arrives on a veth on the host.',
        '• Effect: every control in this diagram is applied on the host side of that veth, so it is intended to hold regardless of what runs inside the guest.',
      ].join('\n'),
      links: [],
    },
    'IP-2': {
      name: 'Per-port table as_port_<id> (kernel netdev hooks)',
      desc: [
        '• Table: a netdev nftables table with ingress and egress hooks bound to this container’s veth.',
        '• Initial state: loaded with policy drop and no accept rule, so it is intended to drop every frame on that port.',
        '• Release: the release step adds accept rules for the agent’s own MAC and IP.',
        '• Mark: the ingress accept rules also set the mark bit 0x10000000 on every frame they accept.',
        '• Gate dependency: the gate on the bridge requires that bit, so only a live port table can open the way.',
        '• Restart: a restarted container gets a new veth with no port table, its frames carry no mark, and the gate drops them.',
      ].join('\n'),
      code: 'agent-net: s10_close_port.go (ClosePort)',
      links: [],
    },
    'IP-3': {
      name: 'Port controls on the veth, neighbour entry on the bridge (iproute2)',
      desc: [
        '• On the veth: isolated on, learning off and locked on, plus a static FDB entry for the agent’s MAC.',
        '• On the bridge: a permanent neighbour entry that binds the agent’s IP to that MAC.',
        '• Purpose: these settings are configured to keep the agent from reaching other bridge ports and from taking over another address.',
      ].join('\n'),
      code: 'agent-net: s13_apply_controls.go (ApplyControls)',
      links: [],
    },
    'IP-4': {
      name: 'agents_gate (kernel netdev hooks on the bridge)',
      desc: [
        '• Table: a netdev nftables table bound to the bridge device with policy drop.',
        '• Ingress (from the container): passes only a frame whose source MAC is in the set released and that carries the mark bit from its own port table.',
        '• Mark: the gate then clears the bit, so it never reaches host routing or iptables.',
        '• Egress (towards the container): passes by destination MAC in released.',
        '• Elements: each element of released carries the container name as its comment.',
        '• Closed by default: a container is intended to stay closed from the moment it is attached until it is armed.',
        '• Restart: an unmarked frame from a restarted container is dropped even if its MAC is still in the set.',
      ].join('\n'),
      code: 'agent-net: s36_install_gate.go',
      links: [],
    },
    'IP-5': {
      name: 'iptables DOCKER-USER and INPUT (L3/L4)',
      desc: [
        '• Keying: rules are keyed by the container IP and tagged agent-net:<name>.',
        '• Accept: its IP towards the DNS resolvers 1.1.1.1 and 9.9.9.9 (port 53) and towards the model API addresses on TCP 443.',
        '• Drop: the rest of its traffic leaving the bridge.',
        '• INPUT: configured to accept only replies to host-initiated flows and to drop other packets from the container IP.',
        '• Bridged traffic: a separate rule tagged agent-net:gate drops it between ports (physdev).',
      ].join('\n'),
      code: 'agent-net: s13_apply_controls.go (iptPlan)',
      links: [],
    },
    'IP-6': {
      name: 'Public internet',
      desc: [
        '• Reachable: only the allowed DNS resolvers and the model API addresses are intended to be reachable.',
        '• Timing: only after the port has been released.',
        '• Everything else: traffic to any other destination is configured to be dropped by the rules above.',
      ].join('\n'),
      links: [],
    },
  },
};
