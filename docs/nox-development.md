# Developing Nox inside cxz

cxz consumes `@lesomnus/nox` from the Git submodule at `ts/packages/nox`.
The parent repository pins an upstream commit; npm workspaces link that checkout
into `ts/node_modules`. No npm publication or global `npm link` is required.

Initialize a checkout:

```sh
git clone --recurse-submodules https://github.com/lesomnus/cxz.git
# For an existing checkout:
git submodule update --init --recursive
npm ci --prefix ts
```

From `ts`, run the usual `npm run dev`, `npm run sandbox`, or `npm run storybook`.
`npm run dev` also starts the application Storybook on port 6006; Ctrl+C stops
both processes. `npm run dev:app` starts only the application.
Edit `packages/nox/src` alongside the application. Development selects Nox's
`nox-source` conditional exports, including source theme tokens and editor/terminal
entries. Vite watches these source files; the application and sandbox use React
Fast Refresh. There is no library build/watch process between a source edit and
the application. React, React DOM and Monaco resolve to the application's copy.
TypeScript uses the matching condition for source-aware checks.

`npm run build`, `npm run sandbox:build` and release builds compile Nox first and
consume its normal distribution entries. CI fetches the pinned submodule, so
building another machine does not silently follow the latest Nox branch.

## Make and share a Nox change

A submodule is its own repository. After initialization it may be detached;
create a branch before changing its source:

```sh
git -C ts/packages/nox switch -c feat/my-component
# Edit ts/packages/nox/src and view the change in cxz.
npm run --prefix ts nox:check
npm run --prefix ts nox:test
npm run --prefix ts build:nox
git -C ts/packages/nox add src
git -C ts/packages/nox commit -m "Improve component"
git -C ts/packages/nox push -u origin feat/my-component
```

Review that branch in lesomnus/nox. Push its commit before updating cxz's pointer
so other clones and CI can fetch it:

```sh
git add ts/packages/nox
git commit -m "Update Nox dependency"
```

After a Nox merge, move the submodule to the merged commit and commit that pointer
in cxz. `git submodule update --remote ts/packages/nox` explicitly selects upstream
main; ordinary initialization stays on the commit cxz records. Finish or preserve
local Nox edits before moving between commits. `git status` in each repository
shows which project owns a change.

Nox owns component implementations, CSS Modules, tokens and its standalone tests.
cxz owns application layouts, integrations and browser regressions. The `:ui`
Storybook commands remain available as host-side previews of the Nox checkout.
See [Nox's local development guide](https://github.com/lesomnus/nox/blob/main/docs/local-development.md)
for its public source/distribution contract.
