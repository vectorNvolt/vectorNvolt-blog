// Entry = island name: flow.entry.jsx → islands/dist/flow.js →
// {{island:flow}} in index.md, which the publisher turns into
// <div id="island-flow"></div> + the inlined bundle.
//
// One bundle mounts the sequence twice: the attack at the island mount point
// and the fixed turn at a plain <div id="flow-fix"> further down the post.
import { render } from 'preact';
import InteractionDiagram from './InteractionDiagram.jsx';

const MOUNTS = {
  'island-flow': { id: 'flow-attack', variant: 'attack' },
  'flow-fix': { id: 'flow-fix', variant: 'fix' },
};

function mountAll() {
  for (const [domId, props] of Object.entries(MOUNTS)) {
    const el = document.getElementById(domId);
    if (el) render(<InteractionDiagram {...props} />, el);
  }
}

// The publisher may inline this script before the mount points appear in
// the markup, so defer until the document is parsed.
if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', mountAll);
} else {
  mountAll();
}
