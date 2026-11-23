// Same-origin calls to the agent-view server.
import { parseRaw } from './rawfile.js';
import { parseNetlist } from './netlist.js';
import { pickNetlistInput } from './signals.js';

export const fileUrl = (id, path) => `/api/execs/${encodeURIComponent(id)}/file?path=${encodeURIComponent(path)}`;

async function ok(url) {
  const r = await fetch(url, { cache: 'no-store' });
  if (!r.ok) throw new Error(`${url}: HTTP ${r.status}`);
  return r;
}

export async function getExecs() {
  const j = await (await ok('/api/execs')).json();
  return j.execs || [];
}

/**
 * Load everything shown for one exec. Never throws: problems end up in the *Error fields.
 * @returns {{raw, rawError, netlist, netlistError, stderr}}
 */
export async function loadExec(exec) {
  const d = { raw: null, rawError: null, netlist: null, netlistError: null, stderr: null };
  const outs = exec.outputs || [];
  const jobs = [];
  if (outs.includes('sim.raw')) {
    jobs.push(
      ok(fileUrl(exec.id, 'out/sim.raw'))
        .then((r) => r.arrayBuffer())
        .then((b) => { d.raw = parseRaw(b); })
        .catch((e) => { d.rawError = e.message; }),
    );
  }
  const name = pickNetlistInput(exec.inputs);
  if (name) {
    jobs.push(
      ok(fileUrl(exec.id, 'in/' + name))
        .then((r) => r.text())
        .then((t) => { d.netlist = parseNetlist(t); })
        .catch((e) => { d.netlistError = e.message; }),
    );
  } else {
    d.netlistError = 'exec has no input netlist';
  }
  await Promise.all(jobs);
  if (!d.raw) {
    try {
      d.stderr = await (await ok(fileUrl(exec.id, 'stderr.log'))).text();
    } catch (e) {
      d.stderr = null;
    }
  }
  return d;
}
