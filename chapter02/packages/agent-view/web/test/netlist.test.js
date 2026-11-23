import { test } from 'node:test';
import assert from 'node:assert/strict';
import { parseNetlist, shortenValue } from '../src/netlist.js';

// howto/manual_test/agent-exec/src/ngspice/rc_design/bench.cir with <R>=4.7k <C>=100n
const BENCH = `RC low-pass check
V1 in 0 PULSE(0 1 0 1u 1u 5m 10m)
R1 in out 4.7k
C1 out 0 100n
.meas tran tau trig v(in) val=0.5 rise=1 targ v(out) val=0.632 rise=1
.meas tran v5m find v(out) at=4.99m
.meas tran v10m find v(out) at=9.99m
.tran 5u 10m
.end
`;

test('bench.cir', () => {
  const n = parseNetlist(BENCH);
  assert.equal(n.title, 'RC low-pass check');
  assert.deepEqual(
    n.elements.map((e) => [e.name, e.kind, e.nodes, e.value]),
    [
      ['V1', 'V', ['in', '0'], 'PULSE(...)'],
      ['R1', 'R', ['in', 'out'], '4.7k'],
      ['C1', 'C', ['out', '0'], '100n'],
    ],
  );
  assert.equal(n.elements[0].full, 'PULSE(0 1 0 1u 1u 5m 10m)');
  assert.deepEqual(n.nets, ['in', 'out']);
  assert.equal(n.cards.length, 5);
  assert.equal(n.cards[3], '.tran 5u 10m');
});

test('comments, continuation, case, gnd', () => {
  const n = parseNetlist(
    `title line
* a comment
r1 IN Out 1k ; inline comment
Vdd IN GND
+ PWL(0 0 1m 5
* comment in the middle
+ 2m 0)
C1 OUT gnd
+ 10n
.END
R9 x y 1
`,
  );
  assert.equal(n.title, 'title line');
  assert.deepEqual(n.elements.map((e) => e.name), ['r1', 'Vdd', 'C1']);
  assert.deepEqual(n.elements[0].nodes, ['in', 'out']);
  assert.equal(n.elements[0].value, '1k');
  assert.deepEqual(n.elements[1].nodes, ['in', '0']);
  assert.equal(n.elements[1].full, 'PWL(0 0 1m 5 2m 0)');
  assert.deepEqual(n.elements[2].nodes, ['out', '0']);
  assert.equal(n.elements[2].value, '10n');
  assert.deepEqual(n.nets, ['in', 'out']);
  assert.deepEqual(n.cards, ['.END']);
});

test('generic elements and K', () => {
  const n = parseNetlist(
    `t
M1 d g s b nmos W=1u L=0.1u
Q1 c b e npn
X1 a b c opamp GAIN=10
E1 o 0 i 0 2
L1 a b 1m
L2 c d 1m
K1 L1 L2 0.9
.subckt foo p q
R5 p q 1
.ends
`,
  );
  const by = Object.fromEntries(n.elements.map((e) => [e.name, e]));
  assert.deepEqual(by.M1.nodes, ['d', 'g', 's', 'b']);
  assert.equal(by.M1.value, 'nmos W=1u L=0.1u');
  assert.deepEqual(by.Q1.nodes, ['c', 'b', 'e']);
  assert.deepEqual(by.X1.nodes, ['a', 'b', 'c']);
  assert.equal(by.X1.value, 'opamp');
  assert.deepEqual(by.E1.nodes, ['o', '0', 'i', '0']);
  assert.equal(by.K1, undefined);
  assert.equal(by.R5, undefined);
  assert.equal(n.cards.length, 2);
});

test('shortenValue', () => {
  assert.equal(shortenValue('4.7k'), '4.7k');
  assert.equal(shortenValue('SIN(0 1 1k 0 0 0)'), 'SIN(0 1 1k 0 0 0)');
  assert.equal(shortenValue('PULSE(0 1 0 1u 1u 5m 10m)'), 'PULSE(...)');
  assert.equal(shortenValue('averyveryverylongmodelname').length, 18);
});

test('.control block is commands, not elements (rcd2 regression)', () => {
  const src = 'RC\nV1 in 0 PULSE(0 1 0 1u 1u 5m 10m)\nR1 in out 10k\nC1 out 0 47n\n.tran 5u 10m\n.CONTROL\nrun\nmeas tran v5m find v(out) at=4.99m\n.endc\nR9 out 0 1k\n.end\n';
  const n = parseNetlist(src);
  assert.deepEqual(n.elements.map((e) => e.name), ['V1', 'R1', 'C1', 'R9']);
  assert.ok(n.cards.includes('run'));
});
