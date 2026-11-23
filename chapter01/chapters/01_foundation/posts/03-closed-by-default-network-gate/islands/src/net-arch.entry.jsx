// Entry = island name: net-arch.entry.jsx → islands/dist/net-arch.js
//   {{island:net-arch}}   → #island-net-arch: the isolation architecture of an
//                           agent container's network path (block diagram)
// One generic component (BlockDiagram) fed by data in groups/net-arch.js. Each
// mount needs a distinct `id` prop (SVG marker ids are global to the page).
import * as g from './groups/net-arch.js';
import BlockDiagram from './BlockDiagram.jsx';
import { mountAll } from './mount.jsx';

mountAll([['island-net-arch', () => <BlockDiagram id="net-arch-block" layout={g.block} text={g.blockText} />]]);
