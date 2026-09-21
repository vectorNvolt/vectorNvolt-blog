// Entry = island name: graph.entry.jsx → islands/dist/graph.js →
// {{island:graph}} in index.md, which the publisher turns into
// <div id="island-graph"></div> + the inlined bundle.
import { render } from 'preact';
import StateMachineDiagram from './StateMachineDiagram.jsx';

const MOUNTS = {
  'island-graph': { id: 'graph' },
};

function mountAll() {
  for (const [domId, props] of Object.entries(MOUNTS)) {
    const el = document.getElementById(domId);
    if (el) render(<StateMachineDiagram {...props} />, el);
  }
}

// The publisher may inline this script before the mount points appear in
// the markup, so defer until the document is parsed.
if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', mountAll);
} else {
  mountAll();
}
