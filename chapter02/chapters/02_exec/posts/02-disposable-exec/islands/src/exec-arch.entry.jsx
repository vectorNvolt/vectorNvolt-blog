// Entry = island name: exec-arch.entry.jsx → islands/dist/exec-arch.js
//   {{island:exec-arch}}         → #island-exec-arch: the architecture of one
//                                  disposable execution (block diagram)
//   <div id="exec-life-seq"></div> → the lifecycle of one execution (sequence
//                                  diagram), placed wherever the post needs it
// One generic component per kind (BlockDiagram / SequenceDiagram) fed by data in
// groups/exec-arch.js. Each mount needs a distinct `id` prop (SVG marker ids
// are global to the page).
import * as g from './groups/exec-arch.js';
import BlockDiagram from './BlockDiagram.jsx';
import SequenceDiagram from './SequenceDiagram.jsx';
import { mountAll } from './mount.jsx';

mountAll([
  ['island-exec-arch', () => <BlockDiagram id="exec-arch-block" layout={g.block} text={g.blockText} />],
  ['exec-life-seq', () => <SequenceDiagram id="exec-life-seq-svg" spec={g.seq} text={g.seqText} />],
]);
