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

The base components inherit the host font and text color. Fixed-width UI text uses
`--ui-font-mono`, including elapsed times, numeric metrics, aliases and status
values. `monospaceFont()` resolves the same CSS token for canvas terminals and
editor defaults; explicit editor font settings still take precedence. The
stylesheet defines only namespaced tokens and styles for library component
classes; it does not reset the host's body, buttons or form fields. Dark is the
default palette. Set `data-ui-theme="light"` on the document root for the light
palette. Root-level `--ui-*` overrides also reach portaled menus and dialogs. Keep
token values in CSS rather than duplicating design measurements in documentation.

`DialogTrigger` requires `adjacent` to show a disabled visual copy above a modal
backdrop. Use it only when the dialog actually sits beside its source. Centered
confirmations and dialogs docked to a screen edge leave the source under the
backdrop. If an anchored dialog cannot fit beside its source after a resize,
hide its foreground trigger until adjacent placement is restored.

The public core entry exports Button/ButtonContent, ConfirmButton, ActionMenu, ValueMenu, SegmentedControl, Switch, ConfirmationDialog, EditableValue, FloatingCard, SettingField, SettingSlider, ElapsedTime, SplitLayout, ResizeHandle and UIProvider. Button's `toolbar` variant provides the compact rectangular shape. Callbacks and selected values belong to the consuming application; cxz adapters handle persistence and session operations separately.

`SplitLayout` takes a controlled `SplitNode` tree, a `renderPane` callback, and `onResize`. `setSplitRatio` applies a bounded ratio update to that tree. Separators support pointer dragging, arrow keys, Home/End and double click to equalize; the host supplies the accessible separator label. It owns no workspace connections or tabs, so it can lay out arbitrary content in other applications. When moving expensive stateful content between panes, keep its instance outside the changing tree and attach a stable portal host to pane slots, as the terminal integration does.

`ResizeHandle` controls one neighboring pane in pixels. Supply `value`, responsive
`min`/`max` limits and `onChange`; `axis` selects horizontal or vertical resizing.
It captures pointer drags, supports arrow keys and Home/End, and accepts optional
`onReset` for double click. The host owns layout and content; the handle does not
remount either pane. Its appearance follows the same separator tokens as split layouts.

## Terminal workbench

The separate terminal entry provides tabs, drag docking, nested resizable panes,
status/clipboard feedback and folding. It does not install xterm, open sockets or
look up projects. Supply your own session renderer; cxz keeps its existing
connection and settings adapter in the app.

```tsx
import { TerminalWorkbench } from "@lesomnus/cxz-ui/terminal";

<TerminalWorkbench
  visible={visible}
  hide={() => setVisible(false)}
  renderSession={(props) => <MyTerminalSession {...props} connect={connect} />}
/>;
```

`TerminalSurfaceProps` describes the renderer contract:

- Each `id` owns a stable mounted session. `groupId` and `placement` change when
  docking; they must not become connection effect dependencies.
- Create a stable DOM host, render its content with a React portal, and attach it
  to `getHost(groupId)` in a layout effect. Hide inactive hosts with `active`.
  Remove the host in a layout-effect cleanup so StrictMode replay is safe.
- `visible`, `active` and `focused` drive fitting and focusing without reconnecting.
  The `ref` exposes `focus()` and `reconnect()`. Report focus with `onFocus(id)`.
- Report connection state, ended state and clipboard feedback with `onStatus`.
  The host can supply an `error`, `formatStatus`, `hideShortcut` and `className`.
  UIProvider translates built-in labels; terminal sizing uses `--ui-*` tokens.

**Components/TerminalWorkbench → Portable** demonstrates the complete contract
using plain React inputs without application providers or CSS. Its drafts and
submitted lines survive tab moves, split-tree rebuilds and folding. This entry
can move to a dedicated terminal package later without changing the application
transport boundary.

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

`src/components/<component>/` keeps each control's implementation, CSS Module and Storybook examples together. The terminal workbench is in `src/terminal`, the optional editor is in `src/editor`, the translation provider is in `src/providers`, and theme tokens are in `src/theme`. Multi-component integration stories live in `src/stories/integration`. Use the public package entries rather than depending on this internal layout. App pages, session behavior and workspace integration are described in [web source organization](../../../docs/web-structure.md).
