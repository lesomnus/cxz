# cxz UI

Reusable React components with their own CSS Modules and a monochrome token palette. The package has no cxz API client, session model, browser settings store or language-pack loader. It requires React 19 and React DOM 19 from the consuming application.

## Using the package

Build a distributable archive from this repository:

```sh
cd ts
npm pack --workspace @lesomnus/cxz-ui --pack-destination /tmp
```

Install the resulting archive in another React project. No registry publication is required:

```sh
npm install /tmp/lesomnus-cxz-ui-0.1.0.tgz
```

Import the public stylesheet once, then import the components you need:

```tsx
import "@lesomnus/cxz-ui/styles.css";
import {
  Button,
  ConfirmationDialog,
  EditableValue,
  ValueMenu,
} from "@lesomnus/cxz-ui";

export function SaveButton() {
  return (
    <Button type="button" onClick={() => save()}>
      Save
    </Button>
  );
}
```

The base components inherit the host font and text color. The stylesheet defines only namespaced tokens and styles for library component classes; it does not reset the host's body, buttons or form fields. Dark is the default palette. Set `data-ui-theme="light"` on the document root for the light palette. Root-level `--ui-*` overrides also reach portaled menus and dialogs. Keep token values in CSS rather than duplicating design measurements in documentation.

The public core entry exports Button/ButtonContent, ConfirmButton, ActionMenu, ValueMenu, SegmentedControl, Switch, ConfirmationDialog, EditableValue, FloatingCard, SettingField, SettingSlider, ElapsedTime and UIProvider. Button's `toolbar` variant provides the compact rectangular shape. Callbacks and selected values belong to the consuming application; cxz adapters handle persistence and session operations separately.

## Translation

English defaults work without a provider. To use an application's translation system, supply a translator and a locale identifier. Changing the locale refreshes component labels; the package itself downloads no language packs and writes no browser storage.

```tsx
import { UIProvider } from "@lesomnus/cxz-ui";

<UIProvider
  locale={locale}
  translate={(message, values) => translate(message, values)}
>
  <App />
</UIProvider>;
```

## Optional source editor

Install `monaco-editor` when using the separate editor entry. Importing the core entry does not load Monaco, its language services or its workers.

```tsx
import { SourceEditor } from "@lesomnus/cxz-ui/editor";

<SourceEditor
  path="example.ts"
  value={source}
  ariaLabel="Source code"
  theme={theme}
  settings={editorSettings}
  onChange={setSource}
/>;
```

The editor accepts plain settings and theme values; it does not know about settings.json, font downloads or application storage. It loads Monaco lazily and ships editor/JSON worker assets. An existing host MonacoEnvironment is respected. The host gives the editor a bounded layout area. The development entry uses source files for Vite hot reload; production uses compiled ES modules and TypeScript declarations.

## Independent previews

From `ts`:

```sh
npm run storybook:ui
npm run build-storybook:ui
npm run test:ui
```

This Storybook imports the package stylesheet and documentation framing only. Its stories demonstrate core controls, value editing, menus, cards, injected translation and the optional editor without app CSS or cxz providers. The existing application Storybook remains the integration reference for session, composer, Question and transcript layouts.

## Source layout

`src/components/<component>/` keeps each control's implementation, CSS Module and Storybook examples together. The optional editor is in `src/editor`, the translation provider is in `src/providers`, and theme tokens are in `src/theme`. Multi-component integration stories live in `src/stories/integration`. Use the public package entries rather than depending on this internal layout. App pages, session behavior and workspace integration are described in [web source organization](../../../docs/web-structure.md).
