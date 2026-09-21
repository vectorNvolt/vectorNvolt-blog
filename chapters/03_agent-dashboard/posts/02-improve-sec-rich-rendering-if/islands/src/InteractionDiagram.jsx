// Interaction (sequence) diagram of one poisoned turn: the operator asks for a
// README summary, the file holds an OSC 52 clipboard write, a DSR cursor query
// and a Markdown link with a hidden target. Two variants of the same lanes:
//   attack  A-1…A-10  the turn as the post-01 code runs it
//   fix     F-1…F-7   the same turn with the choke points in place
// Lifelines are dashed vertical Edges, messages horizontal Edges, self-loops
// small orthogonal loops on the right of a lifeline. Rows are laid out top to
// bottom from the MESSAGES list; badges sit on the arrow's midpoint.
//
// Geometry in viewBox units (VIEW_W = 960 wide). Segments are horizontal or
// vertical only (Edge throws on a diagonal).
import { Diagram, Box, Edge, Badge, DetailPanel, useSelection } from './diagram/primitives.jsx';
import { ATTACK_INFO, FIX_INFO, REF_SHORT } from './data.js';

const LANES = {
  operator: { x: 90, label: 'Operator', sub: 'human' },
  terminal: { x: 270, label: 'Terminal emulator', sub: 'VT parser · pty' },
  harness: { x: 450, label: 'main.py · n06', sub: 'input() ⇄ interrupt()' },
  gateway: { x: 630, label: 'gateway.py', sub: '+ TurnRenderer' },
  claude: { x: 810, label: 'claude -p', sub: 'in the sandbox' },
};
const LANE_GAP = 180;
const HEAD_W = 150;
const HEAD_H = 44;
const LIFE_TOP = 64;
const FIRST_Y = 104;
const ROW = 34;        // one horizontal message
const LOOP_ROW = 62;   // a self-loop with its label
const LOOP_W = 64;
const LOOP_H = 28;

// `self`: a loop on that lane. `dir` defaults to 'end'.
const MESSAGES = {
  attack: [
    { from: 'operator', to: 'terminal', label: 'types "summarise README.md"', badge: 'A-1' },
    { from: 'terminal', to: 'harness', label: 'stdin line → input()' },
    { from: 'harness', to: 'gateway', label: 'send_chat_turn()' },
    { from: 'gateway', to: 'claude', label: '{"type":"user", …}' },
    { self: 'claude', label: 'Read poisoned README', badge: 'A-2' },
    { from: 'claude', to: 'gateway', label: 'user · tool_result — raw bytes', badge: 'A-3' },
    { from: 'gateway', to: 'terminal', label: 'Panel(Text) — ESC survives', badge: 'A-4' },
    { self: 'terminal', label: 'OSC 52 → clipboard · DSR reply', badge: 'A-5' },
    { from: 'terminal', to: 'operator', label: 'clean-looking panel', badge: 'A-6' },
    { from: 'claude', to: 'gateway', label: 'assistant · text with [docs](…)' },
    { from: 'gateway', to: 'terminal', label: 'Markdown → OSC 8, URL hidden', badge: 'A-7' },
    { from: 'claude', to: 'gateway', label: 'result · is_error=false', badge: 'A-8' },
    { from: 'gateway', to: 'terminal', label: 'footer  4.2s · 2 turn(s)' },
    { from: 'gateway', to: 'harness', label: 'return answer' },
    { from: 'harness', to: 'terminal', label: 'interrupt → input()' },
    { from: 'terminal', to: 'harness', label: 'ESC[24;80R + typed line', badge: 'A-9' },
    { from: 'harness', to: 'claude', label: 'next user message + reply', badge: 'A-10' },
  ],
  fix: [
    { from: 'operator', to: 'terminal', label: 'types "summarise README.md"' },
    { from: 'terminal', to: 'harness', label: 'stdin line → input()' },
    { from: 'harness', to: 'gateway', label: 'send_chat_turn()' },
    { from: 'gateway', to: 'claude', label: '{"type":"user", …}' },
    { self: 'claude', label: 'Read the same README' },
    { from: 'claude', to: 'gateway', label: 'user · tool_result — raw bytes' },
    { self: 'gateway', label: 'line cap · schema · guarded loads', badge: 'F-1' },
    { self: 'gateway', label: 'sanitize() · escape(name)', badge: 'F-2' },
    { from: 'gateway', to: 'terminal', label: 'Panel shows ESC]52;… as glyphs' },
    { from: 'terminal', to: 'operator', label: 'the attempt is visible', badge: 'F-3' },
    { from: 'claude', to: 'gateway', label: 'assistant · text with [docs](…)' },
    { from: 'gateway', to: 'terminal', label: 'Markdown(hyperlinks=False)', badge: 'F-4' },
    { from: 'claude', to: 'gateway', label: 'result · is_error=false' },
    { from: 'gateway', to: 'terminal', label: 'footer — as reported', badge: 'F-5' },
    { from: 'gateway', to: 'harness', label: 'return answer' },
    { self: 'harness', label: 'tcflush(TCIFLUSH)', badge: 'F-6' },
    { from: 'harness', to: 'terminal', label: 'interrupt → input()' },
    { from: 'terminal', to: 'harness', label: 'typed line only', badge: 'F-7' },
    { from: 'harness', to: 'claude', label: 'next user message = typed text' },
  ],
};

const VARIANTS = {
  attack: {
    info: ATTACK_INFO,
    hint: 'Hover or click a numbered step to see what happens there and which threat case it is.',
    label: 'Sequence diagram of one poisoned turn as post 01 runs it: a README with escape sequences is read by a tool, rendered by Rich, executed by the terminal, and the terminal\'s reply is sent back to the model as the operator\'s next message',
  },
  fix: {
    info: FIX_INFO,
    hint: 'Hover or click a numbered step to see which choke point acts there and what it closes.',
    label: 'Sequence diagram of the same poisoned turn with the choke points in place: schema and bounds on the exec channel, sanitize() before Rich, links with visible URLs, tcflush before input()',
  },
};

function Label({ text, at, anchor = 'middle' }) {
  const [x, y] = at;
  return (
    <text x={x} y={y} text-anchor={anchor} class="dg-label dg-label-sub">
      {text}
    </text>
  );
}

// Lay the rows out: every message gets a y, self-loops take a taller row.
function layout(messages) {
  let y = FIRST_Y;
  const rows = messages.map((m) => {
    const row = { ...m, y: m.self ? y + 16 : y };
    y += m.self ? LOOP_ROW : ROW;
    return row;
  });
  return { rows, bottom: y };
}

export default function InteractionDiagram({ id = 'flow', variant = 'attack' }) {
  const v = VARIANTS[variant];
  const { rows, bottom } = layout(MESSAGES[variant]);
  const lifeBottom = bottom + 6;
  const viewH = lifeBottom + 20;

  const { shownId, pinnedId, select, setHover } = useSelection();
  const info = shownId ? v.info[shownId] : null;
  const chips = info
    ? [
        ...(info.tc || []).map((c) => ({ tone: 'danger', id: c, text: REF_SHORT[c] })),
        ...(info.fix || []).map((c) => ({ tone: 'safe', id: c, text: REF_SHORT[c] })),
        ...(info.code || []).map((c) => ({ tone: 'info', id: c, text: REF_SHORT[c] })),
      ]
    : [];

  const badges = [];

  return (
    <Diagram
      id={id}
      viewH={viewH}
      label={v.label}
      footer={<DetailPanel id={shownId} title={info?.name} desc={info?.desc} chips={chips} hint={v.hint} />}
    >
      {Object.entries(LANES).map(([key, l]) => (
        <g key={key}>
          <Box x={l.x - HEAD_W / 2} y={20} w={HEAD_W} h={HEAD_H} label={l.label} sub={l.sub} small variant={key === 'claude' ? 'solid-danger' : 'default'} />
          <Edge id={`life-${key}`} points={[[l.x, LIFE_TOP], [l.x, lifeBottom]]} dashed />
        </g>
      ))}

      {rows.map((m, i) => {
        const active = m.badge && shownId === m.badge;
        if (m.self) {
          const x = LANES[m.self].x;
          const pts = [[x, m.y], [x + LOOP_W, m.y], [x + LOOP_W, m.y + LOOP_H], [x, m.y + LOOP_H]];
          if (m.badge) badges.push({ id: m.badge, x: x + LOOP_W + 24, y: m.y + LOOP_H / 2 });
          return (
            <g key={i}>
              <Edge id={`loop-${i}`} points={pts} dir="end" active={active} />
              <Label text={m.label} at={[x + 8, m.y - 8]} anchor="start" />
            </g>
          );
        }
        const x0 = LANES[m.from].x;
        const x1 = LANES[m.to].x;
        // Badge + label go to the arrow's midpoint unless that is another
        // lane's lifeline, then one half-gap towards the sender.
        let mid = (x0 + x1) / 2;
        if (Object.values(LANES).some((l) => l.x === mid)) mid -= Math.sign(x1 - x0) * (LANE_GAP / 2);
        if (m.badge) badges.push({ id: m.badge, x: mid, y: m.y });
        return (
          <g key={i}>
            <Edge id={`msg-${i}`} points={[[x0, m.y], [x1, m.y]]} dir={m.dir || 'end'} active={active} />
            <Label text={m.label} at={[mid, m.y - (m.badge ? 18 : 6)]} />
          </g>
        );
      })}

      {badges.map((b) => (
        <Badge
          key={b.id}
          id={b.id}
          label={b.id.replace('-', '')}
          name={v.info[b.id].name}
          x={b.x}
          y={b.y}
          active={shownId === b.id}
          pressed={pinnedId === b.id}
          onSelect={select}
          onHover={setHover}
        />
      ))}
    </Diagram>
  );
}
