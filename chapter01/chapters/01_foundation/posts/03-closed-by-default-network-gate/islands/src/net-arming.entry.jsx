// Entry = island name: net-arming.entry.jsx → islands/dist/net-arming.js
//   {{island:net-arming}}   → #island-net-arming: the arming sequence of an
//                             agent container (block diagram, left to right)
// One generic component (BlockDiagram) fed by data in groups/net-arming.js. Each
// mount needs a distinct `id` prop (SVG marker ids are global to the page).
import * as g from './groups/net-arming.js';
import BlockDiagram from './BlockDiagram.jsx';
import { mountAll } from './mount.jsx';

mountAll([['island-net-arming', () => <BlockDiagram id="net-arming-block" layout={g.block} text={g.blockText} />]]);
