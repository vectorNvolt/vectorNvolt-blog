// Entry = island name: blocks.entry.jsx → islands/dist/blocks.js →
// {{island:blocks}} in index.md, which the publisher turns into
// <div id="island-blocks"></div> + the inlined bundle.
//
// One bundle mounts the drawing twice: the threat overlay at the island
// mount point and the fix overlay at a plain <div id="blocks-fix"> further
// down the post. Distinct `id` props keep the SVG marker ids apart.
import { render } from 'preact';
import BlockDiagram from './BlockDiagram.jsx';

const MOUNTS = {
  'island-blocks': { id: 'blocks-threat', variant: 'threat' },
  'blocks-fix': { id: 'blocks-fix', variant: 'fix' },
};

function mountAll() {
  for (const [domId, props] of Object.entries(MOUNTS)) {
    const el = document.getElementById(domId);
    if (el) render(<BlockDiagram {...props} />, el);
  }
}

// The publisher may inline this script before the mount points appear in
// the markup, so defer until the document is parsed.
if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', mountAll);
} else {
  mountAll();
}
