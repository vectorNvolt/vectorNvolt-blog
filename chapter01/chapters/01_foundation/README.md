# Foundation

The foundational architecture decisions for an agent framework that runs untrusted, LLM-generated code. The chapter starts with the isolation runtime.

- `posts/01-isolation-runtime/` first post: which isolation boundary I chose for executing agent-generated code, and why. Single post (`type: dev`) with runnable code: `code_package: agent-sandbox`, `code_tag: ch01_found/p01_p02_p03`. The `agent-sandbox` package is published under that git tag on GitHub; the repository URL and commit sha are filled in before publish.
- `islands/build.mjs` bundles every `posts/*/islands/src/*.entry.jsx` into `posts/*/islands/dist/<name>.js`.
- Build the islands: `npm install`, then `npm run build:islands`.
