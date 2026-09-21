// Bundles every posts/*/islands/src/*.entry.jsx into a self-contained
// posts/*/islands/dist/<name>.js (Preact included, no external requests) —
// the publisher inlines that file straight into the Ghost post as a
// <script> tag, next to a `<div id="...">` mount point in the post's markdown.
import { build } from 'esbuild';
import { existsSync, readdirSync, statSync } from 'node:fs';
import { basename, dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const islandsRoot = dirname(fileURLToPath(import.meta.url));
const postsDir = join(islandsRoot, '..', 'posts');

async function main() {
  const posts = readdirSync(postsDir).filter((name) => statSync(join(postsDir, name)).isDirectory());
  let builtAny = false;

  for (const post of posts) {
    const srcDir = join(postsDir, post, 'islands', 'src');
    if (!existsSync(srcDir)) continue;

    const entries = readdirSync(srcDir).filter((f) => f.endsWith('.entry.jsx'));
    if (entries.length === 0) continue;

    const outDir = join(postsDir, post, 'islands', 'dist');
    for (const entry of entries) {
      const outfile = join(outDir, `${basename(entry, '.entry.jsx')}.js`);
      await build({
        entryPoints: [join(srcDir, entry)],
        outfile,
        bundle: true,
        minify: true,
        format: 'iife',
        target: ['es2019'],
        jsx: 'automatic',
        jsxImportSource: 'preact',
        define: { 'process.env.NODE_ENV': '"production"' },
        logLevel: 'info',
      });
      builtAny = true;
      console.log(`built ${outfile}`);
    }
  }

  if (!builtAny) {
    console.log('no island entries found (looked for posts/*/islands/src/*.entry.jsx)');
  }
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
