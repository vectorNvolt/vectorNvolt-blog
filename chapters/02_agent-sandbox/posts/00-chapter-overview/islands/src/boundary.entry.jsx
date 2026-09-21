// Entry = island name: boundary.entry.jsx → islands/dist/boundary.js →
// {{island:boundary}} in index.md, which the publisher turns into
// <div id="island-boundary"></div> + the inlined bundle.
import { render } from 'preact';
import BoundaryDiagram from './BoundaryDiagram.jsx';

const MOUNTS = {
  'island-boundary': { id: 'boundary' },
};

function mountAll() {
  for (const [domId, props] of Object.entries(MOUNTS)) {
    const el = document.getElementById(domId);
    if (el) render(<BoundaryDiagram {...props} />, el);
  }
}

// The publisher may inline this script before the mount points appear in
// the markup, so defer until the document is parsed.
if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', mountAll);
} else {
  mountAll();
}
