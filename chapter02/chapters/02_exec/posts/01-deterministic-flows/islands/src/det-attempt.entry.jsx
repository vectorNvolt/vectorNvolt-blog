// Entry = island name: det-attempt.entry.jsx → islands/dist/det-attempt.js
//   {{island:det-attempt}} → #island-det-attempt: one attempt of a cognitive
//                            step, from START to the next node, with the
//                            failure branch (sequence diagram)
// One generic component (SequenceDiagram) fed by data in groups/det-attempt.js.
// Each mount needs a distinct `id` prop (SVG marker ids are global to the page).
import * as g from './groups/det-attempt.js';
import SequenceDiagram from './SequenceDiagram.jsx';
import { mountAll } from './mount.jsx';

mountAll([['island-det-attempt', () => <SequenceDiagram id="det-attempt-seq" spec={g.seq} text={g.seqText} />]]);
