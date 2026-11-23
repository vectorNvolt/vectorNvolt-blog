// Sequence diagram of one capability group in operation: the lanes are the
// components the group's requirements are allocated to, the phases are the
// outcomes the group has to produce (a refusal, a service inside the bound,
// the provisioning that sets the bound), and every numbered step names the
// requirement it realizes.
//
// Unlike the block diagrams this one lays itself out: lane x from the lane
// count, step y from the step order, so a group spec lists only lanes,
// phases and steps. Every connector is still horizontal or vertical — a
// self-message is a rectangular loop off the lifeline, drawn towards the
// side with room for its text.
//
// A phase's `steps` list holds step ids and combined fragments. A scoped
// goal is drawn as the request, the scope check, then an `alt` fragment with
// one operand per answer:
//   { alt: [{ guard: 'in scope', tone: 'safe', steps: [...] },
//           { guard: 'else — out of scope', tone: 'danger', steps: [...] }] }
// An operand's `steps` may hold fragments again (a second check inside the
// first one's bound); each level is inset from the one around it.
//
// Optional (absent ⇒ drawn exactly as without them):
//   text.steps[id].tests   the tests that exercise the step, a string or a
//                          list; a panel line headed `text.testsLabel` (default
//                          'Tests'). List items are joined with ', ' and get a
//                          zero-width break after each '_' so long names wrap.
//   text.steps[id].code    the function that performs the step, a panel line
//                          headed `text.codeLabel` (default 'Code:').
//   text.steps[id].ips / .assets   interaction points / assets the step crosses
//                          or touches: chips toned as the block diagrams'
//                          hotspots (IP neutral, asset info), after the threats.
//   spec.steps[].code      a short code label drawn in the diagram under the
//                          message line (beside a self-message's text) — the
//                          code overlay; usually with a larger `spec.stepH`.
//   spec.stepH             message pitch in viewBox units (default 46), so an
//                          overlay row fits between two messages.
import { Diagram, Outline, Fragment, Box, Edge, Badge, DetailPanel, useSelection } from './diagram/primitives.jsx';
import { REF_SHORT } from './data.js';

const LEFT = 8; // phase outlines
const RIGHT = 952;
const LANE_LEFT = 52; // lanes start right of the phase labels' column
const LANE_TOP = 20;
const LANE_H = 56;
const FIRST_PHASE_Y = 96;
const PHASE_HEAD = 56; // outline top → first step (room for the phase label)
const PHASE_TAIL = 24; // last step → outline bottom
const PHASE_GAP = 16;
const STEP = 46; // a message
const LOOP_H = 28; // a self-message's loop height
const LOOP_W = 56;
const LOOP_STEP = 64;
const FRAG_INSET = 12; // a fragment's frame inside the phase / fragment around it
const FRAG_ABOVE = 26; // next step's y → the frame top or separator it opens
const FRAG_HEAD = 50; // frame top or separator → first step (tag, guard, message label)
const FRAG_BELOW = 20; // y after an operand's last step → its separator / frame bottom
const FRAG_AFTER = 30; // frame bottom → next step
const HEADER_H = 22; // tag / guard row; a nested frame starts below it

function layout(spec) {
  const n = spec.lanes.length;
  const laneW = (RIGHT - LANE_LEFT) / n;
  const lanes = spec.lanes.map((l, i) => {
    const c = Math.round(LANE_LEFT + laneW * (i + 0.5));
    const w = Math.min(Math.round(laneW) - 24, 200);
    return { ...l, c, box: { x: c - Math.round(w / 2), y: LANE_TOP, w, h: LANE_H } };
  });
  const laneX = Object.fromEntries(lanes.map((l) => [l.key, l.c]));
  const stepH = spec.stepH ?? STEP;

  const phases = [];
  const frags = [];
  const msgs = [];
  let y = FIRST_PHASE_Y;
  // Lowest y a frame may start at: just below the header row (phase label,
  // fragment tag or guard) that the current item follows.
  let minTop = 0;

  const placeStep = (stepId) => {
    const s = spec.steps.find((st) => st.id === stepId);
    if (!s) throw new Error(`SequenceDiagram: unknown step ${stepId}`);
    const x0 = laneX[s.from];
    const x1 = laneX[s.to];
    if (x0 === x1) {
      const left = x0 > (LANE_LEFT + RIGHT) / 2;
      const dx = left ? -LOOP_W : LOOP_W;
      msgs.push({
        ...s,
        points: [[x0, y], [x0 + dx, y], [x0 + dx, y + LOOP_H], [x0, y + LOOP_H]],
        badge: [x0 + dx + (left ? -20 : 20), y + LOOP_H / 2],
        tx: x0 + dx + (left ? -40 : 40),
        ty: y + LOOP_H / 2 + 4,
        cy: y + LOOP_H / 2 + 18,
        anchor: left ? 'end' : 'start',
      });
      y += LOOP_STEP;
    } else {
      const ltr = x1 > x0;
      msgs.push({
        ...s,
        points: [[x0, y], [x1, y]],
        badge: [Math.round((x0 + x1) / 2), y],
        tx: ltr ? x0 + 10 : x0 - 10,
        ty: y - 18,
        cy: y + 24, // below the badge on the line
        anchor: ltr ? 'start' : 'end',
      });
      y += stepH;
    }
    minTop = 0;
  };

  const placeFragment = (frag, depth) => {
    const [op, operandSpecs] = Object.entries(frag)[0];
    const top = Math.max(y - FRAG_ABOVE, minTop);
    const operands = [];
    let opTop = top;
    operandSpecs.forEach((o, i) => {
      if (i > 0) opTop = y - FRAG_BELOW;
      operands.push({ y: opTop, guard: `[${o.guard}]`, tone: o.tone });
      y = opTop + FRAG_HEAD;
      minTop = opTop + HEADER_H + 4;
      placeItems(o.steps, depth + 1);
    });
    const bottom = y - FRAG_BELOW;
    const x = LEFT + FRAG_INSET * (depth + 1);
    frags.push({ op, x, y: top, w: RIGHT - LEFT - 2 * FRAG_INSET * (depth + 1), h: bottom - top, operands, depth });
    y = bottom + FRAG_AFTER;
    minTop = 0;
  };

  const placeItems = (items, depth) => {
    for (const item of items) {
      if (typeof item === 'string') placeStep(item);
      else placeFragment(item, depth);
    }
  };

  for (const ph of spec.phases) {
    const top = y;
    y += PHASE_HEAD;
    minTop = top + HEADER_H + 8;
    placeItems(ph.steps, 0);
    y += PHASE_TAIL - stepH / 2;
    phases.push({ x: LEFT, y: top, w: RIGHT - LEFT, h: y - top, label: ph.label, variant: ph.variant });
    y += PHASE_GAP;
  }
  // Outer frames first, so a nested frame's tag is drawn over its parent.
  frags.sort((a, b) => a.depth - b.depth);
  return { lanes, phases, frags, msgs, viewH: y + 4, lifelineBottom: y - PHASE_GAP };
}

const breakable = (name) => name.replace(/_/g, '_\u200b');
const testsLine = (tests) => (Array.isArray(tests) ? tests.map(breakable).join(', ') : tests);

export default function SequenceDiagram({ id, spec, text }) {
  const { shownId, pinnedId, select, setHover } = useSelection();
  const L = layout(spec);
  const info = shownId ? text.steps[shownId] : null;

  const chips = info
    ? [
        ...(info.reqs || []).map((r) => ({ tone: 'safe', id: r, text: REF_SHORT[r] })),
        ...(info.goals || []).map((g) => ({ tone: 'neutral', id: g, text: REF_SHORT[g] })),
        ...(info.threats || []).map((t) => ({ tone: 'danger', id: t, text: REF_SHORT[t] })),
        ...(info.ips || []).map((p) => ({ tone: 'neutral', id: p, text: REF_SHORT[p] })),
        ...(info.assets || []).map((a) => ({ tone: 'info', id: a, text: REF_SHORT[a] })),
      ]
    : [];

  return (
    <Diagram
      id={id}
      viewH={L.viewH}
      label={text.label}
      footer={
        <DetailPanel
          id={shownId ? `Step ${shownId}` : null}
          title={info?.name}
          desc={info?.desc}
          chips={chips}
          hint={text.hint}
        >
          {info?.tests?.length ? (
            <div class="dg-panel-desc">
              {text.testsLabel || 'Tests'}: {testsLine(info.tests)}
            </div>
          ) : null}
          {info?.code ? (
            <div class="dg-panel-desc">
              {text.codeLabel || 'Code:'} {info.code}
            </div>
          ) : null}
        </DetailPanel>
      }
    >
      {L.phases.map((p) => (
        <Outline key={p.label} {...p} />
      ))}
      {L.frags.map((f) => (
        <Fragment key={`frag-${f.y}-${f.depth}`} {...f} />
      ))}
      {L.lanes.map((l) => (
        <Box key={l.key} {...l.box} label={l.label} sub={l.sub} variant={l.variant} small />
      ))}
      {L.lanes.map((l) => (
        <Edge key={`life-${l.key}`} id={`${id}-lifeline-${l.key}`} points={[[l.c, LANE_TOP + LANE_H], [l.c, L.lifelineBottom]]} dashed />
      ))}
      {L.msgs.map((m) => (
        <Edge key={m.id} id={`${id}-msg-${m.id}`} points={m.points} dir="end" dashed={m.reply} active={shownId === m.id} />
      ))}
      {L.msgs.map((m) => (
        <text key={`t-${m.id}`} x={m.tx} y={m.ty} text-anchor={m.anchor} class="dg-label dg-label-sub">
          {m.text}
        </text>
      ))}
      {L.msgs
        .filter((m) => m.code)
        .map((m) => (
          <text key={`c-${m.id}`} x={m.tx} y={m.cy} text-anchor={m.anchor} class="dg-label dg-label-sub">
            {m.code}
          </text>
        ))}
      {L.msgs.map((m) => (
        <Badge
          key={`b-${m.id}`}
          id={m.id}
          label={m.id}
          name={text.steps[m.id]?.name}
          x={m.badge[0]}
          y={m.badge[1]}
          active={shownId === m.id}
          pressed={pinnedId === m.id}
          onSelect={select}
          onHover={setHover}
        />
      ))}
    </Diagram>
  );
}
