# Web source organization

The web application separates application wiring, pages, feature implementation and shared application infrastructure. The reusable UI package remains independent of all four. `ts/packages/nox` is a Git submodule of `lesomnus/nox`, linked by npm workspaces; see [Nox development](nox-development.md) for source editing and pinned builds.

```text
ts/
├─ packages/nox/src/
│  ├─ components/<component>/
│  ├─ editor/
│  ├─ providers/
│  ├─ theme/
│  └─ stories/integration/
└─ src/
   ├─ main.tsx
   ├─ app/
   │  ├─ router.tsx
   │  ├─ workspace-shell.tsx
   │  ├─ workspace-view.tsx
   │  ├─ sandbox/
   │  └─ styles/
   ├─ pages/
   │  ├─ sessions/
   │  ├─ projects/
   │  ├─ settings/
   │  └─ not-found/
   ├─ features/
   │  ├─ session/
   │  │  ├─ components/
   │  │  ├─ composer/
   │  │  ├─ conversation/
   │  │  ├─ questions/
   │  │  ├─ cards/
   │  │  └─ model/
   │  ├─ settings/components/
   │  └─ workspace/
   │     ├─ editor/
   │     └─ terminal/
   ├─ shared/
   │  ├─ api/
   │  ├─ components/
   │  ├─ content/
   │  ├─ editor/
   │  ├─ i18n/
   │  ├─ navigation/
   │  ├─ scroll/
   │  ├─ settings/
   │  ├─ theme/
   │  ├─ assets/
   │  └─ lib/
   └─ storybook/
```

`app` owns authentication, route registration, providers and the workspace shell. `workspace-view.tsx` chooses the appropriate page. The sandbox has its own entry and boot helpers while using the same shell and pages as production.

`pages` owns screen composition. The sessions page combines the selected conversation and optional workspace editor; the projects page lists projects. Existing settings screen behavior stays together during this structural change. Further decomposition of its controls can happen independently without changing the routing contract.

`features` keeps domain UI and processing close together. Session composition, transcript rendering, questions, card hosting and history projection are grouped by responsibility. Workspace editor and terminal behavior form separate feature areas. Features receive application context values as props rather than importing the workspace shell or its context.

`shared` contains application-wide connection/inventory, settings, translations, fonts, navigation primitives and other utilities. It is still cxz application code: shared API connection state uses feature-owned draft/cache type contracts. It is not the package boundary for reuse by another application. Keep reusable controls in `@lesomnus/nox`, and keep session-specific card hosting in the session feature.

`Connection.selection` owns the current project and session scope shared by Tasks,
Memory and workspace navigation. It stores stable project/session IDs plus the
session's runtime identifier used by Task assignees. Changing project clears the
session; selecting All sessions clears it across pages. Explicit Task link filters
override the remembered scope, while a run filter remains specific to the Task
URL. Authenticated connections retain the selection in `sessionStorage`, keyed
by server origin and reader identity; tabs remain independent, and sign-out clears
the selection. This is navigation state rather than a `settings.json` preference.

Modal boundaries use Nox `Dialog`, including account/session/task forms. The cxz
`AnchoredDialog` adapter owns only positioning and trigger decoration; it uses
`frame={false}` when its caller already supplies a Card. Conditionally mounted
forms use `useDialogClose` to delay cancellation cleanup or successful navigation
until `onClosed`. Cancel ongoing work immediately (for example, disconnect a
login socket); only visual teardown waits for the exit animation.

In the UI package, each component folder contains its implementation, CSS Module and independent Storybook examples. The optional editor has its own entry. Theme tokens and the translation provider have dedicated directories. Consumers import the public package entries; internal file paths are not an API. Application code uses those public entries directly rather than maintaining one-file re-export wrappers.

Tests stay beside the behavior they verify. Application stories stay beside their components; package-control stories demonstrating cxz validation or action policy live under `src/storybook/components`. Standalone Nox stories live only in the Nox submodule and are registered by `.storybook-ui`; the cxz `.storybook` catalog discovers only `src` stories and does not duplicate library examples. Package browser checks use `ui-e2e`, while product integration checks use `storybook-e2e`. Shared preview framing and synthetic session data stay under `src/storybook` and are not used by application entries. Both Storybooks discover stories recursively, so directory moves do not require individual registration.

Global application layout CSS lives under `src/app/styles`; component CSS lives in its UI component folder. Shared native scrollbar styling remains under `src/shared/scroll`. CSS tokens and behavior implementations are the source of truth for design values. Generated protocol files remain under `ts/gen`, and generated UI distribution and Storybook outputs remain excluded from Git.

Accounts, Projects, Tasks and Memory share `resource-page.module.css` for their centered
toolbar/body frames and responsive alignment. Their column width, gap, gutter,
picker width and toolbar row height come from the resource layout tokens in
`app/styles/style.css`; pages should not maintain separate numeric copies.

Accounts and Projects compose `ResourceLayout`, `ResourceDashboard`,
`ResourceDashboardColumn` and `ResourceSessionSection` from
`shared/components/resource-layout.tsx`. These apply the resource width policy to
Nox's SectionLayout/SectionGroup/Section primitives. The shared dashboard owns the
two-column grid, column heights, facing corner radii, action/usage rows and session
section insets. Use `contentPadding` from `resource-page.module.css` for the same
body gutters with either native scrolling or PanelScroll.

`--resource-page-column-width` is the single panel-width setting (420px by default).
The two-column content width is derived in the shared page scope from it and the
shared gap, so page-local token overrides also resize the toolbar and body.
`--resource-action-size` and `--resource-action-radius` similarly own action tile
dimensions; account quota geometry is derived from the same size. The single
responsive breakpoint in `resource-page.module.css` changes the toolbar, content
bound and dashboard together. Do not introduce per-page widths or breakpoints for
these dashboards. Pages supply content, content-specific information heights and
the optional `actionsFirstOnNarrow` ordering; they do not redefine the layout.

## Module imports

`ts/package.json` defines the application's internal paths with the standard `imports` field:

```json
{
  "imports": {
    "#gen/*": "./gen/*.ts",
    "#src/*": "./src/*"
  }
}
```

The root is the web package directory, `ts/`. Use `#gen/cxz/session_pb` for generated protocol code and `#src/shared/api/connection.ts` for application code across folders. `#src` targets include their actual file extension, including `.tsx`, `.css` and asset query suffixes such as `.svg?raw`. Keep imports within the same folder relative (`./router`). Generated files keep their generator-owned imports.

TypeScript's Bundler resolution, Vite, Vitest and the application Storybook read this mapping directly. Do not duplicate it in TypeScript `paths` or individual Vite configurations. The independent UI package continues to use its own local imports and public package entries, without depending on the application's aliases.

See [Storybook](storybook.md) for preview and browser checks, and the [UI package README](https://github.com/lesomnus/nox/blob/main/README.md) for installation, theming, translation and optional-editor usage.
