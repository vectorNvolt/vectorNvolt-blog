// Entry = island name: ngspice.entry.jsx → islands/dist/ngspice.js
//   {{island:ngspice}} → #island-ngspice: who decides what around one ngspice
//                        run (block diagram)
//                      → #ngspice-loop: one pass through the boost-design
//                        process (sequence diagram)
// Generic components fed by data in groups/ngspice.js. Each mount needs a
// distinct `id` prop (SVG marker ids are global to the page).
import * as g from './groups/ngspice.js';
import BlockDiagram from './BlockDiagram.jsx';
import SequenceDiagram from './SequenceDiagram.jsx';
import { mountAll } from './mount.jsx';

mountAll([
  ['island-ngspice', () => <BlockDiagram id="ngspice-block" layout={g.block} text={g.blockText} />],
  ['ngspice-loop', () => <SequenceDiagram id="ngspice-seq" spec={g.seq} text={g.seqText} />],
]);
