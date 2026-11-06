// Mounts several diagrams from a single bundle (Preact is shipped once per
// bundle). `mounts`: [[domId, () => <Diagram …/>]] — the first is normally the
// island's own `island-<name>` div, later ones plain `<div id="…"></div>`
// lines placed elsewhere in the post. A missing mount point is skipped.
import { render } from 'preact';

export function mountAll(mounts) {
  const run = () => {
    for (const [domId, make] of mounts) {
      const el = document.getElementById(domId);
      if (el) render(make(), el);
    }
  };
  // The publisher may inline this script before the mount points appear in
  // the markup, so defer until the document is parsed.
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', run);
  } else {
    run();
  }
}
