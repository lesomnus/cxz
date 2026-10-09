# Web design sandbox

Run the actual cxz web UI against an in-browser Go/WASM simulator. It needs no
Manager, Docker, SSH tunnel, TLS certificate, provider account or paid model call.
The simulator is a design fixture, not a replacement for integration testing the
real Manager, authentication, providers or containers.

The shared [web design principles](web-design.md) explain the visual direction
and interaction intent behind these fixtures and the production UI.

## Start

Use the repository's Go toolchain (go.mod) and Node/npm, then:

```sh
npm ci --prefix ts
npm run --prefix ts sandbox
```

Open `http://127.0.0.1:5173/sandbox.html` if a browser does not open automatically.
The command compiles WASM once and starts Vite on loopback; editing React/CSS
updates the preview. After changing Go simulator code, run
`npm run --prefix ts sandbox:prepare` and reset/reload the preview. There is no
server-side cxz installation to update. Ctrl+C stops the static development server.
The first WASM load is roughly 23 MiB; a loading indicator reports bytes loaded.

Starting is bounded. The sandbox gives the whole boot 15 seconds — the module's
own deadline covers only its last step, so a stall before that used to leave the
page on "Starting sandbox…" with no error, no progress and nothing to retry. On a
timeout it starts once more without the module cache, says so in a note that
stays on screen, and reports a second timeout instead of retrying forever. The
module cache is skipped entirely under browser automation, where every page is a
fresh context and the cache can only write 23 MiB it will never read.

The script works from Windows or Linux with Go and Node installed. It uses Node
process APIs rather than POSIX shell environment syntax, and copies wasm_exec.js
from the same Go toolchain that compiled the module.

## Workspace file preview and fake Connect

When the area after the sidebar/session panel reaches the CSS split breakpoint,
the conversation keeps a reading column and the remaining width shows a file
tree/read-only Monaco view.
The WASM service supplies README, Go/TypeScript and devcontainer fixture files
for each project. **Connect** changes to **Simulated connection** using the
configured pace; **Disconnect** returns to File preview. No Linux VM, container,
remote IDE or real filesystem is started. Tabs and connection state are retained
per project while resizing and switching sessions; conversation drafts are kept.
The [workspace editor contract](web-editor.md) describes real devcontainer
connections and the separate review of Linux/VS Code running inside browser WASM.

## Simulated workspace shell

Press **Ctrl+Backquote** or the **>\_** button to the left of Send to open a
terminal below the composer. The WASM service simulates `pwd`, `ls`, `cat`, `echo`,
`clear`, `help` and `exit` over the same Terminal RPC used by native clients.
`cat README.md` shows the selected project's fixture file. Folding preserves the
screen and input, while switching sessions or Reset closes it. There is no Linux
VM or real command execution; see the [terminal contract](web-terminal.md).

## Scenarios and controls

The Scenario selector opens one of the sample sessions:

- English answers with Markdown, tables, checklists and code.
- Long history exceeding the UI's 512-event cache, with periodic user prompts
  to preview scrollbar markers.
- An approval question using the regular answer controls.
- A simulated usage-limit/error case.
- Seeded random tool activity followed by a canned response.
- A stopped session to review its status indicator.
- A session in a second project to check grouped project/session navigation.

Send any text to trigger timed tool events and a preset response. Tools only
produce fixture events; they never run commands or edit files. Approval answers
resolve the pending question; sending again in that scenario asks another question.
Model/effort dropdowns use simulated provider catalogs and settings commands;
changing settings does not start a chat turn. Stop/resume and interrupt remain
available in the fixture RPCs, with lifecycle behavior covered by Go tests.

The resource sidebar switches between Sessions and Projects. Sessions follow
project/session ordering, without directory icons or branch decoration. Each
session shows a title, then a monospace alias and model, with a TUI-style activity
or question indicator. Buttons have no borders and transparent backgrounds until
hover; pressing scales their content with a bounded contraction along the longest
edge, and actions trigger on release. Button sizes, insets, radii and appearance
come from the shared CSS tokens rather than a separate specification here.

The fixed-width model/effort fields use the current run's provider catalog. Settings
require an idle session. A model change clears explicit effort first, awaiting
provider confirmation before applying the model. Missing capabilities disable the
selectors rather than treating slash commands as chat prompts. Only values open
styled dropdowns; labels stay inert. The current value overlays its original
text position, separated from other choices, with the same full-width row and
padding as the other options. Shared inset tokens keep labels and values aligned;
hovered values brighten. At the screen bottom, choices expand upward while the
current row stays in place. Keyboard navigation, Escape and outside dismissal
are supported.

The icon-only Send sits on the right of a toolbar above the text input. The wrapper
has no padding, and the button's right gap matches its vertical gaps. The input
extends beyond the wrapper while retaining its reading width; their bottom borders
overlap. Inner button rounding follows the outer corner center. Ctrl+Enter sends,
with a hover shortcut. Quota uses the TUI's Braille cells at the Model/Effort label
font size; its intrinsic width and a separate gap keep it clear of the context donut.
Hover reveals reset time. Context is a donut with a used/capacity token popover.
These are simulated snapshots, not real account limits. Status snapshots are
retained separately from the rendered history window. User and assistant headings
align, with message content inset inside the shared conversation edges.

Both viewport fades use the page background color and sit behind the composer.
The bottom fade ramps in smoothly. Ordinary movement extends the outgoing fade
and then settles; edge tension adds a stronger extension within the viewport.
The top fade starts shorter, and directional tension keeps the opposite fade smaller.
Scroll controls, headings and Copy paint above the fades. Message bodies and
task response rows disappear into the background together.

Latest is an icon-only down arrow centered in the composer toolbar, sharing Send's
button style. Its absolute overlay occupies no layout space. It slides up from
behind the input after sufficient upward reading movement and slides back down
after sufficient downward movement. Small reversals consume accumulated distance
without toggling it. At the latest position it hides and resets; hidden controls
cannot capture clicks or focus. Cache insertion/eviction and row measurement are
excluded from movement. Reduced motion switches immediately.

The conversation hides native scrollbars. Hover reveals a handle and user-prompt
markers; approaching the handle widens it. Its local display range is bounded,
independently of total history. Markers and the handle share a range-rebase
animation on the same clock, with stable event identities. Height measurement
retargets that clock without restarting individual markers. During edge tension,
the handle stays near the boundary while markers move in phase with the transcript.
Elastic reserve keeps the stretched handle inside the viewport. Pulling beyond
an end applies saturating tension while accelerating through adjacent ranges.
Returning to the rail starts at the current reading position, including on the
first drag; manual gestures pause live follow immediately. Wheel steps and handle
movement use short, non-overshooting interpolation, settling on the exact target;
OS trackpad motion stays native. Nested scrolling and explicit navigation are respected.

History arrives in pages, starting near the latest entries, with a bounded cache.
Only visible messages and an overscan buffer are mounted. Measured variable heights
drive virtual positioning. Markers use stable message coordinates and an invertible
message-to-pixel map; a fixed total height and visible-message anchor prevent jumps
during measurement, resizing and page replacement. New events follow only while
actually at the bottom. Paging stops at the retained history boundary.

The user input preceding the reading position sits in a fixed overlay at the
top of the transcript, outside the scrolling canvas. Exposure uses equal clearance conditions
above and below the upper edge: the previous input's bottom and the next visible
input's top. A nearby or straddling input hides it; sufficient clearance gradually
reveals the bottom edge and opacity together. Missing edges impose no limit.
Hidden overlays cannot capture clicks or focus. Approaching the top of the column
or focusing the available button slides the whole box down. Once expanded it stays
open for a minimum hold period, then a leaving grace period; returning cancels the
pending close. Focus keeps it expanded too. Touching the approach area reveals it.
Long inputs are height-bounded and scroll independently. Clicking returns to the
original input with space above, preserving that gap through row measurement.
Clicks dismiss instantly, bypassing timers and fades, including while an uncached
input loads. The overlay adds no virtual height and never follows scroll offsets.

The accepted handle/marker experience and regression steps are recorded in
[the scroll experience contract](web-conversation-scroll.md). Current geometry,
thresholds and timing are defined in its linked code, not duplicated in this guide.

**Reset sandbox** recreates the Worker, discarding drafts, history and pending work,
and applies Seed and Pace. Identical seeds and the same per-session sequence of
sends produce identical random tool sequences. Switching scenarios preserves
session state; reloading starts fresh. Event timestamps use the current clock,
including seeded history. The seed controls content, not a fixed date. Response
footers omit the year; metric icons expose names and scope on hover and to
assistive technology. Relative-time rules are defined in `src/message-time.ts`.

While the agent is working, the composer toolbar has a subdued aurora behind it.
The field keeps volume at both ends of the toolbar and fades softly beyond them.
Each cluster contains differently colored orbs revolving around a shared center.
The centers wander freely within bounded regions, while individual orbs slowly
grow, vanish and change ellipticity. Small dense foci drift inside broad diffuse
gradients. New random paths are chosen only at the end of long browser animations;
JavaScript does not run per-frame motion or change layout. All clusters emerge
small at work start and contract gradually at the end, retaining movement through
the visible exit before pausing. Colors are subdued.
Gravity centers stay inside the toolbar. Broad overlapping clusters and a diffuse
moving base cover the whole bar, including both ends, even when individual orbs
vanish. The combined surface swells gently rather than separating into tiny spots.
It lives in a bounded background layer behind the transcript, scroll fades and
composer. Painting is contained to the conversation so large rotating orbs cannot
extend page scrolling. The field tracks the toolbar on layout changes, including
composer expansion and viewport resizing, without per-frame measurements. The toolbar
uses real backdrop blur; normally transparent conversation and event cards also
blur their backdrop to preserve readability.
Independent colored fields drift at different phases and fade in/out without
changing layout or intercepting controls. Waiting for an answer pauses the glow;
reduced motion uses a static glow. Native snapshot/live state controls it, separate
from the historical reading position.

Response Copy icons appear on response hover or control focus without changing
footer geometry. Double-clicking transcript task and event summaries opens a non-modal overlay
below the selected summary. Only its exterior expands and gains a border; its
content and measured height stay unchanged. Details emerge under the summary's
layer, follow scrolling, and close when the trigger leaves view. They stay within
the conversation bounds. The detail exterior matches the expanded summary width,
with square top corners and no repeated title. Input, Output, Result and Approval
use tabs; each selected section is a read-only Monaco editor with detected syntax
and shared editor settings. Only the editor viewport scrolls, with the common
handle skin. Tabs preserve their own reading positions and support arrow keys.
Single clicks do not load details; keyboard activation opens them. The
summary remains clickable above the details: click it again, press Escape, or
click anywhere else in the transcript to dismiss. The tabs have room above and
below; their right-hand Copy icon copies the active section's entire source,
including content outside the editor viewport. There is no separate Close
button. Opening another item replaces the active preview.

Request details and paste previews retain their composer anchor. Cards share the
same shell; composer previews retain rounding on all corners, an outer border, narrow content/header
padding and strong backdrop
blur. The title alone gains a little left margin. Close shares Send's rectangular
style. Code and raw text retain opaque boxes. Inner radii follow outer border and
inset tokens so the corner centers align. A last-child `pre` has no bottom margin.
Composer-anchored previews leave the toolbar, editor and metadata accessible.
Task details may overlay that area when their trigger is near the bottom.
Cards add no transcript height; long content scrolls inside a height capped by
both shared styles and available space.

The host uses overflow clipping with shadow room, clamped to conversation bounds.
Do not use ancestor clip-path, masks or opacity that form a backdrop root and stop
the blur sampling the conversation. Opening another item replaces the previous
preview: Close or Escape never restores an older card. Clicking empty transcript
side margins closes it while content, links, buttons and scrolling stay interactive.
Enter/exit transitions respect reduced motion and never alter transcript coordinates
or handle/marker physics. Previews are discarded on session navigation.

Pending Question/approval requests use the same FloatingCard shell and anchor,
outside the composer form. They have no Close control. Escape, side-margin dismissal
and preview replacement never discard requests or selected/free-text answers.
Only explicit Submit/Allow/Deny resolves them. Enter in an answer does not send the
composer. Whenever a composer-anchored preview is active, questions shrink and
dim to show their background layer. If the preview is at least as tall, questions also move upward
so part of their top remains visible. A shorter preview keeps the bottom anchor,
while still shrinking and dimming. Both layers stay within the conversation.
Transcript-anchored details occupy their own layer below pending questions and
do not replace or cover their controls. ResizeObserver compares actual heights;
transitions respect reduced motion.
Covered questions are inert until the preview closes, restoring state and focus.
Multiple pending requests remain in a bounded, scrollable layer.

Question options are monochrome cards with titles, descriptions and native
radio/checkbox controls, retaining keyboard navigation and accessible labels.
Options and Other editors have no border inside the bordered Question card;
selection, hover and focus use background tones. Fieldsets keep grouping semantics
without default border, margin or padding. Ordinary Other answers reuse ComposerEditor
with multiline text, monospace line numbers, atomic chips and native Undo/Redo.
Answer editors and composer have independent values but share the connection's
bounded paste cache. Submit expands chips to original text in answersJson; Enter
inserts a line and Ctrl+Enter never sends the conversation draft from an answer.
Password answers keep masked native controls. Chip editing releases the Question's
inert state before native insertion, preserving the correct editor's Undo history.

After an accepted Send, a non-interactive copy of the visible draft moves upward
out of the composer while shrinking, followed by the actual input message
emerging into the transcript and returning to full size. These transforms do not
change measured row heights or create optimistic journal entries. Failed sends
keep the draft; edits made during sending survive, and switching sessions removes
the decorative layers without restoring an accepted draft. Reduced-motion
preferences skip the effect. `ts/src/send-motion.ts` coordinates the animation;
CSS tokens define its appearance and timing.

The shared composer is a monospace editor with logical line numbers and no resize
handle. Enter continues Markdown bullet lists at the same space/tab indentation;
empty items end the list, Shift+Enter inserts a plain newline, and fenced code
does not trigger list continuation. Inline code spans keep their visible backtick
delimiters and use a dark background without changing text/caret geometry.
Typing an inline backtick inserts its closing partner; typing the closing
backtick advances past the existing delimiter. Escaped backticks and fenced
code retain ordinary input behavior. Typing on the final logical line keeps
the editor scrolled to the bottom. Content that exceeds the normal visible
height expands the editor within its CSS viewport cap and contracts again
when the draft fits.
Typing three backticks at the start of a line opens an inline black code
block, inserts the matching closing fence two lines below, and places the cursor
on the empty body line between them. The opening backticks remain visible, with
a syntax selector beside them and a shared Close button at the right, without
changing native text/line-number coordinates.
The default Auto setting detects the snippet's language; manually chosen syntax
wins. Close finishes the Markdown fence and moves the cursor into prose after it;
it does not delete the snippet. Existing fenced snippets also render this way,
including multiple blocks, language aliases and longer backtick fences. Code
blocks expand the editor within its CSS height cap.
By default code uses subdued, low-saturation purple/green/blue/brown token colors on black;
the surrounding UI stays monochrome. Highlight.js 11.12.0 is the runtime
dependency, with a bounded set of explicitly registered grammars. User-authored
HTML remains escaped text.

Tab inserts the configured indentation, or indents the selected logical lines.
The [browser editor settings](web-settings.md) can change the indentation size,
choose actual Tab characters, adjust their display width and choose a syntax
palette. Session editor fields inherit global values independently.
Shift+Tab removes up to the configured indentation's leading spaces or one
existing tab per selected line.
A selection ending at the next line's start does not change that next line.
Each operation is one native edit, retaining Undo/Redo, the selection range and
its direction; paste chip labels remain intact. Ctrl+M toggles Tab focus mode
within each editor, with a status notice and an accessible shortcut description.
In focus mode, Tab and Shift+Tab follow normal browser focus order. Ctrl+M returns
them to indentation. IME composition and modified Ctrl/Alt/Meta+Tab are left to
the browser. Question Other answers share the same controls.

Drafts remain native Markdown, preserving selection, Undo/Redo and session draft
restoration. Sending expands code's paste chips before detection, replaces Auto with the detected
syntax name (or plaintext when detection is inconclusive), canonicalizes common
language aliases and finishes an unclosed fence. Surrounding prose and code body
bytes remain intact. Question Other answers use the same editor and serialization.
The highlighter uses a bounded common language set and cached detection samples.
Oversized blocks or lines fall back to unhighlighted text; the limits are defined
in `src/composer-code.ts`. Fences inside
a paste chip grow the enclosing Markdown fence as needed, and Markdown within
standalone chips remains unchanged. See the
[Highlight.js API](https://highlightjs.readthedocs.io/en/latest/api.html) for the
underlying detection/highlighting interface.

Code backgrounds sit below the native caret/selection, and interactive controls
and chips above the textarea. Scrolling uses mirror top/left rather than a
transform, keeping those layers in the same stacking context. During IME
composition, native text is visible and code backgrounds remain in place.

Line numbers follow soft wrapping, scrolling and viewport changes. Text pastes over
800 Unicode characters or containing at least three newlines become inline
chips, matching the TUI threshold. Click a chip, select it with Left/Right and
press Enter, or use Ctrl+P to preview the original; the preview can remove that
occurrence or expand it into editable text. Backspace/Delete removes a whole
chip. If the draft changes while its preview is open, expansion/deletion is
blocked and asks you to reopen the chip, avoiding replacement at a stale offset.
Sending and copying expand chips to original text in one pass, preserving
whitespace; short pastes remain ordinary text. Drafts and paste bodies stay in
the authenticated connection's memory across session navigation, and are lost
on page reload, sign-out or sandbox Reset. Each paste is limited to 1 MiB, with
a 32 MiB connection cache. Paste insertion/removal uses native undo when the
browser supports `insertText`, with a `setRangeText` fallback. IME composition
retains the native textarea and cannot trigger Ctrl+Enter submission.

Fake history and remembered send IDs are bounded by the simulator. Client rendering
uses the regular bounded cache and visible-message overscan from
`src/virtual-messages.tsx`.

## Builds and verification

```sh
npm run --prefix ts sandbox:build
npm run --prefix ts test:sandbox
```

The first builds a standalone static preview into `ts/dist-sandbox`; the second
uses Playwright Chromium to test that build (install the Playwright browser and
OS dependencies first). It covers real WASM RPC, sending, approval, long history,
model/effort selection, reset and seed replay without HTTP RPC/auth requests.
Desktop checks also cover project/session ordering, heading/input alignment,
Ctrl+Enter, fixed field positions, quota/context popovers, bounded button press
scaling, background fades with tension, overlapping input borders, scrollbar prompt phase
at both edges, hover/stretch, near-bottom scroll stability, resizing and full
backward/forward history paging with bounded DOM and cache, first-drag return,
elastic handle bounds and wheel response/settling. A boot test stalls the module
request on purpose to check that the page recovers on its retry, still explains
why afterwards, and reports a boot that never finishes rather than hanging.
Startup also waits for a real project-list RPC before showing the workspace.
A worker that publishes its entry point but cannot answer RPCs is closed under
the same deadline and retried once; the browser suite forces this condition by
dropping the first MessagePort handoff. Late starts from timed-out attempts close
their workers instead of leaking them.

Serve a static build at the origin root, preserving its Worker/WASM assets and
application/wasm MIME type. `npx vite preview --config vite.sandbox.config.ts`
from `ts/` provides a local preview. Development/preview configuration supplies
COOP/COEP isolation headers; a different static host should supply the same.
The local simulator uses Go memory rather than SQLite/OPFS; no conversation
state survives a page reload. payday may cache the WASM executable bytes,
which is separate from conversation state.

Normal `npm run --prefix ts build` still writes only the authenticated production
web client into `internal/webui/assets`. Sandbox binaries, Worker and controls are
not shipped in that build. The generated `.sandbox` and `dist-sandbox` directories
are ignored by Git.

## Implementation boundaries

- `cmd/cxz-sandbox`: WASM-only entry point, grpc-dgram MessagePort gateway.
- `internal/websandbox`: in-memory ProjectService/SessionService subset and
  deterministic fake-agent jobs. Generated protobuf services are shared with the
  real server, but the fixture lifecycle is deliberately simplified. Unimplemented
  RPCs remain unimplemented; it does not exercise the production lifecycle stack.
- `ts/src/sandbox.tsx`: payday sandbox startup, seed/pace controls, reset and a
  sandbox Connection. No browser-auth bypass is added to the production app.
- `ts/src/app.tsx`: shared production workspace/conversation UI.
- `ts/src/connection.ts`: accepts a Connect Transport; normal connections use HTTPS,
  while the sandbox supplies payday's Worker transport.

Use the existing HTTPS browser fixture and Go integration tests for real gateway
behavior. This sandbox helps iterate on layout and interaction without real work.
