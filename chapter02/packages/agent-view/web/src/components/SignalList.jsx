export function SignalList({ signals, selected, onToggle, onClear, filter, onFilter, onHover }) {
  const f = filter.trim().toLowerCase();
  const shown = f ? signals.filter((s) => s.key.includes(f)) : signals;
  return (
    <aside class="panel signals">
      <header>
        <h2>Signals</h2>
        <span class="sub">{selected.size}/{signals.length}</span>
        <span class="grow" />
        <button type="button" class="link" onClick={onClear} disabled={!selected.size}>clear</button>
      </header>
      <input
        type="search"
        class="filter"
        placeholder="filter signals"
        value={filter}
        onInput={(e) => onFilter(e.currentTarget.value)}
      />
      <ul onMouseLeave={() => onHover(null)}>
        {shown.map((s) => (
          <li key={s.name} onMouseEnter={() => onHover(s.name)}>
            <label>
              <input type="checkbox" checked={selected.has(s.name)} onChange={() => onToggle(s.name)} />
              <span class="swatch" style={{ background: s.color }} />
              <span class="sname">{s.name}</span>
              <span class="stype">{s.type}</span>
            </label>
          </li>
        ))}
        {!shown.length && <li class="none">no matching signals</li>}
      </ul>
    </aside>
  );
}
