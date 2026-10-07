# Browser settings

The resource sidebar's **Settings** button opens settings. Topics belong to the left
panel, **General** first and **Editor** second; there are no top tabs. The settings body is at
most **600px** wide and centered, with global and session editor groups stacked.
Production and the WASM sandbox use the same settings components. Editor
controls save immediately; the JSON editor saves explicitly with **Save** or
Ctrl+Enter.

At **1600px of available width** after the resource sidebar and panel, settings
use the conversation layout's **800px** left column and a 1px divider, with a
**settings.json** editor in the remaining space. Below that threshold,
**Edit settings.json** opens the file in the same centered body area; the
**Editor** topic or **Back to settings** returns to the form. On mobile,
the topic panel sits above the body alongside the resource sidebar.

The JSON editor stays mounted when folded: resizing or moving between the form
and file preserves its draft, Monaco Undo/Redo and selection. Shrinking a wide
window while focus is inside the JSON pane keeps that pane available as the
narrow file view. Pristine JSON follows form changes immediately; a dirty draft
uses the conflict protection described below. Form and file panes scroll
independently. Leaving the settings page discards unsaved JSON edits.

The JSON pane and the session's read-only file preview both use
`ts/src/source-editor.tsx`, with the same Monaco options and palette, `#141414`
dark background/gutter (light: `#fafafa`), 12px monospace font, line numbers, Find and scrollbar. The
pane header and footer share styles as well. Only settings JSON is editable.
Monaco loads on first display; folding keeps its model and view. External
reload/import/pristine updates replace its contents; normal typing and theme/
tab changes preserve its undo history. A JSON language worker supplies syntax
highlighting and diagnostics, with schema network requests disabled. Known
preference values still use the whole-file save validation below.

All settings live in one JSON object, stored as the complete file text under
the browser's **localStorage `settings`** key. There are no separate per-setting
storage entries. Export and import use **settings.json**. The format uses flat
dotted keys, following the [VS Code settings file](https://code.visualstudio.com/docs/configure/settings)
pattern; it is ordinary JSON, without comments. Unknown keys are retained when
editing known settings through the UI.

```json
{
  "ui.language": "en",
  "ui.theme": "dark",
  "editor.indentSize": 2,
  "editor.insertSpaces": true,
  "editor.tabSize": 4,
  "editor.colorPalette": "muted",
  "session.editor.tabSize": 8
}
```

## Display language and lazy language packs

English is the default even when the browser prefers another language. The
**General → Language** section offers English and 한국어 by their native names. Choosing a
language does not save, load or activate it. **Apply settings** first downloads
its pack, then persists `ui.language` in the same settings.json document.
`en` and `ko` are currently supported; missing values resolve to `en`, while
unsupported values fail whole-file validation. Other preferences and unknown
keys are retained. Editor controls continue to save immediately.

English source messages are bundled. `src/i18n.ts` has a static allowlist of
loaders; Korean uses a separate Vite chunk via dynamic import. Requests are
deduplicated and successful packs reused for the current page. Reloading with a
saved language loads only that pack. An activation generation discards late
loads after a newer preference wins. A failed download or storage write cannot
apply a newly selected language or replace the saved file; the picker remains
available to try again. A startup/cross-tab load failure retains the active
language and offers Retry. A retry after a module evaluation failure may need
a page reload because browsers cache evaluated modules.

Saving/importing JSON prepares its requested pack before persistence too. A
pending JSON save captures its original conflict baseline and preserves edits
made while loading. Cross-tab changes load and apply the stored language without
remounting the transcript, composer, terminal or Monaco models. `html.lang`,
application labels, accessible descriptions, date formats, relative timestamps
and response durations follow the active locale. Conversations, code, chip
bodies, agent-provided questions, model IDs and settings keys are unchanged.
Backend error text and third-party editor/terminal content retain their own
language; this pack does not configure the connected OpenVSCode workbench.

To add a language, extend Locale/languages/loaders and settings validation, then
add a pack under `src/locales/` that satisfies `LanguagePack`. English messages
are typed source-string keys. Translate whole messages with named placeholders;
do not translate data or concatenate reordered sentence fragments. `i18n.test.ts`
checks pack completeness and placeholder parity. `i18n-react.tsx` exposes React
subscriptions; components using translated strings subscribe so memoized rows
also update without changing keys. There is no new i18n dependency or backend
settings API. `sandbox-e2e/language.spec.ts` checks lazy fetches, persistence,
failures and editor state; production tests cover the actual gateway CSP.

## Theme and reusable controls

**General → Theme** saves `ui.theme` immediately: `light`, `dark`, or `system`.
Missing values resolve to **dark**, preserving the existing appearance; invalid
values fail whole-file validation. System follows `prefers-color-scheme` live,
including changes while the page is open. Same-origin storage changes and reload
use the saved preference. The monochrome ramp in `src/theme.css` covers the app,
composer, cards, menus, scrollbar and fades. Native syntax palettes and both
Monaco panes adapt to the effective theme; the terminal updates in place without
reconnecting. The white Claude Spark keeps its supplied color on a dark backing
in light mode. Connected OpenVSCode retains its own appearance settings.

Frequently reused inputs have their own files:

- `src/value-menu.tsx`: model/effort, language, theme and palette menus. The
  selected row overlays the trigger's text origin; current row, divider, then
  other choices. Native button activation, arrow keys/Home/End, Escape with
  focus restoration, outside dismissal, a bounded scrollable option list and
  automatic up/down placement are shared. A fixed portal avoids clipping in
  scrollable settings panes. Default choices display the effective value in
  muted text; explicit values have normal text. No UI library was added.
- `src/setting-slider.tsx`: native discrete slider with position **0** resetting
  the key, then **1–8** explicit values. The reset position displays the inherited
  numeric value and a reset symbol; keyboard Home/End/arrows work. Existing JSON
  values **9–16** remain valid and their actual number is displayed (thumb at 8)
  until the user chooses a UI value. Reading settings never clamps or rewrites
  the saved file.
- `src/segmented-control.tsx`: equal-width native radio cells with a sliding,
  shadow-free selection box. Tab input uses reset/effective value, Spaces and
  Tab character. Reset deletes the key; explicit false still means a real Tab.
  Arrow-key selection and browser focus traversal use native radio behavior;
  reduced motion disables the indicator animation.

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
invalid. Resetting a control deletes its key and shows the current effective
value in muted text; there are no visible Default/Inherit global prefixes. `indentSize` and `tabSize` are intentionally independent.

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
and selection preserved in conversation inputs. The settings JSON editor uses
Monaco indentation and Undo/Redo with the global editor options. Ctrl+M switches
to browser Tab focus traversal in either editor. Ctrl+Enter saves the current
Monaco buffer. Text editing never modifies preference values until saved.

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
until **Reload saved file**. Saving also compares the expected raw file against
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
