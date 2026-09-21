// Block diagram of the rendering solution: the operator terminal, the host
// process (main.py, node 6, gateway, renderer, exec session), the Docker
// engine, and the persistent `claude` process inside the gVisor container that
// talks to the Anthropic API. Badges B-1…B-7 sit on the links; the panel
// explains what crosses each one.
//
// Geometry is hand laid out in viewBox units (VIEW_W = 960 wide). Segments are
// horizontal or vertical only (Edge throws on a diagonal).
import { Diagram, Outline, Box, Edge, Badge, DetailPanel, useSelection } from './diagram/primitives.jsx';
import { BLOCK_INFO, REF_SHORT } from './data.js';

const VIEW_H = 430;

const NODES = {
  terminal: { x: 20, y: 20, w: 460, h: 56 },
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

// One entry per link with a badge. Badge on the edge when the run is long
// enough, otherwise beside it.
const EDGES = [
  { id: 'B-1', points: [[95, 140], [95, 76]], badge: [95, 108], dir: 'both' },
  { id: 'B-2', points: [[235, 140], [235, 76]], badge: [235, 108], dir: 'end' },
  { id: 'B-3', points: [[235, 240], [235, 196]], badge: [257, 218], dir: 'end' },
  { id: 'B-4', points: [[315, 296], [315, 330]], badge: [337, 313], dir: 'both' },
  { id: 'B-5', points: [[460, 358], [580, 358], [580, 220]], badge: [520, 358], dir: 'both' },
  { id: 'B-6', points: [[640, 185], [700, 185]], badge: [670, 185], dir: 'both' },
  { id: 'B-7', points: [[895, 150], [895, 76]], badge: [895, 113], dir: 'both' },
];

// Plain relations without a badge.
const PLAIN_EDGES = [
  { points: [[440, 196], [440, 240]], dir: 'end' },          // node 6 → gateway
  { points: [[765, 220], [765, 250]], dashed: true },         // claude ⇢ history
  { points: [[880, 220], [880, 330]], dir: 'end', dashed: true }, // claude ⇢ tools
];

export default function BlockDiagram({ id = 'blocks' }) {
  const { shownId, pinnedId, select, setHover } = useSelection();
  const info = shownId ? BLOCK_INFO[shownId] : null;
  const chips = info ? info.chips.map((c) => ({ tone: 'info', id: c, text: REF_SHORT[c] })) : [];

  return (
    <Diagram
      id={id}
      viewH={VIEW_H}
      label="Block diagram: operator terminal, host process, Docker engine, persistent claude process in the sandbox, Anthropic API"
      footer={
        <DetailPanel
          id={shownId}
          title={info?.name}
          desc={info?.desc}
          chips={chips}
          hint="Hover or click a numbered badge to see what crosses that link and which file implements it."
        />
      }
    >
      <Outline {...OUTLINES.host} label="HOST PROCESS · agent-sandbox" variant="info" />
      <Outline {...OUTLINES.container} label="AGENT CONTAINER · runsc" variant="safe" filled />

      <Box {...NODES.terminal} label="Operator terminal" sub="stdin ← input()   ·   stdout ← rich Console" />
      <Box {...NODES.api} label="Anthropic API" sub="Messages · prompt cache" />
      <Box {...NODES.main} label="main.py" sub="graph.invoke()" />
      <Box {...NODES.render} label="render.py" sub="TurnRenderer · THEME" />
      <Box {...NODES.n06} label="n06_exchange" sub="graph node (loop)" />
      <Box {...NODES.gateway} label="gateway.py" sub="start_chat() · send_chat_turn() · close_chat()" />
      <Box {...NODES.session} label="container.py · ExecSession" sub="tty=False · readline() · _demux()" />
      <Box {...NODES.engine} label="Docker Engine" sub="exec API" />
      <Box {...NODES.claude} label="claude -p" sub="stream-json in · stream-json out" variant="solid-safe" />
      <Box {...NODES.history} label="conversation" sub="in-process" small />
      <Box {...NODES.tools} label="tools · Bash, Read, Edit…" sub="--permission-mode auto" small />

      {PLAIN_EDGES.map((e, i) => (
        <Edge key={i} {...e} />
      ))}
      {EDGES.map((e) => (
        <Edge key={e.id} id={e.id} points={e.points} dir={e.dir} active={shownId === e.id} />
      ))}
      {EDGES.map((e) => (
        <Badge
          key={e.id}
          id={e.id}
          label={e.id.replace('B-', '')}
          name={BLOCK_INFO[e.id].name}
          x={e.badge[0]}
          y={e.badge[1]}
          active={shownId === e.id}
          pressed={pinnedId === e.id}
          onSelect={select}
          onHover={setHover}
        />
      ))}
    </Diagram>
  );
}
