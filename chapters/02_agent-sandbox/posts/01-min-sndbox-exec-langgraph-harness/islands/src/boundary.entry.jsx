// Entry = island name: boundary.entry.jsx → islands/dist/boundary.js →
// {{island:boundary}} in index.md. One bundle mounts both runtime variants
// so Preact is shipped once; index.md carries a plain <div> per mount.
import { render } from 'preact';
import SystemBlockDiagram from './SystemBlockDiagram.jsx';

const MOUNTS = {
  'island-boundary': { id: 'boundary-runsc', runtime: 'runsc' },
  'boundary-diagram-runc': { id: 'boundary-runc', runtime: 'runc' },
};

function mountAll() {
  for (const [domId, props] of Object.entries(MOUNTS)) {
    const el = document.getElementById(domId);
    if (el) render(<SystemBlockDiagram {...props} />, el);
  }
}

// The publisher may inline this script before the mount points appear in
// the markup, so defer until the document is parsed.
if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', mountAll);
} else {
  mountAll();
}
