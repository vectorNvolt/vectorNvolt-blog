// Generic system boundary of a container-sandboxed agent harness: the host
// system, the agent container nested inside it, the six interaction points
// (IP-1…IP-6, IP-4 split into 4a/4b by the gVisor Sentry) and, around them,
// the entities the threat inventory needs to name — the operator's terminal
// emulator, the daemon socket, the credentials and filesystem inside the
// container, the host kernel, and what sits on the far side of egress.
//
// Geometry is hand laid out in viewBox units (VIEW_W = 960 wide). Badges sit
// on an edge when it is long enough (GEOM.badgeOnEdgeMin) to still show the
// line and its arrowheads around the circle; otherwise beside it.
import { Diagram, Outline, Box, Edge, Badge, DetailPanel, useSelection } from './diagram/primitives.jsx';
import { POINT_INFO, REF_SHORT } from './data.js';

const VIEW_H = 720;

const NODES = {
  operator: { x: 20, y: 20, w: 120, h: 56 },
  terminal: { x: 180, y: 20, w: 200, h: 56 },
  orchestrator: { x: 50, y: 140, w: 220, h: 80 },
  engine: { x: 340, y: 140, w: 180, h: 80 },
  agent: { x: 370, y: 290, w: 290, h: 70 },
  credentials: { x: 370, y: 400, w: 120, h: 60 },
  fs: { x: 540, y: 400, w: 120, h: 60 },
  sentry: { x: 340, y: 500, w: 350, h: 56 },
  kernel: { x: 50, y: 600, w: 640, h: 70 },
  api: { x: 770, y: 140, w: 150, h: 56 },
  attacker: { x: 770, y: 230, w: 150, h: 56 },
  registry: { x: 770, y: 320, w: 150, h: 56 },
  metadata: { x: 770, y: 410, w: 150, h: 56 },
  internal: { x: 770, y: 500, w: 150, h: 56 },
};

const OUTLINES = {
  host: { x: 20, y: 100, w: 700, h: 600 },
  container: { x: 340, y: 250, w: 350, h: 230 },
  outside: { x: 750, y: 100, w: 190, h: 600 },
};

// One entry per interaction point: the edge it lives on and where its badge
// goes. Segments are horizontal or vertical only (Edge throws on a diagonal).
const EDGES = [
  { id: 'IP-1', points: [[280, 76], [280, 108], [160, 108], [160, 140]], badge: [220, 108], dir: 'both' },
  { id: 'IP-2', points: [[270, 180], [340, 180]], badge: [305, 180], dir: 'end' },
  { id: 'IP-3', points: [[160, 220], [160, 325], [370, 325]], badge: [265, 325], dir: 'both' },
  { id: 'IP-4a', points: [[515, 360], [515, 500]], badge: [515, 430], dir: 'end' },
  { id: 'IP-4b', points: [[515, 556], [515, 600]], badge: [545, 578], dir: 'end' },
  { id: 'IP-5', points: [[600, 360], [600, 400]], badge: [630, 380], dir: 'both' },
  { id: 'IP-6', points: [[660, 325], [740, 325]], badge: [705, 325], dir: 'both' },
];

// Plain relations that are not interaction points: operator ↔ terminal,
// engine creating the container, host processes running on the kernel, and
// the network bus fanning egress out to whatever is reachable.
const PLAIN_EDGES = [
  { points: [[140, 48], [180, 48]], dir: 'both' },
  { points: [[430, 220], [430, 250]], dir: 'end' },
  { points: [[80, 220], [80, 600]], dashed: true },
  { points: [[740, 168], [740, 528]] },
  { points: [[740, 168], [770, 168]], dir: 'end' },
  { points: [[740, 258], [770, 258]], dir: 'end' },
  { points: [[740, 348], [770, 348]], dir: 'end' },
  { points: [[740, 438], [770, 438]], dir: 'end' },
  { points: [[740, 528], [770, 528]], dir: 'end' },
];

export default function BoundaryDiagram({ id = 'boundary' }) {
  const { shownId, pinnedId, select, setHover } = useSelection();
  const info = shownId ? POINT_INFO[shownId] : null;

  const chips = info
    ? [
        ...info.t.map((t) => ({ tone: 'danger', id: t, text: REF_SHORT[t] })),
        ...info.l.map((l) => ({ tone: 'safe', id: l, text: REF_SHORT[l] })),
      ]
    : [];

  return (
    <Diagram
      id={id}
      viewH={VIEW_H}
      label="System boundary of a container-sandboxed agent harness with its six interaction points"
      footer={
        <DetailPanel
          id={shownId}
          title={info?.name}
          desc={info ? `${info.iface}. ${info.desc}` : undefined}
          chips={chips}
          hint="Hover or click a numbered badge to see the interface behind that interaction point, the threat vectors that land on it and the layers that harden it."
        />
      }
    >
      <Outline {...OUTLINES.host} label="HOST SYSTEM" />
      <Outline {...OUTLINES.container} label="AGENT CONTAINER" variant="danger" filled />
      <Outline {...OUTLINES.outside} label="OUTSIDE" variant="danger" />

      <Box {...NODES.operator} label="Operator" sub="(human)" />
      <Box {...NODES.terminal} label="Terminal emulator" sub="interprets ESC/OSC · T9" />
      <Box {...NODES.orchestrator} label="Host orchestrator" sub="LangGraph harness · M and I" />
      <Box {...NODES.engine} label="Docker Engine" sub="daemon socket · T8" />
      <Box {...NODES.agent} label="Agent process" sub="untrusted by construction · U" />
      <Box {...NODES.credentials} label="Credentials" sub="the prize · T1" small />
      <Box {...NODES.fs} label="Filesystem" sub="writable, reused · T5" small />
      <Box {...NODES.sentry} label="gVisor Sentry" sub="userspace kernel · T6g–l residual" variant="solid-safe" />
      <Box {...NODES.kernel} label="HOST KERNEL" sub="shared by everything on the box · T6 T7" variant="solid-dark" />
      <Box {...NODES.api} label="Model API" sub="legitimate egress" small />
      <Box {...NODES.attacker} label="Attacker host" sub="exfil sink, injection · T1 T9" small />
      <Box {...NODES.registry} label="Package registry" sub="slopsquatted deps · T2" small />
      <Box {...NODES.metadata} label="Cloud metadata" sub="169.254.169.254 · T4" small />
      <Box {...NODES.internal} label="Internal network" sub="host services · T4" small />

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
          label={e.id.replace('IP-', '')}
          name={POINT_INFO[e.id].name}
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
