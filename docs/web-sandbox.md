# Web design sandbox

Run the actual cxz web UI against an in-browser Go/WASM simulator. It needs no
Manager, Docker, SSH tunnel, TLS certificate, provider account or paid model call.
The simulator is a design fixture, not a replacement for integration testing the
real Manager, authentication, providers or containers.

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

The script works from Windows or Linux with Go and Node installed. It uses Node
process APIs rather than POSIX shell environment syntax, and copies wasm_exec.js
from the same Go toolchain that compiled the module.

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
hover; pressing uniformly scales their content, capped at 4px along its longest
edge, and actions trigger on release.

The fixed-width model/effort fields use the current run's provider catalog. Settings
require an idle session. A model change clears explicit effort first, awaiting
provider confirmation before applying the model. Missing capabilities disable the
selectors rather than treating slash commands as chat prompts. Only values open
styled dropdowns; labels stay inert. The current value overlays its exact original
text position, separated from other choices, with the same full-width row and
padding as the other options. Menus, value triggers and the composer toolbar share
a narrow 2px inset token. Values and options have 6px horizontal padding;
trigger padding keeps value/label text baselines aligned;
hovered values brighten. At the screen bottom, choices expand
upward while the current row stays in place. Keyboard navigation, Escape and
outside-click dismissal are supported.

The icon-only send button sits on the right of a toolbar above the text input,
inside a zero-padding wrapper. The 28px toolbar has equal 2px top, bottom and right
gaps around its 64px-wide, 24px-high Send button. The wrapper is narrower than the
input, which extends 4px past each side while retaining its width. Their bottom
borders overlap. Wrapper corners are 10px, input corners are 15px, and the inset
Send corner is 7px to share the wrapper corner center. Ctrl+Enter sends, with a hover shortcut.
Quota uses the TUI's eight Braille cells at the same font size as Model/Effort;
its intrinsic width and a separate gap keep it clear of the context donut. Hover
reveals the reset time. Context
is a donut with a used/capacity token popover. These are simulated snapshots, not
real account limits. Status snapshots are retained separately from the rendered
history window. The input extends 10px beyond each conversation edge. User and
assistant headings align, with more padding on message contents.
Both viewport fades use the page background color and sit behind the composer.
The bottom fade starts with a smooth opacity/height ramp, up to the input height.
Ordinary content movement extends the outgoing fade and then settles. Edge
tension adds up to 3.5 input heights, capped at 80% of the viewport. A shorter
top fade indicates earlier history and grows with downward edge tension; in that
direction the opposite bottom fade uses 75% of the tension contribution. Scroll
controls, message headings, Copy buttons and disclosure controls paint above the
fades; the fades affect message bodies without covering other controls. Latest
navigation is an icon-only down arrow in the center of the composer toolbar,
sharing Send's 64×24px borderless button style. Its absolute overlay occupies no
layout space and stays above future toolbar controls. It slides up from behind
the input in 180ms after 96px of upward reading movement, and slides back down
after 96px of downward movement. Small reversals consume the accumulated distance
without toggling it. At the latest position it hides and resets; hidden controls
cannot capture clicks or focus. Cache insertion/eviction and row measurement are
excluded from movement, and reduced motion switches immediately.

The conversation hides native scrollbars. Hover reveals a handle and user-prompt
ticks; approaching the handle widens it. Its local range is capped at twelve
viewport heights in navigation units (100 per message). Prompt ticks and the handle share the same range-rebase
animation on one clock, with stable event identities. Height measurement retargets
that clock without restarting an individual marker. During edge tension, the handle stays
pinned while prompt ticks continue to move in phase with the passing transcript.
A 20px elastic reserve keeps even the fully stretched handle inside the viewport.
Dragging beyond either end applies a saturating spring while accelerating
scrolling through adjacent ranges. Returning to the rail starts at the current
reading position, including on the first drag; manual gestures pause live follow
immediately. Large wheel steps and handle movement use short, non-overshooting
interpolation, settling on the exact target; OS trackpad motion stays native.
Reduced-motion preference, nested scrolling and explicit navigation are respected.
History arrives in 128-event pages, starting
near the latest 256 events, with a cache capped at 512. Only visible messages and
six extra messages on either side are mounted. Measured variable heights drive
virtual positioning; prompt ticks use stable message coordinates and an invertible
message-to-pixel map; a fixed total height and visible-message
anchor prevent jumps during measurement, resizing and page replacement. The
conversation follows new events only while actually at the bottom. Paging stops
at the server's retained history boundary. The latest user input preceding the
reading position sits in a fixed overlay below the title, outside the scrolling
canvas. It uses equal clearance above and below the upper edge: the bottom of
the last passing user input and the top of the next visible input. Either edge
within 96px hides it, including a straddling input. Across 96–128px of clearance,
its bottom edge (up to 8px) and opacity fade in together. Missing edges impose
no limit.
The hidden overlay cannot capture clicks or focus. Approaching the top 28px of
the conversation column or focusing the available button slides the box down
in 180ms. Once expanded it stays open for at least three seconds, even after
the pointer leaves; leaving after that minimum grants one more second. Returning
cancels the pending close. Focus keeps it expanded too. Touching the approach
area focuses and reveals it. Long inputs are
bounded to 180px or 25% of the viewport and scroll independently. Clicking returns
to the original position with 18px above the input, preserving that gap through
row measurement. Clicks dismiss the overlay instantly, bypassing hold timers and
fade animations, including while an uncached input loads. The overlay
contributes no height to the virtual transcript and never tracks scroll offsets.

The accepted handle/marker experience, numeric parameters, restoration steps and
regression checks are recorded in [the scroll experience contract](web-conversation-scroll.md).

**Reset sandbox** recreates the entire Worker, discarding drafts, history and
pending work, and applies the Seed and Pace controls. Identical seeds and the same
per-session sequence of sends produce identical random tool sequences. Switching
scenarios alone preserves session state. Reloading the page also starts fresh.
Event timestamps use the current clock when each event is created, including
seeded history. The seed controls content, not a fixed date. Response footers
show month/day and time without a year; metric icons expose names and usage
scope on hover and to assistive technology.

The shared composer is a fixed-size monospace editor with logical line numbers.
Numbers follow soft wrapping, scrolling and viewport changes. Text pastes over
800 Unicode characters or containing at least three newlines become inline
chips, matching the TUI threshold. Click a chip, select it with Left/Right and
press Enter, or use Ctrl+P to preview the original; the preview can remove that
occurrence or expand it into editable text. Backspace/Delete removes a whole
chip. Sending and copying expand chips to original text in one pass, preserving
whitespace; short pastes remain ordinary text. Drafts and paste bodies stay in
the authenticated connection's memory across session navigation, and are lost
on page reload, sign-out or sandbox Reset. Each paste is limited to 1 MiB, with
a 32 MiB connection cache. Paste insertion/removal uses native undo when the
browser supports `insertText`, with a `setRangeText` fallback. IME composition
retains the native textarea and cannot trigger Ctrl+Enter submission.

History is bounded to 3,000 events per fake session; client rendering still uses
the regular 512-event cache and visible messages plus six on either side. Duplicate send IDs are remembered for 256 requests.

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
elastic handle bounds and wheel response/settling.

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
