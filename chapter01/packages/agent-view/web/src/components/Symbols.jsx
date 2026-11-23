// SVG symbols, drawn in the element's local box (0,0)-(w,h); terminals sit on the box border.
// Colors come from CSS (stroke: currentColor on the parent group).

function zigzag(w, h) {
  const cy = h / 2;
  const a = w * 0.2;
  const b = w * 0.8;
  const n = 6;
  const step = (b - a) / n;
  let d = `M0 ${cy} L${a} ${cy}`;
  for (let i = 0; i < n; i++) d += ` L${a + step * (i + 0.5)} ${cy + (i % 2 ? 8 : -8)}`;
  return d + ` L${b} ${cy} L${w} ${cy}`;
}

function loops(w, h) {
  const cy = h / 2;
  const a = w * 0.15;
  const n = 4;
  const r = (w * 0.7) / (2 * n);
  let d = `M0 ${cy} L${a} ${cy}`;
  for (let i = 1; i <= n; i++) d += ` A${r} ${r} 0 0 1 ${a + 2 * r * i} ${cy}`;
  return d + ` L${w} ${cy}`;
}

export function Sym({ kind, w, h }) {
  const cx = w / 2;
  const cy = h / 2;
  switch (kind) {
    case 'R':
      return <path d={zigzag(w, h)} fill="none" />;
    case 'L':
      return <path d={loops(w, h)} fill="none" />;
    case 'C':
      return (
        <g fill="none">
          <path d={`M${cx} 0 V${cy - 5} M${cx} ${cy + 5} V${h}`} />
          <path d={`M${cx - 13} ${cy - 5} H${cx + 13} M${cx - 13} ${cy + 5} H${cx + 13}`} />
        </g>
      );
    case 'D':
      return (
        <g fill="none">
          <path d={`M0 ${cy} H${cx - 8} M${cx + 8} ${cy} H${w}`} />
          <path d={`M${cx - 8} ${cy - 9} L${cx + 8} ${cy} L${cx - 8} ${cy + 9} Z M${cx + 8} ${cy - 9} V${cy + 9}`} />
        </g>
      );
    case 'V':
      return (
        <g fill="none">
          <path d={`M${cx} 0 V${cy - 15} M${cx} ${cy + 15} V${h}`} />
          <circle cx={cx} cy={cy} r={15} />
          <path d={`M${cx - 4} ${cy - 6} H${cx + 4} M${cx} ${cy - 10} V${cy - 2} M${cx - 4} ${cy + 7} H${cx + 4}`} />
        </g>
      );
    case 'I':
      return (
        <g fill="none">
          <path d={`M${cx} 0 V${cy - 15} M${cx} ${cy + 15} V${h}`} />
          <circle cx={cx} cy={cy} r={15} />
          <path d={`M${cx} ${cy - 8} V${cy + 8} M${cx - 4} ${cy + 3} L${cx} ${cy + 8} L${cx + 4} ${cy + 3}`} />
        </g>
      );
    case 'GND':
      return (
        <g fill="none">
          <path d={`M${cx} 0 V5 M2 5 H${w - 2} M6 9.5 H${w - 6} M9 14 H${w - 9}`} />
        </g>
      );
    default:
      return <rect x={0} y={0} width={w} height={h} rx={3} fill="none" />;
  }
}
