import { execStatus } from '../signals.js';

function label(e) {
  const s = execStatus(e.result);
  return `${e.id}  ·  ${s.text}`;
}

export function Header({ execs, selId, onSelect, follow, onFollow }) {
  const cur = execs && execs.find((e) => e.id === selId);
  const st = cur ? execStatus(cur.result) : null;
  return (
    <header class="top">
      <h1>agent-view</h1>
      <select
        aria-label="exec"
        value={selId || ''}
        disabled={!execs || !execs.length}
        onChange={(e) => onSelect(e.currentTarget.value)}
      >
        {(execs || []).map((e) => (
          <option key={e.id} value={e.id}>{label(e)}</option>
        ))}
      </select>
      {st && <span class={'badge ' + st.cls}>{st.text}</span>}
      {cur && cur.mtime && <span class="sub">{String(cur.mtime)}</span>}
      <span class="grow" />
      <label class="follow">
        <input type="checkbox" checked={follow} onChange={(e) => onFollow(e.currentTarget.checked)} />
        follow newest
      </label>
    </header>
  );
}
