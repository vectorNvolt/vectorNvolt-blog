import { test } from 'node:test';
import assert from 'node:assert/strict';
import { parseRaw } from '../src/rawfile.js';

const VARS = [['time', 'time'], ['v(in)', 'voltage'], ['i(v1)', 'current']];

function header(npts, marker, flags = 'real', vars = VARS) {
  let h = `Title: test\nDate: now\nPlotname: Transient Analysis\nFlags: ${flags}\n`;
  h += `No. Variables: ${vars.length}\nNo. Points: ${npts}\nVariables:\n`;
  vars.forEach(([n, t], i) => (h += `\t${i}\t${n}\t${t}\n`));
  return h + marker + '\n';
}

function binaryRaw(points, flags) {
  const head = new TextEncoder().encode(header(points.length, 'Binary:', flags));
  const body = new ArrayBuffer(points.length * VARS.length * 8);
  const dv = new DataView(body);
  points.flat().forEach((x, i) => dv.setFloat64(i * 8, x, true));
  const out = new Uint8Array(head.length + body.byteLength);
  out.set(head, 0);
  out.set(new Uint8Array(body), head.length);
  return out.buffer;
}

const PTS = [[0, 0, -0.001], [1e-3, 0.5, -0.002], [2e-3, 1, -0.003]];

test('binary real, point-major', () => {
  const r = parseRaw(binaryRaw(PTS));
  assert.equal(r.title, 'test');
  assert.equal(r.plotname, 'Transient Analysis');
  assert.equal(r.flags, 'real');
  assert.deepEqual(r.vars, [
    { index: 0, name: 'time', type: 'time' },
    { index: 1, name: 'v(in)', type: 'voltage' },
    { index: 2, name: 'i(v1)', type: 'current' },
  ]);
  assert.ok(r.data[0] instanceof Float64Array);
  assert.deepEqual(Array.from(r.data[0]), [0, 1e-3, 2e-3]);
  assert.deepEqual(Array.from(r.data[1]), [0, 0.5, 1]);
  assert.deepEqual(Array.from(r.data[2]), [-0.001, -0.002, -0.003]);
});

test('binary accepts Uint8Array view with offset', () => {
  const b = new Uint8Array(binaryRaw(PTS));
  const padded = new Uint8Array(b.length + 3);
  padded.set(b, 3);
  const r = parseRaw(padded.subarray(3));
  assert.deepEqual(Array.from(r.data[1]), [0, 0.5, 1]);
});

test('ascii real', () => {
  let t = header(2, 'Values:');
  t += '0\t0.000000000000000e+00\n\t0.000000000000000e+00\n\t-1.0e-03\n';
  t += '1\t1.0e-03\n\t5.0e-01\n\t-2.0e-03\n';
  const r = parseRaw(new TextEncoder().encode(t).buffer);
  assert.deepEqual(Array.from(r.data[0]), [0, 1e-3]);
  assert.deepEqual(Array.from(r.data[1]), [0, 0.5]);
  assert.deepEqual(Array.from(r.data[2]), [-0.001, -0.002]);
});

test('ascii: only first plot is read', () => {
  let t = header(1, 'Values:') + '0\t1\n\t2\n\t3\n';
  t += header(1, 'Values:').replace('Title: test', 'Title: second') + '0\t9\n\t9\n\t9\n';
  const r = parseRaw(new TextEncoder().encode(t).buffer);
  assert.deepEqual(Array.from(r.data[1]), [2]);
});

test('rejects complex', () => {
  assert.throws(() => parseRaw(binaryRaw(PTS, 'complex')), /complex/);
});

test('rejects truncated binary', () => {
  assert.throws(() => parseRaw(binaryRaw(PTS).slice(0, -8)), /truncated/);
});

test('rejects missing data section', () => {
  assert.throws(() => parseRaw(new TextEncoder().encode('Title: x\n').buffer), /no 'Binary:'/);
});

test('rejects variable count mismatch', () => {
  const t = header(1, 'Values:', 'real', VARS).replace('No. Variables: 3', 'No. Variables: 4');
  assert.throws(() => parseRaw(new TextEncoder().encode(t).buffer), /expected 4/);
});
