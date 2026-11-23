# Foundation

The foundational architecture decisions for an agent framework that runs untrusted, LLM-generated code. The chapter starts with the isolation runtime.

- `posts/01-isolation-runtime/` first post: which isolation boundary I chose for executing agent-generated code, and why. Single post (`type: dev`) with runnable code: `code_package: agent-sandbox`, `code_tag: ch01-ch02/publication`. All six chapter 1 and chapter 2 posts share this tag. It is not cut yet; the commit sha is filled in before publish.
- `islands/build.mjs` bundles every `posts/*/islands/src/*.entry.jsx` into `posts/*/islands/dist/<name>.js`.
- Build the islands: `npm install`, then `npm run build:islands`.
