# Browser settings

The resource sidebar's **설정** button opens settings. Topics belong to the left
panel, currently just **에디터**; there are no top tabs. The settings body is at
most **600px** wide and centered, with global and session editor groups stacked.
Production and the WASM sandbox use the same settings components. Editor
controls save immediately; the JSON editor saves explicitly with **저장** or
Ctrl+Enter.

At **1600px of available width** after the resource sidebar and panel, settings
use the conversation layout's **800px** left column and a 1px divider, with a
**settings.json** editor in the remaining space. Below that threshold,
**settings.json 편집** opens the file in the same centered body area; the
**에디터** topic or **에디터 설정으로 돌아가기** returns to the form. On mobile,
the topic panel sits above the body alongside the resource sidebar.

The JSON editor stays mounted when folded: resizing or moving between the form
and file preserves its draft, native Undo/Redo and selection. Shrinking a wide
window while focus is inside the JSON pane keeps that pane available as the
narrow file view. Pristine JSON follows form changes immediately; a dirty draft
uses the conflict protection described below. Form and file panes scroll
independently. Leaving the settings page discards unsaved JSON edits.

All settings live in one JSON object, stored as the complete file text under
the browser's **localStorage `settings`** key. There are no separate per-setting
storage entries. Export and import use **settings.json**. The format uses flat
dotted keys, following the [VS Code settings file](https://code.visualstudio.com/docs/configure/settings)
pattern; it is ordinary JSON, without comments. Unknown keys are retained when
editing known settings through the UI.

```json
{
  "editor.indentSize": 2,
  "editor.insertSpaces": true,
  "editor.tabSize": 4,
  "editor.colorPalette": "muted",
  "session.editor.tabSize": 8
}
```

## Editor scope and inheritance

| Global key | Default | Behavior |
| --- | --- | --- |
| `editor.indentSize` | `2` | Number of spaces inserted by Tab and removed by Shift+Tab; integer 1–16. |
| `editor.insertSpaces` | `true` | Insert spaces; `false` inserts an actual `\t` character. |
| `editor.tabSize` | `4` | Display existing Tab characters at this many-column tab stops; integer 1–16. Does not rewrite text. |
| `editor.colorPalette` | `"muted"` | Syntax colors: `muted`, `monochrome`, `cool`, or `warm`. UI chrome stays monochrome. |

Each key also has a **`session.editor.*`** counterpart. Resolution is independent
for every field: explicit session value → explicit global value → default.
Explicit `false` is a value, not inheritance. Missing keys inherit; `null` is
invalid. Choosing **전역 설정 상속** deletes that session key, and the option
shows the current effective global value. Choosing **기본값** removes a global
key. `indentSize` and `tabSize` are intentionally independent.

The session scope applies to **all session conversation editors**, including
Question Other answers. It is not a setting for one session ID. Global settings
also configure the browser's read-only Monaco file viewer. Changing settings
updates a mounted viewer in place and preserves the selected file. Lazy editor
startup uses the latest settings even if they changed while Monaco was loading.
The connected **OpenVSCode Server workbench** retains its own VS Code settings;
this browser file does not change devcontainer files or workbench preferences.

The default palette keeps the composer's subdued syntax colors. The four shared
palettes map to CSS token variables for the native composer and to named Monaco
themes for the file viewer. Highlighter language detection and Markdown sending
remain unchanged. Tab and Shift+Tab still perform one native edit with Undo/Redo
and selection preserved. Ctrl+M switches to native Tab focus traversal, including
in the JSON file editor. Text editing never modifies preference values.

## Persistence and failures

Preferences belong to the **browser origin**, survive reload/sign-out/sandbox
Reset, and are shared across its tabs. Different origins, ports and browser
profiles have independent files. Conversation drafts, paste originals and
credentials are not included. No backend settings RPC, OPFS directory, account
sync or server-side file is required. Reading defaults does not write a file.

The complete file is validated before storage or application. The size limit is
1 MiB; known editor values must match their types and ranges. Invalid JSON or a
failed/quota-denied storage write cannot replace the previously saved file or
apply unsaved preferences. An invalid file already in storage remains available
in the JSON editor for repair; the editors use defaults and form controls are
disabled until the file is repaired. A denied storage read displays an error.

The browser `storage` event updates other open tabs, including mounted editors.
Form edits read and modify the latest saved object to preserve other keys. A
dirty JSON draft stays untouched when the stored file changes; saving is blocked
until **저장된 파일 다시 읽기**. Saving also compares the expected raw file against
storage to catch a change before its storage event arrives. Import explicitly
replaces the entire document. This is localStorage persistence, without
multi-tab transactional locking or automatic conflict merging of JSON drafts.

Implementation: `ts/src/editor-settings.ts` defines keys, validation, resolution
and palettes; `settings-store.ts` owns the whole-file store; `settings.ts` exposes
React subscriptions; `settings-page.tsx` provides form/file editing.
`editor-settings.test.ts` and `sandbox-e2e/settings.spec.ts` cover inheritance,
validation, persistence failures, import/export, stale drafts, cross-tab updates,
mobile navigation and live read-only Monaco configuration. The settings browser
test also checks exact 600px centering, the available-width split boundary,
topic navigation and JSON undo/draft retention through folding and resizing.
