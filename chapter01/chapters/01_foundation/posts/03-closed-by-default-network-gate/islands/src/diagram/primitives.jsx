// Building blocks every diagram island is assembled from. A diagram
// component lays out geometry (x/y/w/h in viewBox units) and picks variants;
// it never writes CSS or colours — those live in tokens.js.
//
//   <Diagram id viewH label footer>     frame, theme, styles, arrow markers
//     <Outline .../>                     dashed region:   variant neutral|danger|safe|info, filled
//     <Fragment .../>                    UML combined fragment (alt/opt) in a sequence diagram
//     <Box .../>                         component:       variant default|solid-danger|solid-safe|solid-info|solid-dark
//     <Edge .../>                        connection:      dir none|end|both, dashed, blocked, active
//     <Badge .../>                       hotspot on an edge/box, drives the DetailPanel: shape circle|diamond|square, related
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

// The point half the total path length along a polyline — the middle of the
// route it draws, not just the midpoint of its first and last point, so a
// bent connector's mark still sits on the line.
function midpoint(points) {
  const segs = [];
  let total = 0;
  for (let i = 1; i < points.length; i++) {
    const [x0, y0] = points[i - 1];
    const [x1, y1] = points[i];
    const len = Math.hypot(x1 - x0, y1 - y0);
    segs.push(len);
    total += len;
  }
  let remaining = total / 2;
  for (let i = 0; i < segs.length; i++) {
    if (remaining <= segs[i] || i === segs.length - 1) {
      const [x0, y0] = points[i];
      const [x1, y1] = points[i + 1];
      const frac = segs[i] === 0 ? 0 : remaining / segs[i];
      return [x0 + (x1 - x0) * frac, y0 + (y1 - y0) * frac];
    }
    remaining -= segs[i];
  }
  return points[points.length - 1];
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

// UML combined fragment: a solid frame with the operator (`alt`, `opt`, …) in
// a pentagon tag at its top-left. `operands`: [{ y, guard, tone }] top to
// bottom — the first starts at the frame top and its guard sits beside the
// tag; every later one starts at a dashed separator at its `y`. `tone`
// (safe|danger) colours the guard by the branch it selects.
export function Fragment({ x, y, w, h, op = 'alt', operands = [] }) {
  const tw = GEOM.fragTagW;
  const th = GEOM.fragTagH;
  const tag = [[x, y], [x + tw, y], [x + tw, y + th - GEOM.fragTagCut], [x + tw - GEOM.fragTagCut, y + th], [x, y + th]];
  return (
    <g class="dg-frag">
      <rect class="dg-frag-frame" x={x} y={y} width={w} height={h} stroke-width={GEOM.fragStroke} />
      <polygon class="dg-frag-tag" points={toPoints(tag)} stroke-width={GEOM.fragStroke} />
      <text x={x + 8} y={y + 14} class="dg-frag-op">{op}</text>
      {operands.map((o, i) => {
        const guardCls = `dg-frag-guard${o.tone ? ` dg-frag-guard-${o.tone}` : ''}`;
        if (i === 0) {
          return (
            <text key={i} x={x + tw + 10} y={y + 14} class={guardCls}>
              {o.guard}
            </text>
          );
        }
        return (
          <g key={i}>
            <line class="dg-frag-sep" x1={x} y1={o.y} x2={x + w} y2={o.y} stroke-width={GEOM.fragStroke} stroke-dasharray={GEOM.fragDash} />
            <text x={x + 10} y={o.y + 15} class={guardCls}>
              {o.guard}
            </text>
          </g>
        );
      })}
    </g>
  );
}

// Solid variants always carry light text (.dg-box-solid), whatever the theme.
// `sub` is one line or an array of lines, stacked under the label.
export function Box({ x, y, w, h, label, sub, variant = 'default', small }) {
  const solid = variant.startsWith('solid-');
  const subs = sub ? [].concat(sub) : [];
  const step = small ? 14 : 18;
  const midY = subs.length ? y + h / 2 - 6 - ((subs.length - 1) * 14) / 2 : y + h / 2 + 5;
  return (
    <g class={`dg-box dg-box-${variant}${solid ? ' dg-box-solid' : ''}`}>
      <rect x={x} y={y} width={w} height={h} rx={GEOM.boxRadius} stroke-width={GEOM.boxStroke} />
      <text x={x + w / 2} y={midY} text-anchor="middle" class={small ? 'dg-label dg-label-sm' : 'dg-label dg-label-bold'}>
        {label}
      </text>
      {subs.map((line, i) => (
        <text key={i} x={x + w / 2} y={midY + step + i * 14} text-anchor="middle" class="dg-label dg-label-sub">
          {line}
        </text>
      ))}
    </g>
  );
}

// `dir`: undefined/none = plain line, 'end' = arrow at the last point,
// 'both' = arrows at both ends. Points must form an orthogonal polyline.
// `blocked`: a path that exists but is dropped — drawn dotted in the danger
// tone with a red ✕ at its midpoint, so it never reads as an outline's
// dashed boundary.
// `onSelect`/`onHover` make the edge itself a hotspot — for a connection with
// no interaction point of its own, still worth a click: a wide invisible
// stroke sits under the visible line so a thin or dotted path stays easy to
// hit, same hover-then-pin behaviour as a Badge.
export function Edge({ points, dashed, blocked, active, dir, id, onSelect, onHover }) {
  assertOrthogonal(points, id ? `Edge ${id}` : 'Edge');
  const { markerPrefix } = useContext(DiagramContext);
  const markerId = `${markerPrefix}-${active ? 'active' : 'default'}`;
  const arrow = dir && dir !== 'none';
  const interactive = typeof onSelect === 'function';
  const hoverIfMouse = (e, value) => {
    if (e.pointerType === 'mouse' && onHover) onHover(value);
  };
  const hitProps = interactive
    ? {
        role: 'button',
        tabindex: 0,
        'aria-label': id,
        onClick: () => onSelect(id),
        onPointerEnter: (e) => hoverIfMouse(e, id),
        onPointerLeave: (e) => hoverIfMouse(e, null),
        onKeyDown: (e) => {
          if (e.key === 'Enter' || e.key === ' ') {
            e.preventDefault();
            onSelect(id);
          }
        },
      }
    : {};
  const hit = interactive ? (
    <polyline points={toPoints(points)} fill="none" stroke="transparent" pointer-events="stroke" stroke-width={GEOM.edgeHitWidth} />
  ) : null;
  if (blocked) {
    const [mx, my] = midpoint(points);
    const r = GEOM.blockMarkR;
    const markCls = `dg-marker-blocked${active ? ' dg-marker-blocked-active' : ''}`;
    const line = (
      <>
        <polyline
          class={`dg-edge dg-edge-blocked${active ? ' dg-edge-active' : ''}`}
          points={toPoints(points)}
          fill="none"
          stroke-width={GEOM.edgeStrokeBlocked}
          stroke-dasharray={GEOM.edgeDashBlocked}
        />
        <path
          d={`M${mx - r},${my - r} L${mx + r},${my + r} M${mx + r},${my - r} L${mx - r},${my + r}`}
          class={markCls}
          fill="none"
          stroke-width={GEOM.edgeStrokeBlocked}
          stroke-linecap="round"
        />
      </>
    );
    return interactive ? (
      <g class="dg-edge-hit" {...hitProps}>
        {hit}
        {line}
      </g>
    ) : (
      line
    );
  }
  const line = (
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
  return interactive ? (
    <g class="dg-edge-hit" {...hitProps}>
      {hit}
      {line}
    </g>
  ) : (
    line
  );
}

// `active` drives the highlight (hover or selection); `pressed` is the
// selection alone, which is what aria-pressed should report. `related` is a
// softer highlight for badges linked to the active one. `label` is the short
// text inside the shape (2–3 chars), `name` the accessible name. `shape` is
// 'circle' (interaction points), 'diamond' (assets, drawn in the info tone) or
// 'square' (threat cases, drawn in the danger tone).
export function Badge({ id, label, name, x, y, active, related, pressed, shape = 'circle', onSelect, onHover }) {
  // Hover only from a real mouse: on touch, pointerenter fires on tap and
  // never leaves, which would pin the panel and break click-to-toggle.
  const hoverIfMouse = (e, value) => {
    if (e.pointerType === 'mouse' && onHover) onHover(value);
  };
  const r = active ? GEOM.badgeRadiusActive : GEOM.badgeRadius;
  const d = r + GEOM.diamondExtra;
  const cls = `dg-badge dg-badge-${shape}${active ? ' dg-badge-active' : related ? ' dg-badge-related' : ''}`;
  return (
    <g
      class={cls}
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
      {shape === 'diamond' ? (
        <polygon points={toPoints([[x, y - d], [x + d, y], [x, y + d], [x - d, y]])} stroke-width="1.5" />
      ) : shape === 'square' ? (
        <rect x={x - r} y={y - r} width={2 * r} height={2 * r} rx={GEOM.squareRadius} stroke-width="1.5" />
      ) : (
        <circle cx={x} cy={y} r={r} stroke-width="1.5" />
      )}
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

// Every diagram shares the same unit-to-pixel scale (see tokens.js) so boxes
// read the same size across posts. A diagram whose layout needs a wider
// viewBox (more room for a cramped interconnect, say) passes a larger
// `viewW`; the svg's CSS size scales up by the same factor so its content
// doesn't just render smaller inside the usual footprint, while the panel
// beside it keeps its own fixed width untouched.
const BASE_SVG_BASIS = 680;
const BASE_SVG_MIN = 380;

// `id` must be unique per mounted diagram: several diagrams share one
// document, so their SVG marker ids must not collide. `footer` renders below
// the drawing (normally a <DetailPanel/>).
// `fluid` (with `scrollHint`): the drawing fills the row and scales with the
// frame (container queries in tokens.js: panel beside it from 1240px, below it
// under that, sideways scroll under 720px) instead of keeping a fixed basis.
export function Diagram({ id, viewH, viewW = VIEW_W, label, footer, scrollHint, fluid, children }) {
  ensureStyles();
  const rootRef = useRef(null);
  const pageTheme = usePageTheme(rootRef);
  const markerPrefix = `dg-arrow-${id}`;
  const scale = viewW / VIEW_W;
  const svgStyle =
    scale !== 1 && !fluid ? { flexBasis: `${Math.round(BASE_SVG_BASIS * scale)}px`, minWidth: `${Math.round(BASE_SVG_MIN * scale)}px` } : undefined;
  // With `scrollHint`, the svg sits in its own horizontally scrolling wrapper
  // (scrolling only on narrow screens, see tokens.js) so the footer never scrolls.
  const svg = (
    <svg viewBox={`0 0 ${viewW} ${viewH}`} style={scrollHint ? undefined : svgStyle} role="group" aria-label={label}>
      <defs>
        <ArrowMarker id={`${markerPrefix}-default`} />
        <ArrowMarker id={`${markerPrefix}-active`} active />
      </defs>
      {children}
    </svg>
  );
  return (
    <div class={fluid ? 'dg-diagram dg-fluid' : 'dg-diagram'} data-dg-theme={pageTheme} ref={rootRef}>
      <DiagramContext.Provider value={{ markerPrefix }}>
        {scrollHint ? (
          <div class="dg-scrollwrap" style={svgStyle}>
            <div class="dg-scroll">{svg}</div>
            <div class="dg-scroll-hint">{scrollHint}</div>
          </div>
        ) : (
          svg
        )}
      </DiagramContext.Provider>
      {footer}
    </div>
  );
}
