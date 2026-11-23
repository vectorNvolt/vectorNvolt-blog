// Minimal SPICE netlist parser for drawing. Robust over exact.
//
// - line 1 is the title; `*` lines are comments; `;` starts an inline comment;
//   `+` lines continue the previous logical line; names/nodes are case-insensitive
//   (nodes are lowercased; element names keep their original case).
// - dot-cards are collected in `cards` (never drawn). Elements inside .subckt ... .ends
//   are skipped, so are .control ... .endc command blocks. Parsing stops after .end.
// - R C L V I D: two terminals = first two tokens after the name; value = remaining tokens
//   joined by a space (diode: model name).
// - K (coupling) has no nodes -> skipped.
// - everything else is a generic element. Node count by kind when known
//   (M4 Q3 J3 Z3 E4 G4 F2 H2 B2 S4 W2 T4 O4), clipped at the first token containing '=' or '('.
//   X: nodes = tokens before the first token containing '=', minus the last one (subckt name = value).
//   Unknown kinds: tokens before the first '=' or '(' token; if more than one, the last is the value.
// - node "0" or "gnd" is ground and is normalised to "0".

export const GROUND = '0';
const TWO_TERMINAL = new Set(['R', 'C', 'L', 'V', 'I', 'D']);
const NODE_COUNT = { M: 4, Q: 3, J: 3, Z: 3, E: 4, G: 4, F: 2, H: 2, B: 2, S: 4, W: 2, T: 4, O: 4 };

export function normNode(n) {
  const l = n.toLowerCase();
  return l === 'gnd' ? GROUND : l;
}

/** Shorten long values: `PULSE(0 1 ...)` -> `PULSE(...)`; otherwise truncate with an ellipsis. */
export function shortenValue(v, max = 18) {
  if (v.length <= max) return v;
  const p = v.indexOf('(');
  if (p > 0) return v.slice(0, p).trim() + '(...)';
  return v.slice(0, max - 1) + '…';
}

export function parseNetlist(text) {
  const raw = String(text).replace(/\r\n?/g, '\n').split('\n');
  const title = (raw[0] || '').trim();
  const logical = [];
  for (let i = 1; i < raw.length; i++) {
    let ln = raw[i];
    if (/^\s*\*/.test(ln)) continue;
    const sc = ln.indexOf(';');
    if (sc >= 0) ln = ln.slice(0, sc);
    if (!ln.trim()) continue;
    if (/^\s*\+/.test(ln) && logical.length) {
      logical[logical.length - 1] += ' ' + ln.replace(/^\s*\+/, '').trim();
    } else {
      logical.push(ln.trim());
    }
  }

  const elements = [];
  const cards = [];
  let depth = 0;
  let control = false; // .control ... .endc holds ngspice commands, never elements
  for (const line of logical) {
    const w = line.split(/\s+/)[0].toLowerCase();
    if (control || w === '.control') {
      cards.push(line);
      control = w !== '.endc';
      continue;
    }
    if (line[0] === '.') {
      cards.push(line);
      if (w === '.subckt') depth++;
      else if (w === '.ends') depth = Math.max(0, depth - 1);
      else if (w === '.end') break;
      continue;
    }
    if (depth > 0) continue;
    const tok = line.split(/\s+/);
    const name = tok[0];
    const kind = name[0].toUpperCase();
    if (kind === 'K') continue;
    if (TWO_TERMINAL.has(kind)) {
      if (tok.length < 3) continue;
      const full = tok.slice(3).join(' ');
      elements.push({ name, kind, nodes: [normNode(tok[1]), normNode(tok[2])], value: shortenValue(full), full });
      continue;
    }
    const rest = tok.slice(1);
    let nodes;
    let full = '';
    if (kind === 'X') {
      const eq = rest.findIndex((t) => t.includes('='));
      const head = eq < 0 ? rest : rest.slice(0, eq);
      if (head.length < 2) continue;
      full = head[head.length - 1];
      nodes = head.slice(0, -1);
    } else {
      const cut = rest.findIndex((t) => t.includes('=') || t.includes('('));
      const head = cut < 0 ? rest : rest.slice(0, cut);
      if (NODE_COUNT[kind]) {
        const n = Math.min(NODE_COUNT[kind], head.length);
        nodes = head.slice(0, n);
        full = rest.slice(n).join(' ');
      } else if (head.length > 1) {
        nodes = head.slice(0, -1);
        full = rest.slice(nodes.length).join(' ');
      } else {
        nodes = head;
      }
    }
    if (!nodes.length) continue;
    elements.push({ name, kind, nodes: nodes.map(normNode), value: shortenValue(full), full });
  }

  const nets = [];
  const seen = new Set([GROUND]);
  for (const e of elements) {
    for (const n of e.nodes) {
      if (!seen.has(n)) {
        seen.add(n);
        nets.push(n);
      }
    }
  }
  return { title, elements, nets, cards };
}
