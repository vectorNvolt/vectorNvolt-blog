# Agent sandbox

Posts live in `posts/`. The code they describe lives outside this chapter, in
`packages/agent_sandbox/` (see its README); each post's front matter carries the
`code_tag` (git tag) and `code_package` (which `packages/<name>`) that pin the
exact code state.

Posts can embed interactive diagrams ("islands") as small Preact components. Source
lives per post at `posts/<slug>/islands/src/*.entry.jsx`; `npm run build:islands`
bundles each into a self-contained `posts/<slug>/islands/dist/<name>.js` (Preact
included, no external requests) that the publisher inlines into the Ghost post next
to a `<div id="...">` mount point in the post's markdown.

    npm install
    npm run build:islands
