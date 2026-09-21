// Entry = island name: blocks.entry.jsx → islands/dist/blocks.js →
// {{island:blocks}} in index.md, which the publisher turns into
// <div id="island-blocks"></div> + the inlined bundle.
import { render } from 'preact';
import BlockDiagram from './BlockDiagram.jsx';

const MOUNTS = {
  'island-blocks': { id: 'blocks' },
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
