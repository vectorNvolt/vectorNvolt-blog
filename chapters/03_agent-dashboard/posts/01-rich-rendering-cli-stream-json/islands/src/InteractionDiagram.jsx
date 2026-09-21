// Interaction (sequence) diagram of one exchange turn that includes a tool
// call: operator → node 6 → gateway → exec session → claude (in the sandbox)
// → Anthropic API and back, with the renderer's output on the left.
// Lifelines are dashed vertical Edges, messages are horizontal Edges, badges
// S-1…S-9 mark the steps the panel explains.
//
// Geometry is hand laid out in viewBox units (VIEW_W = 960 wide). Segments are
// horizontal or vertical only (Edge throws on a diagonal).
import { Diagram, Box, Edge, Badge, DetailPanel, useSelection } from './diagram/primitives.jsx';
import { STEP_INFO, REF_SHORT } from './data.js';

const VIEW_H = 780;
const LIFE_TOP = 64;
const LIFE_BOTTOM = 760;

// Lifeline x positions; header boxes are centred on them.
const LANES = {
  operator: { x: 80, label: 'Operator', sub: 'terminal' },
  n06: { x: 240, label: 'n06_exchange', sub: 'graph node' },
  gateway: { x: 400, label: 'gateway.py', sub: '+ TurnRenderer' },
  session: { x: 560, label: 'ExecSession', sub: 'tty=False' },
  claude: { x: 720, label: 'claude -p', sub: 'in the sandbox' },
  api: { x: 880, label: 'Anthropic API', sub: 'prompt cache' },
};
const HEAD_W = 140;
const HEAD_H = 44;

// Messages, top to bottom. `badge` = step id shown on the arrow's midpoint.
const MESSAGES = [
  { from: 'operator', to: 'n06', y: 104, label: 'reply text → Command(resume=…)', badge: 'S-1' },
  { from: 'n06', to: 'gateway', y: 138, label: 'send_chat_turn(c, last user msg)' },
  { from: 'gateway', to: 'claude', y: 172, label: 'write({"type":"user", …} + "\\n")', badge: 'S-2' },
  { from: 'claude', to: 'api', y: 206, label: 'POST: system + tools + full history', badge: 'S-3' },
  { from: 'api', to: 'claude', y: 240, label: 'SSE stream' },
  { from: 'claude', to: 'gateway', y: 274, label: 'stream_event · content_block_delta ×N', badge: 'S-4' },
  { from: 'gateway', to: 'operator', y: 308, label: 'Live tail → Markdown' },
  { from: 'claude', to: 'gateway', y: 342, label: 'assistant · tool_use', badge: 'S-5' },
  { from: 'gateway', to: 'operator', y: 376, label: 'Panel  tool · Bash' },
  { from: 'claude', to: 'gateway', y: 464, label: 'user · tool_result', badge: 'S-7' },
  { from: 'gateway', to: 'operator', y: 498, label: 'Panel  tool result' },
  { from: 'claude', to: 'api', y: 532, label: '2nd call: history + tool_result — prefix cached', dir: 'both' },
  { from: 'claude', to: 'gateway', y: 566, label: 'stream_event ×N · assistant · text' },
  { from: 'gateway', to: 'operator', y: 600, label: 'Markdown' },
  { from: 'claude', to: 'gateway', y: 634, label: 'result · usage · duration_ms', badge: 'S-8' },
  { from: 'gateway', to: 'operator', y: 668, label: 'footer  7.1s · 2 turn(s)' },
  { from: 'gateway', to: 'n06', y: 702, label: 'return answer → transcript' },
  { from: 'n06', to: 'operator', y: 736, label: 'interrupt("Your reply (or /quit):")', badge: 'S-9' },
];

// The tool call itself: a self-loop on the claude lifeline (S-6).
const SELF_LOOP = { points: [[720, 400], [790, 400], [790, 430], [720, 430]], badge: [815, 415], label: 'runs the tool', at: [755, 392] };

function Label({ text, at, anchor = 'middle' }) {
  const [x, y] = at;
  return (
    <text x={x} y={y} text-anchor={anchor} class="dg-label dg-label-sub">
      {text}
    </text>
  );
}

export default function InteractionDiagram({ id = 'flow' }) {
  const { shownId, pinnedId, select, setHover } = useSelection();
  const info = shownId ? STEP_INFO[shownId] : null;
  const chips = info ? info.chips.map((c) => ({ tone: 'info', id: c, text: REF_SHORT[c] })) : [];

  const badges = [];

  return (
    <Diagram
      id={id}
      viewH={VIEW_H}
      label="Sequence diagram of one exchange turn with a tool call, from the operator's reply to the next interrupt"
      footer={
        <DetailPanel
          id={shownId}
          title={info?.name}
          desc={info?.desc}
          chips={chips}
          hint="Hover or click a numbered step to see what happens there and which file does it."
        />
      }
    >
      {Object.entries(LANES).map(([key, l]) => (
        <g key={key}>
          <Box x={l.x - HEAD_W / 2} y={20} w={HEAD_W} h={HEAD_H} label={l.label} sub={l.sub} small variant={key === 'claude' ? 'solid-safe' : 'default'} />
          <Edge id={`life-${key}`} points={[[l.x, LIFE_TOP], [l.x, LIFE_BOTTOM]]} dashed />
        </g>
      ))}

      {MESSAGES.map((m, i) => {
        const x0 = LANES[m.from].x;
        const x1 = LANES[m.to].x;
        // Badge + label go to the arrow's midpoint unless that is another
        // lane's lifeline (lanes are 160 apart), then one half-gap left.
        let mid = (x0 + x1) / 2;
        if (Object.values(LANES).some((l) => l.x === mid)) mid -= 80;
        const active = m.badge && shownId === m.badge;
        if (m.badge) badges.push({ id: m.badge, x: mid, y: m.y });
        return (
          <g key={i}>
            <Edge id={`msg-${i}`} points={[[x0, m.y], [x1, m.y]]} dir={m.dir || 'end'} active={active} />
            <Label text={m.label} at={[mid, m.y - (m.badge ? 18 : 6)]} />
          </g>
        );
      })}

      <Edge id="self-loop" points={SELF_LOOP.points} dir="end" active={shownId === 'S-6'} />
      <Label text={SELF_LOOP.label} at={SELF_LOOP.at} anchor="start" />

      {[...badges, { id: 'S-6', x: SELF_LOOP.badge[0], y: SELF_LOOP.badge[1] }]
        .sort((a, b) => a.id.localeCompare(b.id))
        .map((b) => (
          <Badge
            key={b.id}
            id={b.id}
            label={b.id.replace('S-', '')}
            name={STEP_INFO[b.id].name}
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
