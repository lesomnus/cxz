# Web source organization

The web application separates application wiring, pages, feature implementation and shared application infrastructure. The reusable UI package remains independent of all four.

```text
ts/
├─ packages/ui/src/
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

`shared` contains application-wide connection/inventory, settings, translations, fonts, navigation primitives and other utilities. It is still cxz application code: shared API connection state uses feature-owned draft/cache type contracts. It is not the package boundary for reuse by another application. Keep reusable controls in `@lesomnus/cxz-ui`, and keep session-specific card hosting in the session feature.

In the UI package, each component folder contains its implementation, CSS Module and independent Storybook examples. The optional editor has its own entry. Theme tokens and the translation provider have dedicated directories. Consumers import the public package entries; internal file paths are not an API. Application code uses those public entries directly rather than maintaining one-file re-export wrappers.

Tests stay beside the behavior they verify. Application stories stay beside their components; package-control stories demonstrating application integration live under `src/storybook/components`. Shared preview framing and synthetic session data stay under `src/storybook` and are not used by application entries. Both Storybooks discover stories recursively, so directory moves do not require individual registration.

Global application layout CSS lives under `src/app/styles`; component CSS lives in its UI component folder. Shared native scrollbar styling remains under `src/shared/scroll`. CSS tokens and behavior implementations are the source of truth for design values. Generated protocol files remain under `ts/gen`, and generated UI distribution and Storybook outputs remain excluded from Git.

See [Storybook](storybook.md) for preview and browser checks, and the [UI package README](../ts/packages/ui/README.md) for installation, theming, translation and optional-editor usage.
