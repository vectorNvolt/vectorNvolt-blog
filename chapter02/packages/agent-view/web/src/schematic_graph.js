// Netlist -> ELK graph (pure; layout is done by the caller with elkjs).
// Each element is a node with ports; a non-ground net with ports p0..pn becomes n simple
// edges p0->pk (ELK layered only accepts 1 source / 1 target per edge; mergeEdges makes the
// star render like a hyperedge); every ground connection gets its own small GND node so
// the drawing stays clean.
import { GROUND } from './netlist.js';

export const LAYOUT_OPTIONS = {
  'elk.algorithm': 'layered',
  'elk.direction': 'RIGHT',
  'elk.edgeRouting': 'ORTHOGONAL',
  'elk.spacing.nodeNode': '50',
  'elk.spacing.edgeNode': '24',
  'elk.spacing.edgeEdge': '16',
  'elk.layered.spacing.nodeNodeBetweenLayers': '70',
  'elk.layered.spacing.edgeNodeBetweenLayers': '24',
  'elk.layered.spacing.edgeEdgeBetweenLayers': '16',
  'elk.layered.mergeEdges': 'true',
  'elk.padding': '[top=30,left=30,bottom=30,right=30]',
};

/** 'h' = left/right terminals, 'v' = top/bottom terminals, null = generic box. */
export function orientation(kind) {
  if ('RLD'.includes(kind)) return 'h';
  if ('CVI'.includes(kind)) return 'v';
  return null;
}

const SIZES = { R: [80, 30], L: [80, 30], D: [70, 30], C: [30, 64], V: [40, 72], I: [40, 72] };
export const GND_SIZE = [24, 16];

export function buildGraph(netlist) {
  const children = [];
  const edges = [];
  const meta = { nodes: {}, edges: {} };
  const netPorts = new Map();

  // ELK throws if a FIRST node has an incoming edge from a non-FIRST_SEPARATE node, so only pin
  // V/I sources whose non-ground nets are touched by no other V/I and whose terminals differ.
  const isSource = (el) => el.kind === 'V' || el.kind === 'I';
  const srcPerNet = new Map();
  for (const el of netlist.elements) {
    if (!isSource(el)) continue;
    for (const net of new Set(el.nodes)) if (net !== GROUND) srcPerNet.set(net, (srcPerNet.get(net) || 0) + 1);
  }
  const canBeFirst = (el) =>
    isSource(el) &&
    new Set(el.nodes).size === el.nodes.length &&
    el.nodes.every((net) => net === GROUND || srcPerNet.get(net) === 1);

  netlist.elements.forEach((el, idx) => {
    const id = `e${idx}`;
    const o = orientation(el.kind);
    const n = el.nodes.length;
    let w;
    let h;
    const sides = [];
    if (o) {
      [w, h] = SIZES[el.kind];
      sides.push(...(o === 'h' ? ['WEST', 'EAST'] : ['NORTH', 'SOUTH']));
    } else {
      const west = Math.ceil(n / 2);
      const east = n - west;
      w = 90;
      h = Math.max(50, 24 * Math.max(west, east) + 20);
      for (let i = 0; i < n; i++) sides.push(i < west ? 'WEST' : 'EAST');
    }
    const node = {
      id,
      width: w,
      height: h,
      layoutOptions: { 'elk.portConstraints': 'FIXED_SIDE' },
      ports: el.nodes.map((net, i) => ({
        id: `${id}.p${i}`,
        width: 0,
        height: 0,
        layoutOptions: { 'elk.port.side': sides[i], 'elk.port.index': String(i) },
      })),
    };
    if (canBeFirst(el)) node.layoutOptions['elk.layered.layering.layerConstraint'] = 'FIRST';
    children.push(node);
    meta.nodes[id] = { type: 'element', element: el, orientation: o };
    el.nodes.forEach((net, i) => {
      const pid = `${id}.p${i}`;
      if (net === GROUND) {
        const gid = `g${id}.p${i}`;
        children.push({
          id: gid,
          width: GND_SIZE[0],
          height: GND_SIZE[1],
          layoutOptions: { 'elk.portConstraints': 'FIXED_SIDE' },
          ports: [{ id: `${gid}.p`, width: 0, height: 0, layoutOptions: { 'elk.port.side': 'NORTH' } }],
        });
        meta.nodes[gid] = { type: 'gnd' };
        const eid = `eg.${id}.p${i}`;
        edges.push({ id: eid, sources: [pid], targets: [`${gid}.p`] });
        meta.edges[eid] = { net: GROUND };
      } else {
        if (!netPorts.has(net)) netPorts.set(net, []);
        netPorts.get(net).push(pid);
      }
    });
  });

  for (const [net, ports] of netPorts) {
    if (ports.length < 2) continue;
    for (let k = 1; k < ports.length; k++) {
      const eid = `n:${net}:${k}`;
      edges.push({ id: eid, sources: [ports[0]], targets: [ports[k]] });
      meta.edges[eid] = { net };
    }
  }

  return { graph: { id: 'root', layoutOptions: { ...LAYOUT_OPTIONS }, children, edges }, meta };
}
