// Entry = island name: det-arch.entry.jsx → islands/dist/det-arch.js
//   {{island:det-arch}}   → #island-det-arch: the layered architecture of
//                           agent_graph, from the CLI down to the backends
//                           (block diagram)
// One generic component (BlockDiagram) fed by data in groups/det-arch.js. Each
// mount needs a distinct `id` prop (SVG marker ids are global to the page).
import * as g from './groups/det-arch.js';
import BlockDiagram from './BlockDiagram.jsx';
import { mountAll } from './mount.jsx';

mountAll([['island-det-arch', () => <BlockDiagram id="det-arch-block" layout={g.block} text={g.blockText} />]]);
