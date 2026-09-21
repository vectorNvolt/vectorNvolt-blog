# Agent dashboard

Posts live in `posts/`. This chapter builds on the code in `packages/agent_sandbox/`
(see its README) rather than introducing a new package; each post's front matter
still carries the `code_tag` (git tag) and `code_package: agent_sandbox` that pin
the exact code state the post describes.

Posts can embed interactive diagrams ("islands") as small Preact components. Source
lives per post at `posts/<slug>/islands/src/*.entry.jsx`; `npm run build:islands`
bundles each into a self-contained `posts/<slug>/islands/dist/<name>.js` (Preact
included, no external requests) that the publisher inlines into the Ghost post next
to a `<div id="...">` mount point in the post's markdown.

    npm install
    npm run build:islands
