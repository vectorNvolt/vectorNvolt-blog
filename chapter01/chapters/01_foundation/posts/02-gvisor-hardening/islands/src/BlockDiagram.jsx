// Block diagram of one capability group: the components of the item that
// the group's requirements touch, the interfaces between them, and three
// kinds of hotspot on top —
//   circle  = interaction point (IP-n), on the interface it crosses
//   diamond = asset (AS-n), on the component that holds it
//   square  = threat case (Tnx), where its attack path lands
// Selecting any hotspot highlights the ones it is connected to (`related`)
// and lists them in the panel together with the capability goals and the
// requirements that hold there.
//
// Optional: an item may carry `code` — the files and functions that implement
// it — shown as a second panel line (existing .dg-panel-desc class).
// A link with a `flow` id is itself a hotspot (clickable, text in
// `text.items[flow]`); `blocked: true` draws it as a dropped path.
//
// Optional status overlay — what a later post did to the group (all keys
// optional; without `text.status` the diagram is drawn exactly as without it):
//   text.status        { id → key | { status: key, note? } }; id is a
//                      requirement or a hotspot (IP / asset / threat / flow)
//   text.statusLabel   { key → label }: box sub-line, legend, chip suffix
//   text.statusChip    { key → label } for the chips only (default statusLabel)
//   text.statusTone    { key → safe | info | danger | neutral } (default neutral)
//   text.statusLegend  legend title;  text.statusChipId  the id on a hotspot's
//                      own status chip (default 'Status')
//   layout.statusBoxes [{ id, x, y, w, h, small? }]: one Box per requirement,
//                      sub = its status label, variant from its tone
//   layout.statusHalo  half-size of a filled Outline behind every hotspot
//                      badge that has a status (tone = its status's tone)
//   layout.statusLegend { x, y, titleW, step, swatch, order? }: a legend row
// A requirement chip then reads `<short> · <chip label>` and its `note` is a
// panel line; a selected hotspot with a status gets a status chip first and
// its `note` as a panel line. Existing variants only: tone → Outline variant,
// and `solid-safe` / `solid-info` / `solid-danger` / `default` for a Box.
//
// The component is generic: geometry comes from `layout` (groups/<grp>.js,
// hand laid out in viewBox units, orthogonal connectors only) and every word
// the panel shows from `text` (same file, kept apart from the geometry).
import { Diagram, Outline, Box, Edge, Badge, DetailPanel, useSelection } from './diagram/primitives.jsx';
import { REF_SHORT } from './data.js';

const SHAPE = { ip: 'circle', asset: 'diamond', threat: 'square' };
const TONE = { ip: 'neutral', asset: 'info', threat: 'danger' };
const BOX_VARIANT = { safe: 'solid-safe', info: 'solid-info', danger: 'solid-danger', neutral: 'default' };

function kindOf(id) {
  if (id.startsWith('IP-')) return 'ip';
  if (id.startsWith('AS-')) return 'asset';
  return 'threat';
}

// Relations are declared once per item (`links`) and read in both directions.
function relatedOf(items, id) {
  const out = new Set(items[id]?.links || []);
  for (const [other, item] of Object.entries(items)) {
    if ((item.links || []).includes(id)) out.add(other);
  }
  out.delete(id);
  return [...out];
}

// Status overlay lookups (see the header); all empty when `text.status` is absent.
function statusOf(text) {
  const map = text.status || {};
  const entry = (id) => {
    const v = map[id];
    return v == null ? null : typeof v === 'string' ? { status: v } : v;
  };
  const key = (id) => entry(id)?.status;
  const tone = (k) => (text.statusTone || {})[k] || 'neutral';
  const label = (k) => (text.statusLabel || {})[k];
  const chip = (k) => (text.statusChip || {})[k] || label(k);
  return { entry, key, tone, label, chip };
}

function StatusLegend({ geom, title, st, keys }) {
  const order = geom.order || keys;
  return (
    <>
      {title ? (
        <text x={geom.x} y={geom.y + 16} class="dg-label dg-label-sub">
          {title}
        </text>
      ) : null}
      {order.map((k, i) => {
        const x = geom.x + (geom.titleW || 0) + i * geom.step;
        return (
          <g key={`legend-${k}`}>
            <Outline x={x} y={geom.y} w={geom.swatch} h={geom.swatch} variant={st.tone(k)} filled />
            <text x={x + geom.swatch + 8} y={geom.y + 16} class="dg-label dg-label-sub">
              {st.label(k)}
            </text>
          </g>
        );
      })}
    </>
  );
}

export default function BlockDiagram({ id, layout, text }) {
  const { shownId, pinnedId, select, setHover } = useSelection();
  const info = shownId ? text.items[shownId] : null;
  const related = shownId ? relatedOf(text.items, shownId) : [];
  const st = statusOf(text);
  const reqText = (r) => {
    const k = st.key(r);
    return k ? `${REF_SHORT[r]} · ${st.chip(k)}` : REF_SHORT[r];
  };
  const own = shownId ? st.entry(shownId) : null;
  const notes = info ? (info.reqs || []).filter((r) => st.entry(r)?.note) : [];

  const order = ['ip', 'asset', 'threat'];
  const chips = info
    ? [
        ...(own ? [{ tone: st.tone(own.status), id: text.statusChipId || 'Status', text: st.chip(own.status) }] : []),
        ...related
          .slice()
          .sort((a, b) => order.indexOf(kindOf(a)) - order.indexOf(kindOf(b)))
          .map((r) => ({ tone: TONE[kindOf(r)], id: r, text: REF_SHORT[r] || text.items[r]?.name })),
        ...(info.goals || []).map((g) => ({ tone: 'neutral', id: g, text: REF_SHORT[g] })),
        ...(info.reqs || []).map((r) => ({ tone: 'safe', id: r, text: reqText(r) })),
      ]
    : [];

  const hotspots = [
    ...layout.ips.map((h) => ({ ...h, kind: 'ip' })),
    ...layout.assets.map((h) => ({ ...h, kind: 'asset' })),
    ...layout.threats.map((h) => ({ ...h, kind: 'threat' })),
  ];

  return (
    <Diagram
      id={id}
      viewH={layout.viewH}
      viewW={layout.viewW}
      label={text.label}
      footer={
        <DetailPanel
          id={shownId}
          title={info?.name}
          desc={info?.desc}
          chips={chips}
          hint={text.hint || "Circles are interaction points, diamonds assets, squares threat cases. Hover or click one to highlight what it connects to; the panel lists them with the capability goals (grey) and requirements (green) that hold there."}
        >
          {info?.code ? <div class="dg-panel-desc">Code: {info.code}</div> : null}
          {own?.note ? <div class="dg-panel-desc">{own.note}</div> : null}
          {notes.map((r) => (
            <div key={`note-${r}`} class="dg-panel-desc">
              <strong>{r}</strong> — {st.entry(r).note}
            </div>
          ))}
        </DetailPanel>
      }
    >
      {layout.outlines.map((o) => (
        <Outline key={o.label} {...o} />
      ))}
      {Object.entries(layout.nodes).map(([k, n]) => (
        <Box key={k} {...n} />
      ))}
      {(layout.statusBoxes || []).map((b) => (
        <Box
          key={`status-${b.id}`}
          x={b.x}
          y={b.y}
          w={b.w}
          h={b.h}
          small={b.small}
          label={b.id}
          sub={st.label(st.key(b.id))}
          variant={BOX_VARIANT[st.tone(st.key(b.id))]}
        />
      ))}
      {layout.links.map((e, i) => (
        <Edge
          key={`l${i}`}
          id={e.flow || `${id}-link-${i}`}
          points={e.points}
          dir={e.dir}
          dashed={e.dashed}
          blocked={e.blocked}
          active={e.flow ? shownId === e.flow : undefined}
          onSelect={e.flow ? select : undefined}
          onHover={e.flow ? setHover : undefined}
        />
      ))}
      {layout.ips.map((e) => (
        <Edge
          key={e.id}
          id={`${id}-${e.id}`}
          points={e.points}
          dir={e.dir}
          dashed={e.dashed}
          active={shownId === e.id || related.includes(e.id)}
        />
      ))}
      {layout.statusHalo
        ? hotspots
            .filter((h) => st.key(h.id))
            .map((h) => (
              <Outline
                key={`halo-${h.id}`}
                x={h.at[0] - layout.statusHalo}
                y={h.at[1] - layout.statusHalo}
                w={2 * layout.statusHalo}
                h={2 * layout.statusHalo}
                variant={st.tone(st.key(h.id))}
                filled
              />
            ))
        : null}
      {layout.statusLegend ? (
        <StatusLegend geom={layout.statusLegend} title={text.statusLegend} st={st} keys={Object.keys(text.statusLabel || {})} />
      ) : null}
      {hotspots.map((h) => (
        <Badge
          key={h.id}
          id={h.id}
          label={h.label}
          name={text.items[h.id]?.name}
          shape={SHAPE[h.kind]}
          x={h.at[0]}
          y={h.at[1]}
          active={shownId === h.id}
          related={related.includes(h.id)}
          pressed={pinnedId === h.id}
          onSelect={select}
          onHover={setHover}
        />
      ))}
    </Diagram>
  );
}
