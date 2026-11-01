# Web rendering and development diagnostics

The conversation retains a bounded event history while mounting only the visible
messages and nearby rows. Pixel scrolling must reuse those rows until the visible
window, records or renderer changes. Pinned input content should also remain stable
while its container moves. Rail markers use the same windowing approach: retain
nearby markers and outgoing animation positions, without mounting markers for
the entire history. These changes must preserve the existing rail mapping,
animation phase, reading anchor and follow behavior.

Dragging the conversation handle tracks directly through the middle half of its
travel. An outward gesture in the upper/lower quarters progressively adds
upward/downward auto-scroll and elastic pull, continuing beyond the literal rail
ends. Merely grabbing an end-positioned thumb or moving inward never activates
the opposing pull; reversing inward cancels it even within that quarter. The local mapping
shifts only by achieved auto-scroll, preserving pointer tracking and marker phase
without jumping to a window boundary. Canceling an active pull rebases at the
current reading position and stops the added velocity.
Each new outward pull uses its own starting point: the quarter boundary when
entering from the center, or the last inactive pointer position when already
inside that quarter. Regrabbing an end therefore starts with zero pressure,
instead of instantly applying the entire distance from the quarter boundary.
At a loaded-history edge, an active pull also requests the page in that direction
while native scrolling is clamped. Waiting for a nonzero scroll delta would
otherwise prevent that first page from ever being requested.

React owns marker membership and styling; `ScrollMarkers` reuses its elements
while `Transcript` paints their changing positions through refs. Range rebasing
reads the last painted inline positions, avoiding computed-style reads. The
ordered marker model is built when records, content origin or search matches
change; a binary window lookup visits only the current rail band and explicitly
retained outgoing markers, even for a jump between distant ranges. Row-height and
reading-knot corrections reuse logical marker coordinates.

`VirtualMessages` is memoized with a stable mapping callback. Scrollbar-only
animation frames must not rebuild message rows. While reading older messages,
`useTranscriptMotion` keeps its navigation baseline without measuring message
positions or column width for a live-arrival effect. Those measurements remain
available while following new responses. Equal viewport and pinned-prompt values
must not schedule redundant updates.

Mounted rows keep a stable ref and ResizeObserver subscription while the visible
window moves. The observer reuses delivered border-box dimensions and content
padding instead of reading each row's client rectangle again. Cached row heights
and reading knots are pruned when the event window changes, rather than on every
height correction.

The real conversation also paints top/bottom fades during scrolling and elastic
pulls. Keep their changing CSS variables on the fade elements: placing inherited
variables on the transcript area invalidates every message/card subtree's style.
Read composer geometry before writing either fade. Floating question cards
observe the bottom fade directly and reuse its inline height without forcing
computed-style resolution on the common ancestor. Component-only previews omit
this paint path; the real-app performance fixture verifies fade behavior and
that scroll values do not inherit through messages.

Retained virtual rows memoize their rendered content separately from their top
position, so measuring a neighbor does not recreate their cards. Historical
reading also skips arrival-animation geometry capture entirely; resuming live
follow establishes a new baseline. DOM rows and observed heights remain bounded
by the existing virtual/event windows, and event replacement still updates a row.

Markdown remounts reuse sanitized HTML with separate file-link modes. This
in-memory LRU retains at most 256 entries and a 2 MiB UTF-16 input/output text
budget; oversized results bypass it. It retains no DOM, interactive code controls
or selected language. Plain Markdown avoids an empty code-portal update, and
uncolored code tokens remain text nodes. Normalized questions are weakly cached
by immutable payload and decoding mode. Nested ScrollArea/ScrollFade variables
belong to their leaf overlays too; closed ValueMenus do not measure popup width.
Code highlighting resolves its palette without resolving editor fonts or reading
the document's computed styles during card creation.

React development builds record changed props in Performance Tracks. Those
diagnostics have CPU and memory costs and can fail while cloning large details;
they are not conversation events or server responses. See
[React Performance Tracks](https://react.dev/reference/dev-tools/react-performance-tracks)
and the [upstream serialization issue](https://github.com/react/react/issues/37330).

`src/shared/browser/react-performance.ts` bounds React's retained timing entries
and changed-prop details in the app, WASM sandbox and development Storybook.
Only recognized React tracks are affected. App-owned measures and errors remain
intact; labels shared with other instrumentation are not cleared. A React-only
`DataCloneError` retries the timing without changed props. This is a diagnostic
budget, not a replacement for fixing application allocations or React's internal
prop traversal. Production builds do not install this wrapper. Limits live in
the implementation, and HMR removes the installed wrapper on disposal.

Transcript, virtual-row, search and event-detail components receive protocol
snapshots through stable readers from `features/session/model/event-source.ts`. React's changed-prop
inspection otherwise enumerates large `Uint8Array` payloads before the timing
wrapper can limit their retained details. The reader preserves the original
event, payload and serialization; it does not strip metadata or disable React
diagnostics. Payload decoding is shared by immutable buffer identity through a
weak cache. Replacing the buffer invalidates the decoded snapshot.

When a fetched page exceeds the conversation byte budget, historical reading
retains a contiguous window containing the saved reading anchor and nearby
context. Pagination reads the connection cache's current snapshot, including
updates not yet delivered to React, rather than a previous render's event list.

From `ts`, run `npm run test:performance` for the development-build regression
fixture. It exercises repeated scrolling over long history, checks bounded rows,
markers and timing entries, then loads additional records. A full-capacity
fixture crosses rail ranges and verifies that historical reading performs no
message-position reads for arrival animation. A separate RPC pagination fixture
opens the real development app with large approval metadata and verifies frame
progress, bounded request counts and reading-anchor retention as pages arrive.
Private recorded pages can be replayed locally but must not be committed. Other
scroll and work-dialog behavior remains covered by the existing Storybook tests. Compare
timings with other builds and tests idle; parallel compilation distorts browser
frame times. Synthetic measurements do not establish the exact cause of an OOM
in a user's browser without its stack and memory profile.

Use `/test-support/transcript-performance.html?count=8192&content=mixed` for many
short replies and resolved question cards. The scroll regressions verify that a
retained row is not observed again and that delivered resize measurements avoid
per-row rectangle reads. Storybook's Conversation / EventCards / Long Mixed
Conversation provides the same mix for manual handle-drag checks.
`content=complex` adds Markdown tables, fenced code and long question bodies.
The regressions also cover row-content reuse during height correction, snapshot
replacement, sanitizer/link-mode isolation, and Markdown cache eviction/bypass.
