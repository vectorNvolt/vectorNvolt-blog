import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  makeSignals, defaultSelection, nearestIndex, fmtSI, fmtPlain, netOfSignal,
  sourceSignal, signalMap, execStatus, pickNetlistInput, PALETTE,
} from '../src/signals.js';

const raw = {
  vars: [
    { index: 0, name: 'time', type: 'time' },
    { index: 1, name: 'v(in)', type: 'voltage' },
    { index: 2, name: 'v(out)', type: 'voltage' },
    { index: 3, name: 'i(v1)', type: 'current' },
  ],
};

test('signals, colors, default selection', () => {
  const s = makeSignals(raw);
  assert.deepEqual(s.map((x) => x.name), ['v(in)', 'v(out)', 'i(v1)']);
  assert.deepEqual(s.map((x) => x.index), [1, 2, 3]);
  assert.deepEqual(s.map((x) => x.axis), ['V', 'V', 'A']);
  assert.equal(s[0].color, PALETTE[0]);
  assert.deepEqual(defaultSelection(s), ['v(in)', 'v(out)']);
  const many = Array.from({ length: 5 }, (_, i) => ({ name: `v(n${i})`, type: 'voltage' }));
  assert.deepEqual(defaultSelection(many), []);
  assert.equal(sourceSignal(signalMap(s), 'V1').name, 'i(v1)');
  assert.equal(netOfSignal('V(Out)'), 'out');
  assert.equal(netOfSignal('i(v1)'), null);
});

test('nearestIndex', () => {
  const xs = [0, 1, 2, 4];
  assert.equal(nearestIndex(xs, -1), 0);
  assert.equal(nearestIndex(xs, 0.4), 0);
  assert.equal(nearestIndex(xs, 0.6), 1);
  assert.equal(nearestIndex(xs, 3.1), 3);
  assert.equal(nearestIndex(xs, 9), 3);
  assert.equal(nearestIndex([], 1), -1);
});

test('formatting', () => {
  assert.equal(fmtSI(0.0015, 's'), '1.5 ms');
  assert.equal(fmtSI(2e-6, 's'), '2 µs');
  assert.equal(fmtSI(0, 'V'), '0 V');
  assert.equal(fmtSI(1500, 'Hz'), '1.5 kHz');
  assert.equal(fmtPlain(0.63212, 'V'), '0.632 V');
});

test('exec status + netlist input', () => {
  assert.equal(execStatus(null).cls, 'run');
  assert.equal(execStatus({ exit: 0 }).cls, 'ok');
  assert.deepEqual(execStatus({ exit: 1, cause: 'timeout' }), { cls: 'fail', text: 'timeout' });
  assert.equal(pickNetlistInput(['foo', 'netlist']), 'netlist');
  assert.equal(pickNetlistInput(['foo']), 'foo');
  assert.equal(pickNetlistInput([]), null);
});
