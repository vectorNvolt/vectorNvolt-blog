import { render } from 'preact';
import { useCallback, useEffect, useMemo, useRef, useState } from 'preact/hooks';
import 'uplot/dist/uPlot.min.css';
import './style.css';
import { getExecs, loadExec, fileUrl } from './api.js';
import { defaultSelection, fmtSI, makeSignals, signalMap, xUnit } from './signals.js';
import { Header } from './components/Header.jsx';
import { SignalList } from './components/SignalList.jsx';
import { Waveform } from './components/Waveform.jsx';
import { Schematic } from './components/Schematic.jsx';

const sigOf = (e) => (e ? [e.id, e.mtime, JSON.stringify(e.result), (e.outputs || []).join(',')].join('|') : null);

function NoRaw({ exec, detail }) {
  const r = exec.result;
  return (
    <section class="panel noraw">
      <header><h2>No waveform</h2></header>
      <div class="panel-body scroll">
        {detail.rawError ? (
          <p class="err">Could not read sim.raw: {detail.rawError}</p>
        ) : (
          <p>This exec produced no <code>sim.raw</code>.</p>
        )}
        <p>
          Result: {r ? <code>{JSON.stringify(r)}</code> : 'none yet (still running?)'}
          {r && r.cause ? <> &mdash; cause: <strong>{r.cause}</strong></> : null}
        </p>
        <p>
          stderr (<a href={fileUrl(exec.id, 'stderr.log')} target="_blank" rel="noreferrer">stderr.log</a>):
        </p>
        <pre>{detail.stderr == null ? '(no stderr.log available)' : detail.stderr || '(empty)'}</pre>
      </div>
    </section>
  );
}

function App() {
  const [execs, setExecs] = useState(null);
  const [listErr, setListErr] = useState(null);
  const [selId, setSelId] = useState(null);
  const [follow, setFollow] = useState(true);
  const [detail, setDetail] = useState(null);
  const [selected, setSelected] = useState(() => new Set());
  const [filter, setFilter] = useState('');
  const [hoverSig, setHoverSig] = useState(null);
  const [cursorT, setCursorT] = useState(null);
  const lastLoaded = useRef(null);

  const refresh = useCallback(() => {
    getExecs().then((l) => { setExecs(l); setListErr(null); }).catch((e) => setListErr(e.message));
  }, []);

  useEffect(() => {
    refresh();
    const es = new EventSource('/api/events');
    es.addEventListener('execs', refresh);
    return () => es.close();
  }, [refresh]);

  // auto-select: first load, vanished selection, or follow-newest
  useEffect(() => {
    if (!execs || !execs.length) return;
    const newest = execs[0].id;
    if (selId == null || !execs.some((e) => e.id === selId) || (follow && newest !== selId)) setSelId(newest);
  }, [execs, follow, selId]);

  const cur = execs ? execs.find((e) => e.id === selId) : null;
  const curSig = sigOf(cur);

  useEffect(() => {
    if (!cur) return undefined;
    let dead = false;
    loadExec(cur).then((d) => {
      if (dead) return;
      const same = lastLoaded.current === cur.id;
      lastLoaded.current = cur.id;
      setDetail({ id: cur.id, ...d });
      if (d.raw) {
        const sigs = makeSignals(d.raw);
        const names = new Set(sigs.map((s) => s.name));
        setSelected((prev) => (same ? new Set([...prev].filter((n) => names.has(n))) : new Set(defaultSelection(sigs))));
      } else {
        setSelected(new Set());
      }
      setCursorT(null);
    });
    return () => { dead = true; };
  }, [curSig]);

  const ready = detail && cur && detail.id === cur.id ? detail : null;
  const signals = useMemo(() => (ready && ready.raw ? makeSignals(ready.raw) : []), [ready && ready.raw]);
  const sigMap = useMemo(() => signalMap(signals), [signals]);

  const toggle = useCallback((name) => {
    setSelected((prev) => {
      const n = new Set(prev);
      if (n.has(name)) n.delete(name);
      else n.add(name);
      return n;
    });
  }, []);

  const onSelect = (id) => {
    setSelId(id);
    setFollow(!!execs && execs[0].id === id);
  };

  let tText = null;
  if (ready && ready.raw && ready.raw.data[0].length) {
    const xs = ready.raw.data[0];
    const t = cursorT == null ? xs[xs.length - 1] : cursorT;
    tText = `${ready.raw.vars[0].name} = ${fmtSI(t, xUnit(ready.raw.vars[0].type))}${cursorT == null ? ' (end)' : ''}`;
  }

  let main;
  if (listErr && !execs) main = <div class="msg err">Cannot reach the server: {listErr}</div>;
  else if (!execs) main = <div class="msg">Loading...</div>;
  else if (!execs.length) main = <div class="msg">No exec results in .output/exec yet.</div>;
  else if (!ready) main = <div class="msg">Loading exec {selId}...</div>;
  else {
    main = (
      <div class={'body' + (ready.raw ? '' : ' noraw-layout')}>
        {ready.raw && (
          <SignalList
            signals={signals}
            selected={selected}
            onToggle={toggle}
            onClear={() => setSelected(new Set())}
            filter={filter}
            onFilter={setFilter}
            onHover={setHoverSig}
          />
        )}
        <div class="panels">
          <Schematic
            netlist={ready.netlist}
            netlistError={ready.netlistError}
            raw={ready.raw}
            sigMap={sigMap}
            selected={selected}
            hoverSig={hoverSig}
            toggleSignal={toggle}
            probeT={cursorT}
            tText={tText}
          />
          {ready.raw ? (
            <Waveform raw={ready.raw} signals={signals} selected={selected} onCursor={setCursorT} />
          ) : (
            <NoRaw exec={cur} detail={ready} />
          )}
        </div>
      </div>
    );
  }

  return (
    <div class="app">
      <Header execs={execs} selId={selId} onSelect={onSelect} follow={follow} onFollow={setFollow} />
      {listErr && execs && <div class="msg err slim">Refresh failed: {listErr}</div>}
      {main}
    </div>
  );
}

render(<App />, document.getElementById('app'));
