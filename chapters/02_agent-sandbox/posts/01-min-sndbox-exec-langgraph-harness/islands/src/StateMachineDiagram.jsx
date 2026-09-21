// State machine of the minimal harness: the six LangGraph nodes wired in
// graph.py, the conditional edges from route_after_*(), the two self-loops
// (auth_gateway, exchange) and the nodes that pause on interrupt(). Badges
// sit on the nodes; the panel shows what each node calls, the interaction
// points it touches and the threat cases that live there.
//
// Geometry is hand laid out in viewBox units (VIEW_W = 960 wide).
import { Diagram, Box, Edge, Badge, DetailPanel, useSelection } from './diagram/primitives.jsx';
import { NODE_INFO, REF_SHORT } from './data.js';

const VIEW_H = 450;

const NODES = {
  start: { x: 20, y: 60, w: 60, h: 60 },
  N1: { x: 110, y: 60, w: 150, h: 60 },
  N2: { x: 290, y: 60, w: 150, h: 60 },
  N3: { x: 470, y: 60, w: 150, h: 60 },
  N4: { x: 650, y: 60, w: 150, h: 60 },
  N5: { x: 650, y: 250, w: 150, h: 60 },
  N6: { x: 470, y: 250, w: 150, h: 60 },
  end: { x: 470, y: 390, w: 150, h: 44 },
};

// Transitions. `label` is drawn beside the edge at `at`. Segments are
// horizontal or vertical only (Edge throws on a diagonal).
const EDGES = [
  { id: 'start', points: [[80, 90], [110, 90]], dir: 'end' },
  { id: '1-2', points: [[260, 90], [290, 90]], dir: 'end' },
  { id: '2-3', points: [[440, 90], [470, 90]], dir: 'end', label: 'no container', at: [455, 138], anchor: 'middle' },
  { id: '2-4', points: [[365, 120], [365, 160], [690, 160], [690, 120]], dir: 'end', label: 'container exists → reattach', at: [527, 174], anchor: 'middle' },
  { id: '3-4', points: [[620, 90], [650, 90]], dir: 'end' },
  { id: '4-4', points: [[770, 60], [770, 30], [700, 30], [700, 60]], dir: 'end', label: 'not authenticated · attempts < MAX', at: [735, 22], anchor: 'middle' },
  { id: '4-5', points: [[760, 120], [760, 250]], dir: 'end', label: 'authenticated', at: [770, 220], anchor: 'start' },
  { id: '4-end', points: [[800, 90], [840, 90], [840, 412], [620, 412]], dir: 'end', label: 'attempts ≥ MAX_AUTH_ATTEMPTS', at: [850, 300], anchor: 'start', vertical: true },
  { id: '5-6', points: [[650, 280], [620, 280]], dir: 'end' },
  { id: '6-6', points: [[470, 262], [440, 262], [440, 290], [470, 290]], dir: 'end', label: 'agent ↔ user turn', at: [432, 305], anchor: 'end' },
  { id: '6-end', points: [[545, 310], [545, 390]], dir: 'end', label: 'done (/quit)', at: [555, 355], anchor: 'start' },
];

// Which nodes pause the graph on interrupt() and what the operator sees.
const SUB = {
  N1: 'interrupt() · name',
  N2: 'find() · ensure_running()',
  N3: 'containers.run(runsc…)',
  N4: 'interrupt() · OAuth code',
  N5: 'interrupt() · first task',
  N6: 'interrupt() · claude -p',
};

function EdgeLabel({ text, at, anchor = 'middle', vertical }) {
  const [x, y] = at;
  return (
    <text x={x} y={y} text-anchor={anchor} class="dg-label dg-label-sub" transform={vertical ? `rotate(-90 ${x} ${y})` : undefined}>
      {text}
    </text>
  );
}

export default function StateMachineDiagram({ id = 'graph' }) {
  const { shownId, pinnedId, select, setHover } = useSelection();
  const info = shownId ? NODE_INFO[shownId] : null;

  const chips = info
    ? [
        ...info.ip.map((p) => ({ tone: 'info', id: p, text: REF_SHORT[p] })),
        ...info.t.map((t) => ({ tone: 'danger', id: t, text: REF_SHORT[t] })),
      ]
    : [];

  const nodeIds = Object.keys(NODE_INFO);

  return (
    <Diagram
      id={id}
      viewH={VIEW_H}
      label="LangGraph state machine of the minimal harness: six nodes, two self-loops, four interrupt points"
      footer={
        <DetailPanel
          id={shownId}
          title={info?.name}
          desc={info?.desc}
          chips={chips}
          hint="Hover or click a numbered node for what it calls, the interaction points it touches, and the threat cases that live there."
        />
      }
    >
      <Box {...NODES.start} label="START" variant="solid-dark" small />
      <Box {...NODES.end} label="END" variant="solid-dark" small />
      {nodeIds.map((n) => (
        <Box key={n} {...NODES[n]} label={NODE_INFO[n].name.replace(' (loop)', '')} sub={SUB[n]} />
      ))}

      {EDGES.map((e) => (
        <Edge key={e.id} id={e.id} points={e.points} dir={e.dir} />
      ))}
      {EDGES.filter((e) => e.label).map((e) => (
        <EdgeLabel key={`${e.id}-label`} text={e.label} at={e.at} anchor={e.anchor} vertical={e.vertical} />
      ))}

      {nodeIds.map((n) => (
        <Badge
          key={n}
          id={n}
          label={n.replace('N', '')}
          name={NODE_INFO[n].name}
          x={NODES[n].x}
          y={NODES[n].y}
          active={shownId === n}
          pressed={pinnedId === n}
          onSelect={select}
          onHover={setHover}
        />
      ))}
    </Diagram>
  );
}
