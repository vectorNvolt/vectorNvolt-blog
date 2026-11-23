import { test } from 'node:test';
import assert from 'node:assert/strict';
import ELK from 'elkjs/lib/elk.bundled.js';
import { parseNetlist } from '../src/netlist.js';
import { buildGraph } from '../src/schematic_graph.js';

const NET = parseNetlist(`RC
V1 in 0 PULSE(0 1 0 1u 1u 5m 10m)
R1 in out 4.7k
C1 out 0 100n
.end
`);

test('RC graph structure', () => {
  const { graph, meta } = buildGraph(NET);
  const o = graph.layoutOptions;
  assert.equal(o['elk.algorithm'], 'layered');
  assert.equal(o['elk.direction'], 'RIGHT');
  assert.equal(o['elk.edgeRouting'], 'ORTHOGONAL');

  const byId = Object.fromEntries(graph.children.map((c) => [c.id, c]));
  const elems = graph.children.filter((c) => meta.nodes[c.id].type === 'element');
  const gnds = graph.children.filter((c) => meta.nodes[c.id].type === 'gnd');
  assert.equal(elems.length, 3);
  assert.equal(gnds.length, 2); // one GND node per ground connection

  const [v1, r1, c1] = elems;
  assert.equal(v1.layoutOptions['elk.layered.layering.layerConstraint'], 'FIRST');
  assert.equal(r1.layoutOptions['elk.layered.layering.layerConstraint'], undefined);
  assert.equal(r1.layoutOptions['elk.portConstraints'], 'FIXED_SIDE');
  assert.deepEqual(r1.ports.map((p) => p.layoutOptions['elk.port.side']), ['WEST', 'EAST']);
  assert.deepEqual(c1.ports.map((p) => p.layoutOptions['elk.port.side']), ['NORTH', 'SOUTH']);
  assert.deepEqual(v1.ports.map((p) => p.layoutOptions['elk.port.side']), ['NORTH', 'SOUTH']);

  const nets = graph.edges.map((e) => meta.edges[e.id].net).sort();
  assert.deepEqual(nets, ['0', '0', 'in', 'out']); // each net here has <=2 ports -> one edge

  // every edge endpoint refers to an existing port
  const ports = new Set(graph.children.flatMap((c) => c.ports.map((p) => p.id)));
  for (const e of graph.edges) for (const p of [...e.sources, ...e.targets]) assert.ok(ports.has(p), p);
  assert.ok(byId[gnds[0].id]);

  const out = graph.edges.find((e) => meta.edges[e.id].net === 'out');
  assert.deepEqual(out.sources, [`${r1.id}.p1`]);
  assert.deepEqual(out.targets, [`${c1.id}.p0`]);
});

test('3-port net becomes simple edges, generic ports', () => {
  const n = parseNetlist('t\nR1 a b 1\nR2 a c 1\nR3 a d 1\nM1 b c d 0 nmos\n');
  const { graph, meta } = buildGraph(n);
  for (const e of graph.edges) {
    assert.equal(e.sources.length, 1);
    assert.equal(e.targets.length, 1);
  }
  const a = graph.edges.filter((e) => meta.edges[e.id].net === 'a');
  assert.equal(a.length, 2);
  assert.deepEqual(a.map((e) => e.id), ['n:a:1', 'n:a:2']);
  const m = graph.children.find((c) => meta.nodes[c.id].element?.kind === 'M');
  assert.equal(m.ports.length, 4);
  assert.equal(meta.nodes[m.id].orientation, null);
});

test('V sources sharing a net are not both FIRST; ELK layout succeeds', async () => {
  const n = parseNetlist('t\nVIN vin 0 DC 12\nVSENSE vin lx_in 0\nR1 lx_in 0 1k\n');
  const { graph } = buildGraph(n);
  const first = new Set(
    graph.children
      .filter((c) => c.layoutOptions['elk.layered.layering.layerConstraint'] === 'FIRST')
      .map((c) => c.id),
  );
  assert.ok(!first.has('e0'));
  assert.ok(!first.has('e1'));
  for (const e of graph.edges) {
    const src = e.sources[0].split('.')[0];
    const tgt = e.targets[0].split('.')[0];
    assert.ok(!(first.has(src) && first.has(tgt)), e.id);
  }
  const layout = await new ELK().layout(graph);
  assert.equal(layout.edges.length, graph.edges.length);
});

test('ELK layout resolves with 3-way and 4-way nets plus ground', async () => {
  const n = parseNetlist('t\nV1 in 0 1\nR1 in mid 1\nR2 mid out 1\nC1 mid 0 1\nC2 out 0 1\nR3 mid 0 1\n');
  const { graph, meta } = buildGraph(n);
  const layout = await new ELK().layout(graph);
  assert.equal(layout.edges.length, graph.edges.length);
  for (const e of layout.edges) {
    assert.ok(e.sections && e.sections.length > 0, e.id);
    assert.ok(meta.edges[e.id], e.id);
  }
});
