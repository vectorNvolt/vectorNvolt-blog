import { useEffect, useRef, useState } from 'preact/hooks';
import uPlot from 'uplot';
import { fmtSI, xUnit } from '../signals.js';

const css = (name, fallback) => getComputedStyle(document.documentElement).getPropertyValue(name).trim() || fallback;

export function Waveform({ raw, signals, selected, onCursor }) {
  const box = useRef(null);
  const host = useRef(null);
  const plot = useRef(null);
  const cursorCb = useRef(onCursor);
  cursorCb.current = onCursor;
  const [scheme, setScheme] = useState(0);

  const sel = signals.filter((s) => selected.has(s.name));
  const key = sel.map((s) => s.name).join('|');

  useEffect(() => {
    const mq = window.matchMedia('(prefers-color-scheme: dark)');
    const f = () => setScheme((x) => x + 1);
    mq.addEventListener('change', f);
    return () => mq.removeEventListener('change', f);
  }, []);

  const fit = () => {
    const u = plot.current;
    if (!u || !box.current) return;
    const legend = u.root.querySelector('.u-legend');
    const w = box.current.clientWidth;
    const h = Math.max(140, box.current.clientHeight - (legend ? legend.offsetHeight : 0) - 4);
    if (w > 0) u.setSize({ width: w, height: h });
  };

  useEffect(() => {
    if (plot.current) {
      plot.current.destroy();
      plot.current = null;
    }
    cursorCb.current(null);
    if (!sel.length || !host.current) return undefined;
    const fg = css('--muted', '#888');
    const grid = css('--grid', 'rgba(128,128,128,0.25)');
    const xu = xUnit(raw.vars[0].type);
    const hasV = sel.some((s) => s.axis === 'V');
    const hasA = sel.some((s) => s.axis === 'A');
    const yAxis = (scale, unit, side, showGrid) => ({
      scale,
      side,
      stroke: fg,
      size: 70,
      grid: { show: showGrid, stroke: grid, width: 1 },
      ticks: { stroke: grid, width: 1 },
      values: (u, vs) => vs.map((v) => fmtSI(v, unit)),
    });
    const axes = [
      {
        scale: 'x',
        stroke: fg,
        grid: { stroke: grid, width: 1 },
        ticks: { stroke: grid, width: 1 },
        values: (u, vs) => vs.map((v) => fmtSI(v, xu)),
      },
    ];
    if (hasV) axes.push(yAxis('V', 'V', 3, true));
    if (hasA) axes.push(yAxis('A', 'A', hasV ? 1 : 3, !hasV));
    const opts = {
      width: Math.max(200, box.current.clientWidth),
      height: Math.max(140, box.current.clientHeight - 40),
      scales: { x: { time: false } },
      cursor: { drag: { x: true, y: false } },
      axes,
      series: [
        { label: raw.vars[0].name, value: (u, v) => (v == null ? '--' : fmtSI(v, xu)) },
        ...sel.map((s) => ({
          label: s.name,
          scale: s.axis,
          stroke: s.color,
          width: 1.75,
          value: (u, v) => (v == null ? '--' : fmtSI(v, s.axis)),
        })),
      ],
      hooks: {
        setCursor: [
          (u) => {
            const i = u.cursor.idx;
            cursorCb.current(i == null ? null : u.data[0][i]);
          },
        ],
      },
    };
    const data = [raw.data[0], ...sel.map((s) => raw.data[s.index])];
    plot.current = new uPlot(opts, data, host.current);
    fit();
    return () => {
      if (plot.current) {
        plot.current.destroy();
        plot.current = null;
      }
    };
  }, [raw, key, scheme]);

  useEffect(() => {
    if (!box.current) return undefined;
    const ro = new ResizeObserver(fit);
    ro.observe(box.current);
    return () => ro.disconnect();
  }, []);

  return (
    <section class="panel waveform">
      <header>
        <h2>Waveform</h2>
        <span class="sub">{raw.plotname}</span>
        <span class="grow" />
        <span class="hint">drag = zoom, double-click = reset</span>
      </header>
      <div class="panel-body" ref={box}>
        <div class="plot" ref={host} />
        {!sel.length && <div class="msg overlay">Select signals in the list (or click a net / source in the schematic).</div>}
      </div>
    </section>
  );
}
