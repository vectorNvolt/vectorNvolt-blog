// Entry = island name: isolation.entry.jsx → islands/dist/isolation.js
//   {{island:isolation}}                → alternative 1, bare Linux process
//   <div id="iso-runc"></div>           → alternative 2, Docker / runc
//   <div id="iso-wasm"></div>           → alternative 3, WebAssembly / WASI runtime
//   <div id="iso-gvisor"></div>         → alternative 4, gVisor / runsc
//   <div id="iso-microvm"></div>        → alternative 5, microVM (Firecracker)
//   <div id="iso-kata"></div>           → alternative 6, Kata Containers
//   <div id="iso-vm"></div>             → alternative 7, standard VM (QEMU/KVM)
// One generic component (BlockDiagram) fed by per-alternative data in
// groups/isolation.js. Each mount needs a distinct `id` prop (SVG marker ids
// are global to the page). The plain <div> lines sit in the post's sections
// (in Ghost HTML cards); a missing mount point is skipped.
import * as g from './groups/isolation.js';
import BlockDiagram from './BlockDiagram.jsx';
import { mountAll } from './mount.jsx';

mountAll([
  ['island-isolation', () => <BlockDiagram id="iso-bare-block" layout={g.bareBlock} text={g.bareText} />],
  ['iso-runc', () => <BlockDiagram id="iso-runc-block" layout={g.runcBlock} text={g.runcText} />],
  ['iso-wasm', () => <BlockDiagram id="iso-wasm-block" layout={g.wasmBlock} text={g.wasmText} />],
  ['iso-gvisor', () => <BlockDiagram id="iso-gvisor-block" layout={g.gvisorBlock} text={g.gvisorText} />],
  ['iso-microvm', () => <BlockDiagram id="iso-microvm-block" layout={g.microvmBlock} text={g.microvmText} />],
  ['iso-kata', () => <BlockDiagram id="iso-kata-block" layout={g.kataBlock} text={g.kataText} />],
  ['iso-vm', () => <BlockDiagram id="iso-vm-block" layout={g.vmBlock} text={g.vmText} />],
]);
