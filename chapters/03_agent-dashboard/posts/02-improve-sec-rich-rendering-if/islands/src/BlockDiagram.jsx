// Block diagram of the rendering interface — the post-01 topology (operator
// terminal, host process, Docker engine, `claude` in the sandbox) with the
// operator split from the terminal emulator, because both are interpreters.
// One drawing, two overlays picked by `variant`:
//   threat  V-1…V-7  where sandbox bytes are interpreted on the host
//   fix     C-1…C-6  the choke points this post adds
// The container is `danger` in both: from the interface's point of view it is
// the untrusted source, whatever gVisor does for the sandbox itself. The host
// outline flips from danger to safe.
//
// Geometry is hand laid out in viewBox units (VIEW_W = 960 wide). Segments are
// horizontal or vertical only (Edge throws on a diagonal).
import { Diagram, Outline, Box, Edge, Badge, DetailPanel, useSelection } from './diagram/primitives.jsx';
import { THREAT_INFO, CHOKE_INFO, REF_SHORT } from './data.js';

const VIEW_H = 430;

const NODES = {
  operator: { x: 20, y: 20, w: 106, h: 56 },
  terminal: { x: 150, y: 20, w: 330, h: 56 },
  api: { x: 780, y: 20, w: 160, h: 56 },
  main: { x: 40, y: 140, w: 110, h: 56 },
  render: { x: 170, y: 140, w: 130, h: 56 },
  n06: { x: 320, y: 140, w: 140, h: 56 },
  gateway: { x: 170, y: 240, w: 290, h: 56 },
  session: { x: 170, y: 330, w: 290, h: 56 },
  engine: { x: 520, y: 150, w: 120, h: 70 },
  claude: { x: 700, y: 150, w: 210, h: 70 },
  history: { x: 700, y: 250, w: 130, h: 50 },
  tools: { x: 700, y: 330, w: 210, h: 50 },
};

const OUTLINES = {
  host: { x: 20, y: 110, w: 460, h: 300 },
  container: { x: 670, y: 110, w: 270, h: 300 },
};

// The links of the post-01 diagram. B-1 has to elbow to reach the terminal
// box now that the operator sits to its left; the run and the badges stay
// above the host outline's top edge (y = 110).
const LINKS = {
  'B-1': { points: [[100, 140], [100, 93], [190, 93], [190, 76]], badge: [145, 93], dir: 'both' },
  'B-2': { points: [[235, 140], [235, 76]], badge: [235, 96], dir: 'end' },
  'B-3': { points: [[235, 240], [235, 196]], badge: [257, 218], dir: 'end' },
  'B-4': { points: [[315, 296], [315, 330]], badge: [337, 313], dir: 'both' },
};

const PLAIN_EDGES = [
  { points: [[126, 48], [150, 48]] },                              // operator — terminal
  { points: [[440, 196], [440, 240]], dir: 'end' },                // node 6 → gateway
  { points: [[460, 358], [580, 358], [580, 220]], dir: 'both' },   // B-5 exec socket
  { points: [[640, 185], [700, 185]], dir: 'both' },               // B-6 process stdin/stdout
  { points: [[860, 150], [860, 76]], dir: 'both' },                // B-7 CLI ⇄ API (left of the corner badge)
  { points: [[765, 220], [765, 250]], dashed: true },              // claude ⇢ history
  { points: [[880, 220], [880, 330]], dir: 'end', dashed: true },  // claude ⇢ tools
];

// A badge hanging on a box's top-right corner (a hotspot on a box, not a link).
const corner = (n) => [n.x + n.w, n.y];

// Per-variant: which badges exist, where they sit, and which link they light up.
const OVERLAYS = {
  threat: {
    info: THREAT_INFO,
    badges: [
      { id: 'V-1', at: corner(NODES.operator) },
      { id: 'V-2', at: corner(NODES.terminal) },
      { id: 'V-3', at: LINKS['B-1'].badge, link: 'B-1' },
      { id: 'V-4', at: LINKS['B-2'].badge, link: 'B-2' },
      { id: 'V-5', at: LINKS['B-3'].badge, link: 'B-3' },
      { id: 'V-6', at: LINKS['B-4'].badge, link: 'B-4' },
      { id: 'V-7', at: corner(NODES.claude) },
    ],
    host: 'danger',
    hostLabel: 'HOST',
    subs: {
      main: 'input()',
      render: 'Rich · no sanitiser',
      gateway: 'json.loads() · type dispatch',
      session: 'readline() · unbounded',
    },
    hint: 'Hover or click a badge to see which interpreter acts on the bytes there and which threat cases land on it.',
    label: 'Block diagram of the rendering interface with the interpreters on the host marked: operator, terminal emulator, input(), Rich, json.loads, readline(), and the untrusted claude process',
  },
  fix: {
    info: CHOKE_INFO,
    badges: [
      { id: 'C-1', at: LINKS['B-1'].badge, link: 'B-1' },
      { id: 'C-2', at: LINKS['B-2'].badge, link: 'B-2' },
      { id: 'C-3', at: LINKS['B-3'].badge, link: 'B-3' },
      { id: 'C-4', at: LINKS['B-4'].badge, link: 'B-4' },
      { id: 'C-5', at: corner(NODES.claude) },
      { id: 'C-6', at: corner(NODES.terminal) },
    ],
    host: 'safe',
    hostLabel: 'HOST',
    subs: {
      main: 'tcflush() · input()',
      render: 'sanitize() → Rich',
      gateway: 'schema · guarded loads',
      session: 'readline() · line cap',
    },
    hint: 'Hover or click a badge to see what the choke point does and which cases it closes.',
    label: 'Block diagram of the rendering interface with the choke points marked: tcflush before input(), sanitize() before Rich, schema and bounds on the exec channel, envelope metadata rendered as reported',
  },
};

export default function BlockDiagram({ id = 'blocks', variant = 'threat' }) {
  const ov = OVERLAYS[variant];
  const { shownId, pinnedId, select, setHover } = useSelection();
  const info = shownId ? ov.info[shownId] : null;
  const chips = info
    ? [
        ...(info.tc || []).map((c) => ({ tone: 'danger', id: c, text: REF_SHORT[c] })),
        ...(info.fix || []).map((c) => ({ tone: 'safe', id: c, text: REF_SHORT[c] })),
        ...(info.code || []).map((c) => ({ tone: 'info', id: c, text: REF_SHORT[c] })),
      ]
    : [];
  const activeLink = shownId ? ov.badges.find((b) => b.id === shownId)?.link : null;

  return (
    <Diagram
      id={id}
      viewH={VIEW_H}
      label={ov.label}
      footer={<DetailPanel id={shownId} title={info?.name} desc={info?.desc} chips={chips} hint={ov.hint} />}
    >
      <Outline {...OUTLINES.host} label={ov.hostLabel} variant={ov.host} />
      <Outline {...OUTLINES.container} label="CONTAINER · untrusted source" variant="danger" filled />

      <Box {...NODES.operator} label="Operator" sub="human" />
      <Box {...NODES.terminal} label="Terminal emulator" sub="VT parser · clipboard · title · pty" />
      <Box {...NODES.api} label="Anthropic API" sub="Messages · prompt cache" />
      <Box {...NODES.main} label="main.py" sub={ov.subs.main} />
      <Box {...NODES.render} label="render.py" sub={ov.subs.render} />
      <Box {...NODES.n06} label="n06_exchange" sub="graph node (loop)" />
      <Box {...NODES.gateway} label="gateway.py" sub={ov.subs.gateway} />
      <Box {...NODES.session} label="container.py · ExecSession" sub={ov.subs.session} />
      <Box {...NODES.engine} label="Docker Engine" sub="exec API" />
      <Box {...NODES.claude} label="claude -p" sub="writable rootfs · on PATH" variant="solid-danger" />
      <Box {...NODES.history} label="conversation" sub="in-process" small />
      <Box {...NODES.tools} label="tools · Bash, Read, Edit…" sub="--permission-mode auto" small />

      {PLAIN_EDGES.map((e, i) => (
        <Edge key={i} {...e} />
      ))}
      {Object.entries(LINKS).map(([lid, l]) => (
        <Edge key={lid} id={lid} points={l.points} dir={l.dir} active={activeLink === lid} />
      ))}
      {ov.badges.map((b) => (
        <Badge
          key={b.id}
          id={b.id}
          label={b.id.replace('-', '')}
          name={ov.info[b.id].name}
          x={b.at[0]}
          y={b.at[1]}
          active={shownId === b.id}
          pressed={pinnedId === b.id}
          onSelect={select}
          onHover={setHover}
        />
      ))}
    </Diagram>
  );
}
