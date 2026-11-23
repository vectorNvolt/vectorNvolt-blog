// Bundles src/main.jsx into ../internal/ui/static/app.js (+ app.css, emitted by esbuild
// next to the JS because main.jsx imports the CSS). Everything is bundled: no CDN.
import { build } from 'esbuild';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

const here = dirname(fileURLToPath(import.meta.url));

// elkjs/lib/elk.bundled.js may mention the optional 'web-worker' package; we never use a worker.
const stubWebWorker = {
  name: 'stub-web-worker',
  setup(b) {
    b.onResolve({ filter: /^web-worker$/ }, () => ({ path: 'web-worker', namespace: 'stub' }));
    b.onLoad({ filter: /.*/, namespace: 'stub' }, () => ({ contents: 'module.exports = undefined;', loader: 'js' }));
  },
};

await build({
  absWorkingDir: here,
  entryPoints: ['src/main.jsx'],
  outfile: join(here, '../internal/ui/static/app.js'),
  bundle: true,
  format: 'iife',
  minify: true,
  target: 'es2020',
  jsx: 'automatic',
  jsxImportSource: 'preact',
  define: { 'process.env.NODE_ENV': '"production"' },
  plugins: [stubWebWorker],
  logLevel: 'info',
});
