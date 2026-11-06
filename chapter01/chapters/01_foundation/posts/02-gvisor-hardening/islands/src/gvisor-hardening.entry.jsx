// Entry = island name: gvisor-hardening.entry.jsx → islands/dist/gvisor-hardening.js
//   {{island:gvisor-hardening}}         → #island-gvisor-hardening: the runtime boundary
//   <div id="gvisor-capability-runc"></div>   → SYS_ADMIN under runc (shared kernel)
//   <div id="gvisor-capability-gvisor"></div> → SYS_ADMIN under gVisor (Sentry)
// One generic BlockDiagram fed by per-variant data in groups/gvisor-hardening.js
// (variants: runtime | runc | gvisor). Each mount has a distinct `id` prop (SVG
// marker ids are global to the page).
import { variants } from './groups/gvisor-hardening.js';
import BlockDiagram from './BlockDiagram.jsx';
import { mountAll } from './mount.jsx';

const diagram = (id, variant) => () => <BlockDiagram id={id} layout={variants[variant].block} text={variants[variant].blockText} />;

mountAll([
  ['island-gvisor-hardening', diagram('gvisor-runtime-boundary', 'runtime')],
  ['gvisor-capability-runc', diagram('gvisor-cap-runc', 'runc')],
  ['gvisor-capability-gvisor', diagram('gvisor-cap-gvisor', 'gvisor')],
]);
