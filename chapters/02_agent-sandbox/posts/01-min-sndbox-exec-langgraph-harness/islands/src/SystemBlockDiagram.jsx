// System block diagram of the minimal harness as implemented in
// src/agent_sandbox: the concrete components (main.py/graph.py, the Docker
// Engine reached via docker.from_env(), the claude CLI process with its login
// state and writable rootfs, the gVisor Sentry, the host kernel) and the six
// interaction points between them. `runtime` selects what IP-4 looks like:
// 'runsc' (the config default) splits it into 4a/4b around the Sentry;
// 'runc' (what CONTAINER_RUNTIME=runc in the environment gives you) is one
// hop straight to the host kernel.
//
// Geometry is hand laid out in viewBox units (VIEW_W = 960 wide). Badges sit
// on an edge when it is long enough (GEOM.badgeOnEdgeMin) to still show the
// line and its arrowheads around the circle; otherwise beside it.
import { Diagram, Outline, Box, Edge, Badge, DetailPanel, useSelection } from './diagram/primitives.jsx';
import { POINT_INFO, REF_SHORT } from './data.js';

const NODES = {
  operator: { x: 20, y: 20, w: 120, h: 56 },
  terminal: { x: 180, y: 20, w: 200, h: 56 },
  orchestrator: { x: 50, y: 140, w: 220, h: 80 },
  engine: { x: 340, y: 140, w: 180, h: 80 },
  cli: { x: 370, y: 290, w: 290, h: 70 },
  login: { x: 370, y: 400, w: 120, h: 60 },
  rootfs: { x: 540, y: 400, w: 120, h: 60 },
  sentry: { x: 340, y: 500, w: 350, h: 56 },
  api: { x: 770, y: 140, w: 150, h: 56 },
  anything: { x: 770, y: 230, w: 150, h: 56 },
  siblings: { x: 770, y: 320, w: 150, h: 56 },
};

function layout(runtime) {
  const isRunc = runtime === 'runc';
  const kernelY = isRunc ? 520 : 600;
  const hostH = isRunc ? 520 : 600;
  return {
    isRunc,
    viewH: kernelY + 100,
    kernel: { x: 50, y: kernelY, w: 640, h: 70 },
    outlines: {
      host: { x: 20, y: 100, w: 700, h: hostH },
      container: { x: 340, y: 250, w: 350, h: 230 },
      outside: { x: 750, y: 100, w: 190, h: hostH },
    },
    // One entry per interaction point: the edge it lives on and where its
    // badge goes. Segments are horizontal or vertical only.
    edges: [
      { id: 'IP-1', points: [[280, 76], [280, 108], [160, 108], [160, 140]], badge: [220, 108], dir: 'both' },
      { id: 'IP-2', points: [[270, 180], [340, 180]], badge: [305, 180], dir: 'end' },
      { id: 'IP-3', points: [[160, 220], [160, 325], [370, 325]], badge: [265, 325], dir: 'both' },
      ...(isRunc
        ? [{ id: 'IP-4', points: [[515, 360], [515, kernelY]], badge: [515, 440], dir: 'end' }]
        : [
            { id: 'IP-4a', points: [[515, 360], [515, 500]], badge: [515, 430], dir: 'end' },
            { id: 'IP-4b', points: [[515, 556], [515, kernelY]], badge: [545, 578], dir: 'end' },
          ]),
      { id: 'IP-5', points: [[600, 360], [600, 400]], badge: [630, 380], dir: 'both' },
      { id: 'IP-6', points: [[660, 325], [740, 325]], badge: [705, 325], dir: 'both' },
    ],
    // Plain relations that are not interaction points: operator ↔ terminal,
    // containers.run() creating the container, the CLI writing its login
    // state, the harness sharing the kernel, and the bridge fanning out.
    plain: [
      { points: [[140, 48], [180, 48]], dir: 'both' },
      { points: [[430, 220], [430, 250]], dir: 'end' },
      { points: [[430, 360], [430, 400]], dir: 'end' },
      { points: [[80, 220], [80, kernelY]], dashed: true },
      { points: [[740, 168], [740, 348]] },
      { points: [[740, 168], [770, 168]], dir: 'end' },
      { points: [[740, 258], [770, 258]], dir: 'end' },
      { points: [[740, 348], [770, 348]], dir: 'end' },
    ],
  };
}

export default function SystemBlockDiagram({ id = 'boundary', runtime = 'runsc' }) {
  const { shownId, pinnedId, select, setHover } = useSelection();
  const L = layout(runtime);
  const points = POINT_INFO[runtime];
  const info = shownId ? points[shownId] : null;

  const chips = info
    ? [
        ...info.uc.map((u) => ({ tone: 'info', id: u, text: REF_SHORT[u] })),
        ...info.closed.map((t) => ({ tone: 'safe', id: t, text: REF_SHORT[t] })),
        ...info.open.map((t) => ({ tone: 'danger', id: t, text: REF_SHORT[t] })),
      ]
    : [];

  return (
    <Diagram
      id={id}
      viewH={L.viewH}
      label={`System block diagram of the minimal harness under runtime=${runtime} with its interaction points`}
      footer={
        <DetailPanel
          id={shownId}
          title={info?.name}
          desc={info ? `${info.code}. ${info.desc}` : undefined}
          chips={chips}
          hint="Hover or click a numbered badge for where that interaction point lives in the code, the use cases on it, and the threat cases it closes (green) or leaves open (red)."
        />
      }
    >
      <Outline {...L.outlines.host} label="HOST SYSTEM" variant={L.isRunc ? 'danger' : 'neutral'} />
      <Outline {...L.outlines.container} label="AGENT CONTAINER" variant={L.isRunc ? 'danger' : 'safe'} filled />
      <Outline {...L.outlines.outside} label="BRIDGE NETWORK" variant="danger" />

      <Box {...NODES.operator} label="Operator" sub="(human)" />
      <Box {...NODES.terminal} label="Terminal emulator" sub="input() · print()" />
      <Box {...NODES.orchestrator} label="Host process" sub="main.py → graph.py · MemorySaver" />
      <Box {...NODES.engine} label="Docker Engine" sub="docker.from_env()" />
      <Box {...NODES.cli} label="claude CLI" sub="auth login · -p --output-format text" />
      <Box {...NODES.login} label="Login state" sub="OAuth session" small />
      <Box {...NODES.rootfs} label="Rootfs" sub="read_only=False" small />
      {L.isRunc ? null : <Box {...NODES.sentry} label="gVisor Sentry" sub="runtime=runsc · userspace kernel" variant="solid-safe" />}
      <Box {...L.kernel} label="HOST KERNEL" sub={L.isRunc ? 'shared — one hop from the container' : 'shared by host process, engine and Sentry'} variant={L.isRunc ? 'solid-danger' : 'solid-dark'} />
      <Box {...NODES.api} label="Anthropic API" sub="OAuth + inference" small />
      <Box {...NODES.anything} label="Anything else" sub="registries · IMDS · host" small />
      <Box {...NODES.siblings} label="Sibling sandboxes" sub="same default bridge" small />

      {L.plain.map((e, i) => (
        <Edge key={i} {...e} />
      ))}
      {L.edges.map((e) => (
        <Edge key={e.id} id={e.id} points={e.points} dir={e.dir} active={shownId === e.id} />
      ))}
      {L.edges.map((e) => (
        <Badge
          key={e.id}
          id={e.id}
          label={e.id.replace('IP-', '')}
          name={points[e.id].name}
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
