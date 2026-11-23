import { useEffect, useMemo, useRef, useState } from 'preact/hooks';
import ELK from 'elkjs/lib/elk.bundled.js';
import { buildGraph } from '../schematic_graph.js';
import { Sym } from './Symbols.jsx';
import { fmtPlain, netOfSignal, netSignalKey, nearestIndex, sourceSignal } from '../signals.js';

const elk = new ELK(); // bundled build: no web worker, CSP-safe

const pkey = (p) => `${Math.round(p.x * 10)},${Math.round(p.y * 10)}`;

const onSegment = (p, a, b) =>
  (a.x === b.x ? p.x === a.x : a.y === b.y ? p.y === a.y : false) &&
  p.x >= Math.min(a.x, b.x) && p.x <= Math.max(a.x, b.x) &&
  p.y >= Math.min(a.y, b.y) && p.y <= Math.max(a.y, b.y);

// A net is drawn as several simple ELK edges. Collect, per net: all wire polylines, junction
// dots (ELK junctionPoints, points shared by >=2 edges, bends lying on another edge's segment;
// port positions excluded) and the longest segment (where the single net label goes).
function netGeometry(edges) {
  const lines = [];
  const perEdge = edges.map((e) => (e.sections || []).map((s) => [s.startPoint, ...(s.bendPoints || []), s.endPoint]));
  perEdge.forEach((ls) => lines.push(...ls));
  const ports = new Set();
  for (const ls of perEdge) for (const pts of ls) { ports.add(pkey(pts[0])); ports.add(pkey(pts[pts.length - 1])); }

  let best = null;
  let bestLen = -1;
  for (const pts of lines) {
    for (let i = 1; i < pts.length; i++) {
      const len = Math.abs(pts[i].x - pts[i - 1].x) + Math.abs(pts[i].y - pts[i - 1].y);
      if (len > bestLen) {
        bestLen = len;
        best = [pts[i - 1], pts[i]];
      }
    }
  }

  const junc = new Map();
  const add = (p) => { if (!ports.has(pkey(p))) junc.set(pkey(p), p); };
  for (const e of edges) for (const p of e.junctionPoints || []) add(p);
  const owners = new Map();
  perEdge.forEach((ls, ei) => ls.forEach((pts) => pts.forEach((p) => {
    const k = pkey(p);
    if (!owners.has(k)) owners.set(k, { p, set: new Set() });
    owners.get(k).set.add(ei);
  })));
  for (const { p, set } of owners.values()) if (set.size >= 2) add(p);
  perEdge.forEach((ls, ei) => ls.forEach((pts) => pts.forEach((p) => {
    perEdge.forEach((ls2, ej) => {
      if (ej === ei) return;
      for (const q of ls2) for (let i = 1; i < q.length; i++) if (onSegment(p, q[i - 1], q[i])) add(p);
    });
  })));
  return { lines, junctions: [...junc.values()], best };
}

export function Schematic({ netlist, netlistError, raw, sigMap, selected, hoverSig, toggleSignal, probeT, tText }) {
  const [state, setState] = useState({ status: 'idle' });
  const [tip, setTip] = useState(null);
  const tipTimer = useRef(0);

  useEffect(() => {
    if (!netlist) {
      setState({ status: 'idle' });
      return undefined;
    }
    if (!netlist.elements.length) {
      setState({ status: 'empty' });
      return undefined;
    }
    let dead = false;
    setState({ status: 'layout' });
    const { graph, meta } = buildGraph(netlist);
    elk
      .layout(graph)
      .then((layout) => !dead && setState({ status: 'ready', layout, meta }))
      .catch((e) => !dead && setState({ status: 'error', error: String(e && e.message ? e.message : e) }));
    return () => { dead = true; };
  }, [netlist]);

  const geo = useMemo(() => {
    if (state.status !== 'ready') return null;
    const byNet = new Map();
    for (const e of state.layout.edges || []) {
      const net = state.meta.edges[e.id].net;
      if (!byNet.has(net)) byNet.set(net, []);
      byNet.get(net).push(e);
    }
    // ground: one wire per GND node, no sharing, so it is geometry-wise independent per edge
    const edges = [...byNet].map(([net, es]) => ({ id: net, net, ...netGeometry(es) }));
    return { edges };
  }, [state]);

  const showTip = (x, y, text) => {
    setTip({ x, y, text });
    clearTimeout(tipTimer.current);
    tipTimer.current = setTimeout(() => setTip(null), 1600);
  };
  useEffect(() => () => clearTimeout(tipTimer.current), []);

  const hoverNet = hoverSig ? netOfSignal(hoverSig) : null;
  const probe = (sig) => {
    if (!raw || !sig) return null;
    const xs = raw.data[0];
    if (!xs.length) return null;
    const i = nearestIndex(xs, probeT == null ? xs[xs.length - 1] : probeT);
    return raw.data[sig.index][i];
  };

  const clickNet = (net, ev, at) => {
    const sig = sigMap.get(netSignalKey(net));
    if (sig) toggleSignal(sig.name);
    else showTip(at.x, at.y, `v(${net}): not in sim.raw`);
    ev.stopPropagation();
  };
  const clickSource = (el, ev, at) => {
    const sig = sourceSignal(sigMap, el.name);
    if (sig) toggleSignal(sig.name);
    else showTip(at.x, at.y, `i(${el.name.toLowerCase()}): not in sim.raw`);
    ev.stopPropagation();
  };

  let body;
  if (netlistError) body = <div class="msg">Schematic unavailable: {netlistError}</div>;
  else if (!netlist) body = <div class="msg">No netlist input for this exec.</div>;
  else if (state.status === 'empty') body = <div class="msg">The netlist has no drawable elements.</div>;
  else if (state.status === 'error') body = <div class="msg err">Layout failed: {state.error}</div>;
  else if (state.status !== 'ready') body = <div class="msg">Laying out schematic...</div>;
  else {
    const { layout, meta } = state;
    body = (
      <svg class="sch" viewBox={`0 0 ${layout.width || 100} ${layout.height || 100}`} preserveAspectRatio="xMidYMid meet" role="img" aria-label="schematic">
        <g class="wires">
          {geo.edges.map((e) => {
            const gnd = e.net === '0';
            const sig = gnd ? null : sigMap.get(netSignalKey(e.net));
            const on = !!sig && selected.has(sig.name);
            const hl = hoverNet === e.net;
            const style = on ? { stroke: sig.color } : undefined;
            const cls = 'wire' + (on ? ' on' : '') + (hl ? ' hl' : '');
            return (
              <g key={e.id}>
                {e.lines.map((pts, i) => {
                  const d = pts.map((p) => `${p.x},${p.y}`).join(' ');
                  return (
                    <g key={i}>
                      <polyline class={cls} style={style} points={d} fill="none" />
                      {!gnd && (
                        <polyline
                          class="hit"
                          points={d}
                          fill="none"
                          onClick={(ev) => clickNet(e.net, ev, e.best[0])}
                        />
                      )}
                    </g>
                  );
                })}
                {e.junctions.map((p, i) => (
                  <circle key={i} class="junction" cx={p.x} cy={p.y} r={on ? 4 : 3} style={on ? { fill: sig.color } : undefined} />
                ))}
                {!gnd && e.best && (() => {
                  const [a, b] = e.best;
                  const vertical = a.x === b.x;
                  const x = (a.x + b.x) / 2 + (vertical ? 6 : 0);
                  const y = (a.y + b.y) / 2 + (vertical ? 4 : -6);
                  const v = probe(sig);
                  return (
                    <text
                      class={'netlabel' + (on ? ' on' : '') + (hl ? ' hl' : '')}
                      x={x}
                      y={y}
                      text-anchor={vertical ? 'start' : 'middle'}
                      onClick={(ev) => clickNet(e.net, ev, { x, y })}
                    >
                      {e.net}
                      {v != null && (
                        <tspan class="tag" dx="6" style={sig && on ? { fill: sig.color } : undefined}>
                          {fmtPlain(v, 'V')}
                        </tspan>
                      )}
                    </text>
                  );
                })()}
              </g>
            );
          })}
        </g>
        <g class="elements">
          {layout.children.map((c) => {
            const m = meta.nodes[c.id];
            if (m.type === 'gnd') {
              return (
                <g key={c.id} class="sym gnd" transform={`translate(${c.x},${c.y})`}>
                  <Sym kind="GND" w={c.width} h={c.height} />
                </g>
              );
            }
            const el = m.element;
            const src = el.kind === 'V' || el.kind === 'I';
            const sig = src ? sourceSignal(sigMap, el.name) : null;
            const on = !!sig && selected.has(sig.name);
            const generic = !m.orientation;
            const vertical = m.orientation === 'v';
            const at = { x: c.x + c.width / 2, y: c.y };
            return (
              <g
                key={c.id}
                class={'sym' + (src ? ' source' : '') + (on ? ' on' : '')}
                style={on ? { color: sig.color } : undefined}
                transform={`translate(${c.x},${c.y})`}
                onClick={src ? (ev) => clickSource(el, ev, at) : undefined}
              >
                {src && <rect class="hit" x={0} y={0} width={c.width} height={c.height} />}
                <Sym kind={generic ? 'BOX' : el.kind} w={c.width} h={c.height} />
                {generic && (c.ports || []).map((p, i) => {
                  const west = p.x < c.width / 2;
                  return (
                    <text key={i} class="pin" x={p.x + (west ? 5 : -5)} y={p.y + 3} text-anchor={west ? 'start' : 'end'}>
                      {el.nodes[i] === '0' ? 'gnd' : el.nodes[i]}
                    </text>
                  );
                })}
                {generic ? (
                  <>
                    <text class="name" x={c.width / 2} y={c.height / 2 - 2} text-anchor="middle">{el.name}</text>
                    <text class="value" x={c.width / 2} y={c.height / 2 + 11} text-anchor="middle">{el.value}</text>
                  </>
                ) : vertical ? (
                  <>
                    <text class="name" x={c.width + 6} y={c.height / 2 - 2}>{el.name}</text>
                    <text class="value" x={c.width + 6} y={c.height / 2 + 11}>{el.value}</text>
                  </>
                ) : (
                  <>
                    <text class="name" x={c.width / 2} y={-14} text-anchor="middle">{el.name}</text>
                    <text class="value" x={c.width / 2} y={c.height + 14} text-anchor="middle">{el.value}</text>
                  </>
                )}
              </g>
            );
          })}
        </g>
        {tip && (
          <text class="tip" x={tip.x} y={tip.y - 8} text-anchor="middle">{tip.text}</text>
        )}
      </svg>
    );
  }

  return (
    <section class="panel schematic">
      <header>
        <h2>Schematic</h2>
        {netlist && netlist.title && <span class="sub">{netlist.title}</span>}
        <span class="grow" />
        {tText && <span class="tval">{tText}</span>}
      </header>
      <div class="panel-body">{body}</div>
    </section>
  );
}
