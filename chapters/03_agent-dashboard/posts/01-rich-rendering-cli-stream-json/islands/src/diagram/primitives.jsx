// Building blocks every diagram island is assembled from. A diagram
// component lays out geometry (x/y/w/h in viewBox units) and picks variants;
// it never writes CSS or colours — those live in tokens.js.
//
//   <Diagram id viewH label footer>     frame, theme, styles, arrow markers
//     <Outline .../>                     dashed region:   variant neutral|danger|safe|info, filled
//     <Box .../>                         component:       variant default|solid-danger|solid-safe|solid-info|solid-dark
//     <Edge .../>                        connection:      dir none|end|both, dashed, active
//     <Badge .../>                       hotspot on an edge/box, drives the DetailPanel
//   </Diagram>
//   <DetailPanel .../> + <Chip .../>     text block under the drawing
//   useSelection()                       hover/click state shared by Badge + DetailPanel

import { createContext } from 'preact';
import { useContext, useRef, useState } from 'preact/hooks';
import { usePageTheme } from './theme.js';
import { ensureStyles, GEOM, VIEW_W } from './tokens.js';

const DiagramContext = createContext({ markerPrefix: 'dg-arrow' });

export function toPoints(pairs) {
  return pairs.map(([x, y]) => `${x},${y}`).join(' ');
}

// Connectors are orthogonal only: every segment is horizontal or vertical.
// A diagonal is a layout mistake, not a style choice, so it fails loudly at
// render time (the island shows nothing and the console names the edge)
// rather than shipping. Route around obstacles with intermediate points.
export function assertOrthogonal(points, what = 'Edge') {
  for (let i = 1; i < points.length; i++) {
    const [x0, y0] = points[i - 1];
    const [x1, y1] = points[i];
    if (x0 !== x1 && y0 !== y1) {
      throw new Error(`${what}: diagonal segment (${x0},${y0})→(${x1},${y1}); connectors must be horizontal/vertical — add a bend point`);
    }
  }
  return points;
}

export function Outline({ x, y, w, h, label, variant = 'neutral', filled }) {
  return (
    <g class={`dg-outline dg-outline-${variant}${filled ? ' dg-outline-filled' : ''}`}>
      <rect x={x} y={y} width={w} height={h} rx={GEOM.outlineRadius} stroke-width={GEOM.outlineStroke} stroke-dasharray={GEOM.outlineDash} />
      {label ? <text x={x + 14} y={y + 22} class="dg-label dg-label-outline">{label}</text> : null}
    </g>
  );
}

// Solid variants always carry light text (.dg-box-solid), whatever the theme.
export function Box({ x, y, w, h, label, sub, variant = 'default', small }) {
  const solid = variant.startsWith('solid-');
  const midY = sub ? y + h / 2 - 6 : y + h / 2 + 5;
  return (
    <g class={`dg-box dg-box-${variant}${solid ? ' dg-box-solid' : ''}`}>
      <rect x={x} y={y} width={w} height={h} rx={GEOM.boxRadius} stroke-width={GEOM.boxStroke} />
      <text x={x + w / 2} y={midY} text-anchor="middle" class={small ? 'dg-label dg-label-sm' : 'dg-label dg-label-bold'}>
        {label}
      </text>
      {sub ? (
        <text x={x + w / 2} y={midY + (small ? 14 : 18)} text-anchor="middle" class="dg-label dg-label-sub">
          {sub}
        </text>
      ) : null}
    </g>
  );
}

// `dir`: undefined/none = plain line, 'end' = arrow at the last point,
// 'both' = arrows at both ends. Points must form an orthogonal polyline.
export function Edge({ points, dashed, active, dir, id }) {
  assertOrthogonal(points, id ? `Edge ${id}` : 'Edge');
  const { markerPrefix } = useContext(DiagramContext);
  const markerId = `${markerPrefix}-${active ? 'active' : 'default'}`;
  const arrow = dir && dir !== 'none';
  return (
    <polyline
      class={`dg-edge${active ? ' dg-edge-active' : ''}`}
      points={toPoints(points)}
      fill="none"
      stroke-width={active ? GEOM.edgeStrokeActive : GEOM.edgeStroke}
      stroke-dasharray={dashed ? GEOM.edgeDash : undefined}
      marker-end={arrow ? `url(#${markerId})` : undefined}
      marker-start={dir === 'both' ? `url(#${markerId})` : undefined}
    />
  );
}

// `active` drives the highlight (hover or selection); `pressed` is the
// selection alone, which is what aria-pressed should report. `label` is the
// short text inside the circle (2–3 chars), `name` the accessible name.
export function Badge({ id, label, name, x, y, active, pressed, onSelect, onHover }) {
  // Hover only from a real mouse: on touch, pointerenter fires on tap and
  // never leaves, which would pin the panel and break click-to-toggle.
  const hoverIfMouse = (e, value) => {
    if (e.pointerType === 'mouse' && onHover) onHover(value);
  };
  return (
    <g
      class={`dg-badge${active ? ' dg-badge-active' : ''}`}
      role="button"
      tabindex="0"
      aria-pressed={pressed}
      aria-label={name ? `${id}: ${name}` : id}
      onClick={() => onSelect(id)}
      onPointerEnter={(e) => hoverIfMouse(e, id)}
      onPointerLeave={(e) => hoverIfMouse(e, null)}
      onKeyDown={(e) => {
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault();
          onSelect(id);
        }
      }}
    >
      <circle cx={x} cy={y} r={active ? GEOM.badgeRadiusActive : GEOM.badgeRadius} stroke-width="1.5" />
      <text x={x} y={y + 4} text-anchor="middle" class="dg-badge-label">
        {label ?? id}
      </text>
    </g>
  );
}

// Hover shows, click pins (click again to unpin). Returns the id to show
// (hover wins over the pinned one) and the pinned id for aria-pressed.
export function useSelection() {
  const [pinned, setPinned] = useState(null);
  const [hover, setHover] = useState(null);
  const select = (id) => setPinned((cur) => (cur === id ? null : id));
  return { shownId: hover || pinned, pinnedId: pinned, select, setHover };
}

export function Chip({ tone = 'neutral', id, text }) {
  return (
    <span class={`dg-chip dg-chip-${tone}`}>
      <span class="dg-chip-id">{id}</span>
      {text ? <span class="dg-chip-desc">{text}</span> : null}
    </span>
  );
}

// `chips`: [{ tone, id, text }]. With no `title` the panel shows `hint`.
export function DetailPanel({ id, title, desc, chips, hint, children }) {
  return (
    <div class="dg-panel" aria-live="polite">
      {title ? (
        <>
          <div class="dg-panel-title">
            {id ? <>{id} — </> : null}
            {title}
          </div>
          {desc ? <div class="dg-panel-desc">{desc}</div> : null}
          {chips && chips.length ? (
            <div class="dg-chips">
              {chips.map((c) => (
                <Chip key={`${c.tone}-${c.id}`} {...c} />
              ))}
            </div>
          ) : null}
          {children}
        </>
      ) : (
        <div class="dg-hint">{hint}</div>
      )}
    </div>
  );
}

function ArrowMarker({ id, active }) {
  return (
    <marker id={id} viewBox="0 0 10 10" refX="8" refY="5" markerWidth="6" markerHeight="6" orient="auto-start-reverse">
      <path d="M0,0 L10,5 L0,10 z" class={`dg-marker${active ? ' dg-marker-active' : ''}`} />
    </marker>
  );
}

// `id` must be unique per mounted diagram: several diagrams share one
// document, so their SVG marker ids must not collide. `footer` renders below
// the drawing (normally a <DetailPanel/>).
export function Diagram({ id, viewH, viewW = VIEW_W, label, footer, children }) {
  ensureStyles();
  const rootRef = useRef(null);
  const pageTheme = usePageTheme(rootRef);
  const markerPrefix = `dg-arrow-${id}`;
  return (
    <div class="dg-diagram" data-dg-theme={pageTheme} ref={rootRef}>
      <DiagramContext.Provider value={{ markerPrefix }}>
        <svg viewBox={`0 0 ${viewW} ${viewH}`} role="group" aria-label={label}>
          <defs>
            <ArrowMarker id={`${markerPrefix}-default`} />
            <ArrowMarker id={`${markerPrefix}-active`} active />
          </defs>
          {children}
        </svg>
      </DiagramContext.Provider>
      {footer}
    </div>
  );
}
