// Pure helpers shared by the UI: signal model, colors, SI formatting, probes, exec status.

// Fixed 8-color palette, mid-tone so it reads on both dark and light backgrounds.
export const PALETTE = ['#4c9be8', '#e8743b', '#2fb67c', '#d6559b', '#b08cf0', '#d9a81c', '#26b5c9', '#e5534b'];

/**
 * Signals = every rawfile var except the scale (index 0). Color is assigned by position among
 * the signals, so it is stable for a given rawfile and does not change with the selection.
 * axis: 'A' for current, 'V' for everything else.
 */
export function makeSignals(raw) {
  return raw.vars.slice(1).map((v, i) => ({
    name: v.name,
    key: v.name.toLowerCase(),
    type: v.type,
    index: i + 1, // position in raw.vars / raw.data
    axis: v.type === 'current' ? 'A' : 'V',
    color: PALETTE[i % PALETTE.length],
  }));
}

export function signalMap(signals) {
  return new Map(signals.map((s) => [s.key, s]));
}

export function defaultSelection(signals) {
  const volts = signals.filter((s) => s.type === 'voltage');
  return volts.length <= 4 ? volts.map((s) => s.name) : [];
}

export const netSignalKey = (net) => `v(${net.toLowerCase()})`;

/** ngspice names a source current i(v1) (newer) or v1#branch (older). */
export function sourceSignal(map, name) {
  const l = name.toLowerCase();
  return map.get(`i(${l})`) || map.get(`${l}#branch`) || null;
}

/** Net name for a v(<net>) signal name, or null. */
export function netOfSignal(name) {
  const m = /^v\((.+)\)$/i.exec(name);
  return m ? m[1].toLowerCase() : null;
}

/** Index of the sample in ascending xs nearest to t. */
export function nearestIndex(xs, t) {
  const n = xs.length;
  if (!n) return -1;
  if (t <= xs[0]) return 0;
  if (t >= xs[n - 1]) return n - 1;
  let lo = 0;
  let hi = n - 1;
  while (hi - lo > 1) {
    const mid = (lo + hi) >> 1;
    if (xs[mid] <= t) lo = mid;
    else hi = mid;
  }
  return t - xs[lo] <= xs[hi] - t ? lo : hi;
}

const PREFIXES = [['G', 1e9], ['M', 1e6], ['k', 1e3], ['', 1], ['m', 1e-3], ['µ', 1e-6], ['n', 1e-9], ['p', 1e-12], ['f', 1e-15]];

/** 0.0015, 's' -> "1.5 ms" */
export function fmtSI(v, unit = '') {
  if (v === null || v === undefined || Number.isNaN(v)) return '';
  if (v === 0) return '0 ' + unit;
  const a = Math.abs(v);
  let pick = PREFIXES[PREFIXES.length - 1];
  for (const p of PREFIXES) {
    if (a >= p[1] * 0.9995) {
      pick = p;
      break;
    }
  }
  return `${Number((v / pick[1]).toPrecision(3))} ${pick[0]}${unit}`;
}

/** Plain value tag for probes, e.g. "0.632 V". */
export function fmtPlain(v, unit) {
  return `${Number(v.toPrecision(3))} ${unit}`;
}

export function xUnit(type) {
  return type === 'time' ? 's' : type === 'frequency' ? 'Hz' : '';
}

/** Badge info for an exec list entry. */
export function execStatus(result) {
  if (!result) return { cls: 'run', text: 'no result' };
  if (result.exit === 0 && !result.cause) return { cls: 'ok', text: 'ok' };
  return { cls: 'fail', text: result.cause || `exit ${result.exit}` };
}

/** Pick the netlist input name: first starting with "netlist", else first input. */
export function pickNetlistInput(inputs) {
  const list = inputs || [];
  return list.find((n) => n.toLowerCase().startsWith('netlist')) || list[0] || null;
}
