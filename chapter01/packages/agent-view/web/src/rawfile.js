// ngspice rawfile reader (port of howto/manual_test/agent-exec/src/ngspice/readraw.py).
// Binary (little-endian float64, point-major) and ASCII real rawfiles; first plot only.

const latin1 = new TextDecoder('latin1');

function indexOfBytes(u8, str, from = 0, to = u8.length) {
  const pat = Array.from(str, (c) => c.charCodeAt(0));
  const last = to - pat.length;
  outer: for (let i = from; i <= last; i++) {
    for (let j = 0; j < pat.length; j++) if (u8[i + j] !== pat[j]) continue outer;
    return i;
  }
  return -1;
}

/**
 * @param {ArrayBuffer|Uint8Array} buf
 * @returns {{title:string, plotname:string, flags:string, vars:{index:number,name:string,type:string}[], data:Float64Array[]}}
 */
export function parseRaw(buf) {
  const u8 = buf instanceof Uint8Array ? buf : new Uint8Array(buf);
  const bpos = indexOfBytes(u8, 'Binary:\n');
  const vpos = indexOfBytes(u8, 'Values:\n', 0, bpos >= 0 ? bpos : u8.length);
  if (bpos < 0 && vpos < 0) throw new Error("rawfile: no 'Binary:' or 'Values:' section found");
  const binary = vpos < 0;
  const pos = binary ? bpos : vpos;
  const markerLen = 8;

  const head = {};
  const vars = [];
  let inVars = false;
  for (const ln of latin1.decode(u8.subarray(0, pos)).split(/\r?\n/)) {
    if (inVars && (ln[0] === '\t' || ln[0] === ' ')) {
      const f = ln.trim().split(/\s+/);
      if (f.length < 3) throw new Error('rawfile: bad variable line: ' + JSON.stringify(ln));
      vars.push({ index: Number(f[0]), name: f[1], type: f[2] });
    } else if (ln.includes(':')) {
      const k = ln.slice(0, ln.indexOf(':')).trim();
      head[k] = ln.slice(ln.indexOf(':') + 1).trim();
      inVars = k === 'Variables';
    }
  }
  const nvars = parseInt(head['No. Variables'], 10);
  const npts = parseInt(head['No. Points'], 10);
  if (!Number.isFinite(nvars) || !Number.isFinite(npts)) {
    throw new Error("rawfile: missing or bad 'No. Variables' / 'No. Points'");
  }
  const flags = head['Flags'] || '';
  if (flags.split(/\s+/).includes('complex')) {
    throw new Error('rawfile: complex data (e.g. AC analysis) is not supported');
  }
  if (vars.length !== nvars) {
    throw new Error(`rawfile: variable list has ${vars.length} entries, expected ${nvars}`);
  }

  const data = vars.map(() => new Float64Array(npts));
  const start = pos + markerLen;
  if (binary) {
    const need = nvars * npts * 8;
    if (u8.length - start < need) {
      throw new Error(`rawfile: truncated file: ${u8.length - start} bytes of data, need ${need}`);
    }
    const dv = new DataView(u8.buffer, u8.byteOffset + start, need);
    let off = 0;
    for (let i = 0; i < npts; i++) {
      for (let j = 0; j < nvars; j++, off += 8) data[j][i] = dv.getFloat64(off, true);
    }
  } else {
    let text = latin1.decode(u8.subarray(start));
    const cut = text.indexOf('Title:');
    if (cut >= 0) text = text.slice(0, cut);
    const toks = text.split(/\s+/).filter(Boolean);
    if (toks.length < npts * (nvars + 1)) {
      throw new Error(`rawfile: truncated file: ${toks.length} values, need ${npts * (nvars + 1)}`);
    }
    for (let i = 0; i < npts; i++) {
      for (let j = 0; j < nvars; j++) {
        const x = Number(toks[i * (nvars + 1) + 1 + j]);
        if (Number.isNaN(x)) throw new Error('rawfile: bad numeric value in ASCII Values section');
        data[j][i] = x;
      }
    }
  }
  return { title: head['Title'] || '', plotname: head['Plotname'] || '', flags, vars, data };
}
