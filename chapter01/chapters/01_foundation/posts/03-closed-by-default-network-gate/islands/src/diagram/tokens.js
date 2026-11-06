// Diagram styling standard: colour tokens, geometry constants and the one
// stylesheet every diagram island shares. Diagrams never hardcode colours or
// sizes — they pick a `variant` / `tone` and the classes below resolve it, so
// every diagram on the blog reads the same and follows the page theme.
//
// Class prefix is `dg-`. It is fixed here and in primitives.jsx; a diagram
// component only ever uses the exported constants and the primitives.

// Every diagram uses the same viewBox width so boxes, labels and badges are
// the same physical size on the page; only the height is per diagram.
export const VIEW_W = 960;

export const GEOM = {
  boxRadius: 8,
  boxStroke: 1.5,
  outlineRadius: 10,
  outlineStroke: 2,
  outlineDash: '6 4',
  edgeStroke: 1.5,
  edgeStrokeActive: 3,
  edgeDash: '5 4',
  // Blocked path: round dots, heavier than an edge, unlike an outline's dashes.
  edgeStrokeBlocked: 2.75,
  edgeDashBlocked: '0.1 6',
  // Half-width of the ✕ drawn at the midpoint of a blocked path.
  blockMarkR: 6,
  // Invisible wide stroke laid under a clickable edge so a thin line/dotted
  // path is still easy to hit.
  edgeHitWidth: 14,
  badgeRadius: 13,
  badgeRadiusActive: 15,
  // A diamond's half-diagonal is the badge radius plus this, so its label
  // fits between the slanted sides.
  diamondExtra: 3,
  // Corner radius of the threat-case square (half-side = badge radius).
  squareRadius: 3,
  // Minimum edge length that still shows line + arrowheads around a badge;
  // shorter edges get their badge placed beside the line instead.
  badgeOnEdgeMin: 60,
  // Combined fragment (UML alt/opt frame) in a sequence diagram: frame stroke,
  // operand-separator dash, and the operator tag's pentagon size.
  fragStroke: 1.25,
  fragDash: '4 3',
  fragTagW: 34,
  fragTagH: 20,
  fragTagCut: 6,
};

// Light is the default; dark applies via prefers-color-scheme unless the
// runtime detector (theme.js) has pinned data-dg-theme explicitly, which is
// what keeps the island in step with the page's own toggle.
const LIGHT_TOKENS = `
  --dg-bg: #f6f8fa;
  --dg-border: #d0d7de;
  --dg-box: #ffffff;
  --dg-box-stroke: #8c959f;
  --dg-outline: #8c959f;
  --dg-text: #1f2328;
  --dg-text-sub: #57606a;
  --dg-on-solid: #e8edf2;
  --dg-on-solid-sub: #d5dbe1;
  --dg-edge: #6e7781;
  --dg-blocked: #e8590c;
  --dg-accent: #ffd43b;
  --dg-accent-stroke: #b8860b;
  --dg-on-accent: #1b2631;
  --dg-badge: #e6ebf1;
  --dg-badge-stroke: #6e7781;
  --dg-badge-text: #1f2328;
  --dg-panel: #ffffff;
  --dg-panel-title: #8a6500;
  --dg-panel-text: #1f2328;
  --dg-hint: #6e7781;

  --dg-danger: #c0392b;
  --dg-danger-deep: #7b241c;
  --dg-danger-tint: rgba(192, 57, 43, 0.08);
  --dg-safe: #27ae60;
  --dg-safe-deep: #145a32;
  --dg-safe-tint: rgba(39, 174, 96, 0.08);
  --dg-info: #2e86c1;
  --dg-info-deep: #1b4f72;
  --dg-info-tint: rgba(46, 134, 193, 0.08);
  --dg-solid-dark: #34495e;
  --dg-solid-dark-stroke: #1b2631;

  --dg-chip-info: #0b4f9c;
  --dg-chip-info-border: #7fb3ff;
  --dg-chip-info-bg: rgba(127, 179, 255, 0.15);
  --dg-chip-danger: #9c2a20;
  --dg-chip-danger-border: #ff8a80;
  --dg-chip-danger-bg: rgba(255, 138, 128, 0.15);
  --dg-chip-safe: #1e6b3a;
  --dg-chip-safe-border: #7ddba0;
  --dg-chip-safe-bg: rgba(125, 219, 160, 0.15);
  --dg-chip-neutral: #57606a;
  --dg-chip-neutral-border: #b6bec8;
  --dg-chip-neutral-bg: rgba(182, 190, 200, 0.15);
`;

const DARK_TOKENS = `
  --dg-bg: #10151c;
  --dg-border: #2c3644;
  --dg-box: #1e2530;
  --dg-box-stroke: #4a5568;
  --dg-outline: #5d6d7e;
  --dg-text: #e8edf2;
  --dg-text-sub: #a9b4c0;
  --dg-on-solid: #e8edf2;
  --dg-on-solid-sub: #d5dbe1;
  --dg-edge: #7f8c9a;
  --dg-blocked: #ff922b;
  --dg-accent: #f4d03f;
  --dg-accent-stroke: #b7950b;
  --dg-on-accent: #1b2631;
  --dg-badge: #2c3644;
  --dg-badge-stroke: #7f8c9a;
  --dg-badge-text: #e8edf2;
  --dg-panel: #161d27;
  --dg-panel-title: #f4d03f;
  --dg-panel-text: #cfd8e3;
  --dg-hint: #8b97a5;

  --dg-danger: #c0392b;
  --dg-danger-deep: #7b241c;
  --dg-danger-tint: rgba(192, 57, 43, 0.10);
  --dg-safe: #27ae60;
  --dg-safe-deep: #145a32;
  --dg-safe-tint: rgba(39, 174, 96, 0.10);
  --dg-info: #2e86c1;
  --dg-info-deep: #1b4f72;
  --dg-info-tint: rgba(46, 134, 193, 0.10);
  --dg-solid-dark: #1b2631;
  --dg-solid-dark-stroke: #5d6d7e;

  --dg-chip-info: #7fb3ff;
  --dg-chip-info-border: #2f5d8a;
  --dg-chip-info-bg: rgba(127, 179, 255, 0.08);
  --dg-chip-danger: #ff8a80;
  --dg-chip-danger-border: #8a3a33;
  --dg-chip-danger-bg: rgba(255, 138, 128, 0.08);
  --dg-chip-safe: #7ddba0;
  --dg-chip-safe-border: #2f6b45;
  --dg-chip-safe-bg: rgba(125, 219, 160, 0.08);
  --dg-chip-neutral: #a9b4c0;
  --dg-chip-neutral-border: #4a5568;
  --dg-chip-neutral-bg: rgba(169, 180, 192, 0.08);
`;

export const STYLES = `
  .dg-diagram { ${LIGHT_TOKENS} }
  @media (prefers-color-scheme: dark) {
    .dg-diagram:not([data-dg-theme="light"]) { ${DARK_TOKENS} }
  }
  .dg-diagram[data-dg-theme="dark"] { ${DARK_TOKENS} }

  /* Own the box model inside the component regardless of what the host page's
     stylesheet does with box-sizing elsewhere — the flex math below assumes
     border-box on every child. */
  .dg-diagram, .dg-diagram *, .dg-diagram *::before, .dg-diagram *::after {
    box-sizing: border-box;
  }

  /* frame: drawing and panel sit side by side, wrapping to a stack when the
     row can't fit both at a readable size */
  .dg-diagram {
    background: var(--dg-bg);
    border: 1px solid var(--dg-border);
    border-radius: 12px;
    padding: 12px;
    margin: 1.5em auto;
    max-width: 100%;
    display: flex;
    flex-wrap: wrap;
    align-items: flex-start;
    gap: 16px;
    font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Helvetica, Arial, sans-serif;
  }
  /* svg grows ~2px for every 1px the panel grows, so extra page width goes
     mostly to the drawing instead of stretching the text column past a
     readable line length. */
  .dg-diagram svg { flex: 2 1 680px; min-width: 380px; height: auto; display: block; }

  /* typography */
  .dg-label { fill: var(--dg-text); font-size: 13px; }
  .dg-label-bold { font-weight: 600; }
  .dg-label-sm { font-size: 11px; }
  .dg-label-sub { fill: var(--dg-text-sub); font-size: 11px; }
  .dg-label-outline { fill: var(--dg-text-sub); font-size: 11px; letter-spacing: 0.02em; text-transform: uppercase; }

  /* outlines: dashed regions grouping boxes */
  .dg-outline rect { fill: none; stroke: var(--dg-outline); }
  .dg-outline-danger rect { stroke: var(--dg-danger); }
  .dg-outline-safe rect { stroke: var(--dg-safe); }
  .dg-outline-info rect { stroke: var(--dg-info); }
  .dg-outline-filled.dg-outline-neutral rect { fill: var(--dg-box); }
  .dg-outline-filled.dg-outline-danger rect { fill: var(--dg-danger-tint); }
  .dg-outline-filled.dg-outline-safe rect { fill: var(--dg-safe-tint); }
  .dg-outline-filled.dg-outline-info rect { fill: var(--dg-info-tint); }

  /* combined fragments: solid frame, operator tag, dashed operand separators,
     guards in the tone of the branch they select */
  .dg-frag-frame { fill: none; stroke: var(--dg-box-stroke); }
  .dg-frag-tag { fill: var(--dg-box); stroke: var(--dg-box-stroke); }
  .dg-frag-sep { stroke: var(--dg-box-stroke); }
  .dg-frag-op { fill: var(--dg-text); font-size: 11px; font-weight: 700; }
  .dg-frag-guard { fill: var(--dg-text-sub); font-size: 11px; font-weight: 600; }
  .dg-frag-guard-safe { fill: var(--dg-chip-safe); }
  .dg-frag-guard-danger { fill: var(--dg-chip-danger); }

  /* boxes: components */
  .dg-box rect { fill: var(--dg-box); stroke: var(--dg-box-stroke); }
  .dg-box-solid-danger rect { fill: var(--dg-danger); stroke: var(--dg-danger-deep); }
  .dg-box-solid-safe rect { fill: var(--dg-safe-deep); stroke: var(--dg-safe); }
  .dg-box-solid-info rect { fill: var(--dg-info-deep); stroke: var(--dg-info); }
  .dg-box-solid-dark rect { fill: var(--dg-solid-dark); stroke: var(--dg-solid-dark-stroke); }
  .dg-box-solid .dg-label { fill: var(--dg-on-solid); }
  .dg-box-solid .dg-label-sub { fill: var(--dg-on-solid-sub); }

  /* edges */
  .dg-edge { stroke: var(--dg-edge); }
  .dg-edge-active { stroke: var(--dg-accent-stroke); }
  .dg-marker { fill: var(--dg-edge); }
  .dg-marker-active { fill: var(--dg-accent-stroke); }
  .dg-edge-blocked { stroke: var(--dg-blocked); stroke-linecap: round; }
  .dg-marker-blocked { stroke: var(--dg-blocked); }
  .dg-edge-blocked.dg-edge-active { stroke: var(--dg-accent-stroke); }
  .dg-marker-blocked-active { stroke: var(--dg-accent-stroke); }

  /* activity diagram (ActivityDiagram.jsx): node label, the file names inside
     a node (monospace; colour from dg-label-sub), the "changed" outline; the
     description of a step is in the shared detail panel */
  .dg-flow-label { font-size: 11.5px; font-weight: 600; }
  .dg-flow-node { cursor: pointer; }
  .dg-flow-node:focus { outline: none; }
  .dg-flow-changed rect { stroke: var(--dg-safe); stroke-width: 3; }
  .dg-flow-node-active rect, .dg-flow-node:focus rect { stroke: var(--dg-accent-stroke); stroke-width: 3; }
  .dg-flow-file { font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; font-size: 11px; }
  .dg-flow-edge-label { fill: var(--dg-text-sub); font-size: 11px; font-style: italic; }
  /* narrow screens: keep the drawing at a tappable size (boxes about 44px
     high) and let the frame scroll sideways; the panel stays under it */
  .dg-scroll-hint { display: none; margin-top: 4px; font-size: 12px; font-style: italic; color: var(--dg-hint); text-align: center; }
  @media (max-width: 720px) {
    .dg-scrollwrap { flex: 1 1 100% !important; min-width: 0 !important; max-width: 100%; }
    .dg-scroll { overflow-x: auto; }
    .dg-scroll svg { min-width: 800px; }
    .dg-scroll-hint { display: block; }
    .dg-diagram:has(.dg-flow-node) .dg-panel { position: static; flex-basis: 100%; min-width: 0; max-width: none; }
  }

  /* fluid drawing (Diagram \`fluid\`, the flow island): the frame is the size
     container, so the breakpoints follow the post column, not the viewport.
     Default (and any browser without container queries) = drawing full width
     with the panel below; it scales with the frame through its viewBox.
     720px..1239px: drawing and panel both 100% wide (panel below), the svg
     scales up as well as down (no width cap).
     >= 1240px: panel beside the drawing, fixed 300px; the drawing gets the
     rest of the row (>= ~900px).
     < 720px: drawing keeps 800px and scrolls sideways, swipe hint shown. */
  .dg-fluid { container-type: inline-size; }
  .dg-diagram.dg-fluid .dg-scrollwrap { flex: 1 1 100% !important; width: 100% !important; min-width: 0 !important; max-width: none !important; }
  .dg-diagram.dg-fluid .dg-scroll svg { width: 100%; min-width: 0; max-width: none; }
  .dg-diagram.dg-fluid .dg-panel { flex: 0 0 100% !important; width: 100% !important; min-width: 0 !important; max-width: none !important; position: static; max-height: none; }
  @container (min-width: 1240px) {
    .dg-diagram.dg-fluid .dg-scrollwrap { flex: 1 1 0 !important; width: auto !important; }
    .dg-diagram.dg-fluid .dg-panel { flex: 0 0 300px !important; width: 300px !important; min-width: 300px !important; max-width: 300px !important; position: sticky; max-height: calc(100vh - 32px); }
  }
  @container (max-width: 719px) {
    .dg-diagram.dg-fluid .dg-scroll { overflow-x: auto; }
    .dg-diagram.dg-fluid .dg-scroll svg { min-width: 800px; }
    .dg-diagram.dg-fluid .dg-scroll-hint { display: block; }
  }

  /* clickable edge with no badge of its own: a wide transparent stroke gives
     it a comfortable hit area without widening the visible line. */
  .dg-edge-hit { cursor: pointer; }
  .dg-edge-hit:focus { outline: none; }
  .dg-edge-hit:focus .dg-edge { stroke: var(--dg-accent-stroke); }
  .dg-edge-hit:focus .dg-edge-blocked { stroke: var(--dg-accent-stroke); }

  /* badges: the interactive hotspots */
  .dg-badge { cursor: pointer; }
  .dg-badge circle, .dg-badge polygon, .dg-badge rect { fill: var(--dg-badge); stroke: var(--dg-badge-stroke); }
  .dg-badge-diamond polygon { fill: var(--dg-box); stroke: var(--dg-info); }
  .dg-badge-square rect { fill: var(--dg-box); stroke: var(--dg-danger); }
  .dg-badge-related circle, .dg-badge-related polygon, .dg-badge-related rect { stroke: var(--dg-accent-stroke); stroke-width: 2.5; }
  .dg-badge-active circle, .dg-badge-active polygon, .dg-badge-active rect { fill: var(--dg-accent); stroke: var(--dg-accent-stroke); }
  .dg-badge:focus { outline: none; }
  .dg-badge:focus circle, .dg-badge:focus polygon, .dg-badge:focus rect { stroke: var(--dg-accent-stroke); stroke-width: 2.5; }
  .dg-badge-label { fill: var(--dg-badge-text); font-size: 11px; font-weight: 700; pointer-events: none; }
  .dg-badge-diamond .dg-badge-label { fill: var(--dg-chip-info); font-size: 10px; }
  .dg-badge-diamond.dg-badge-active .dg-badge-label { fill: var(--dg-on-accent); }
  .dg-badge-square .dg-badge-label { fill: var(--dg-chip-danger); font-size: 10px; }
  .dg-badge-square.dg-badge-active .dg-badge-label { fill: var(--dg-on-accent); }
  .dg-badge-active .dg-badge-label { fill: var(--dg-on-accent); }

  /* detail panel: beside the drawing when there's room, full-width below it
     once the row wraps. Sticky, so on a drawing taller than the viewport the
     panel follows the scroll until the diagram's bottom edge takes it away;
     capped at the viewport height and scrolls itself if its text runs longer.
     Once wrapped below the drawing the sticky has no effect. */
  .dg-diagram .dg-panel {
    box-sizing: border-box;
    flex: 1 1 320px;
    min-width: 280px;
    max-width: 440px;
    position: sticky;
    top: 16px;
    max-height: calc(100vh - 32px);
    overflow-y: auto;
    padding: 12px 14px;
    background: var(--dg-panel);
    border: 1px solid var(--dg-border);
    border-radius: 8px;
    min-height: 64px;
  }
  .dg-diagram .dg-panel-title { color: var(--dg-panel-title); font-weight: 700; font-size: 14px; margin-bottom: 4px; }
  .dg-diagram .dg-panel-desc { color: var(--dg-panel-text); font-size: 13.5px; line-height: 1.5; margin: 0 0 8px; white-space: pre-line; }
  .dg-diagram .dg-hint { color: var(--dg-hint); font-size: 13px; font-style: italic; margin: 0; }
  .dg-diagram .dg-chips { display: flex; flex-wrap: wrap; gap: 8px; }
  .dg-diagram .dg-chip { display: flex; align-items: baseline; gap: 5px; font-size: 11px; padding: 3px 10px; border-radius: 999px; border: 1px solid; }
  .dg-chip-id { font-weight: 700; }
  .dg-chip-desc { font-weight: 500; opacity: 0.85; }
  .dg-chip-info { color: var(--dg-chip-info); border-color: var(--dg-chip-info-border); background: var(--dg-chip-info-bg); }
  .dg-chip-danger { color: var(--dg-chip-danger); border-color: var(--dg-chip-danger-border); background: var(--dg-chip-danger-bg); }
  .dg-chip-safe { color: var(--dg-chip-safe); border-color: var(--dg-chip-safe-border); background: var(--dg-chip-safe-bg); }
  .dg-chip-neutral { color: var(--dg-chip-neutral); border-color: var(--dg-chip-neutral-border); background: var(--dg-chip-neutral-bg); }
`;

// Inject the shared stylesheet once per page, however many diagrams mount
// (and however many bundles: the marker attribute survives across islands).
const STYLE_ID = 'dg-diagram-styles';
export function ensureStyles() {
  if (typeof document === 'undefined' || document.getElementById(STYLE_ID)) return;
  const el = document.createElement('style');
  el.id = STYLE_ID;
  el.textContent = STYLES;
  document.head.appendChild(el);
}
