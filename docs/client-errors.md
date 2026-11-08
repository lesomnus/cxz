# Browser error center

The button immediately above Profile opens active errors and their history.
Its header has an Open errors page link to `/c/_/errors`; the standalone page
uses the same live store, showing every status together with Active first. It follows the Projects/Tasks resource
layout: an aligned top toolbar with history controls and a compact Nox table
with status icon, centered Kind, an unlabeled occurrence count, Operation,
Diagnostics, Target, Message, Last seen and actions columns. Operation contains
the readable work name; Diagnostics contains the recorded RPC/HTTP/runtime/client
evidence. Identical values appear once. The Count heading retains an accessible
name while remaining visually empty. The resolved circle is 7px across.
Row spacing and dividers share the Projects list stylesheet. Nox Table and
normal list rows stay transparent. ResourcePage owns the semantic
`--resource-page-background` surface and supplies the same color to nested
ScrollAreas; Projects, Tasks, Memory, Search, Accounts and Errors reuse it.
Page components do not repeat background colors. Nested section scroll areas
can still explicitly use their section surface.
Active uses a plain orange triangle, Resolved a small green circle,
Dismissed a gray square and Stale a gray triangle.
Clicking a page row, or focusing it and pressing Enter/Space, opens a Nox modal
with the full message, timestamps and original diagnostic references. Escape and
Close return focus to the row. No separate Details button is shown. Dismiss
handles its own click without opening the modal, and selecting row text does
not open it. Kind (including the heading) and Count are explicitly centered
through Nox Table alignment.
The page Dismiss button retains its rendered 30px width and is 30×30px with a
12px close icon. Its absolute positioning uses the actual action-cell height
through a size container, keeping top/bottom/right gaps equal even on wrapped
rows. The sidebar Dismiss overlay keeps its separate layout. The dialog uses ErrorTable variant="dialog": its first line is the
message and its second line has muted small kind-specific diagnostic values on
the left, then elapsed time and count on the right. Dismiss is its only row button. It is hidden at rest and fills the row height on
a right-side gradient overlay on hover or keyboard focus, without reserving a
button column. Touch devices keep it visible with content padding. Long messages and
diagnostic summaries ellipsize to retain two lines, with the full text in native titles.
History status is retained for assistive text and the row title. Details remain
available from the independent page via the header link.
Dismiss is a row action. The page toolbar places Clear history first and
Dismiss all at the far right, both with 12px icons. The shared ResourceToolbar
action group owns their 48×28px shape and icon sizing, also used by Tasks and
Accounts; no Errors-specific toolbar CSS is needed. Dismiss all changes only
currently Active entries to Dismissed in one store update; existing history is
preserved and an already expired episode remains Stale. It is disabled when
there are no Active entries. Clear history removes closed episodes and is
disabled when history is empty. Pending closures are flushed before clearing
closed IDs so delayed storage merges cannot restore their older Active copies. The dialog has
no footer, clear button or column-heading row; its body contains only the table.
Its scrollbar sits outside the right edge (ScrollArea uses a positive handle-center
offset to produce a negative CSS right). Card reveal/body wrappers allow the
rail through while the scroll viewport clips the rows. The page does
not embed or stretch the dialog card. Narrow screens keep the status icon and move kind, operation/target and count/time into the
message cell. Details remains a viewport-limited modal.
Only the sidebar uses Active/History tabs. Its page link always opens `/c/_/errors`;
legacy `?tab=history` links also show the unified list.
Its plain triangle turns red while there are active alerting errors. The button
and icon wrapper stay transparent, including hover/open states, with no exclamation
mark or ink effect. Each record shows
the failure, operation, target when available, timestamps and repeat count.
Repeated failures of the same operation and reference share an active entry.
A failure after recovery or staleness starts a new episode.

All Connection RPC transports, including uploads and in-process connections,
report failures here. Terminal/login WebSocket failures, sign-out failures,
project runtime status errors and uncaught browser errors are included too.
Captured RPC error text is normally suppressed in workspace pages, forms, chips
and action menus. Attended session/conversation-loading errors remain inline.
Local validation, browser capability feedback, retry controls, warnings about
uncertain operation results and errors within the agent transcript remain in
their relevant views. Standalone components keep inline feedback when they are
used without the workspace error center.

## Notification policy

Error context accepts `notify: false`: retain the episode and its full lifecycle,
but exclude it from the sidebar error button's alert color/count. Missing values
notify by default; persisted values must be actual booleans. The page and opened
sidebar panel still list all records, including silent Active entries. Details
shows whether an episode is included in alerts.

```ts
errors.report(key, error, { notify: false });
// For a captured Connect RPC, an explicit call attribute overrides policy:
c.sessions.get(request, errorNotificationOptions(false));
```

`errorNotificationOptions` comes from `shared/errors/error-notification.ts`.
An initial conversation read owned by a waiting view is silent, including its
retries after leaving that view. Session resolution also uses the explicit call
attribute. Session Get/Transcript/History and supporting read calls use the
connection's loading policy, covering duplicate metadata queries too. Once the
conversation has loaded, visible history pagination, message jumps and preceding
input lookups explicitly remain silent; they report failures inline instead.
Optional account-quota and project-activity transcript reads also pass
`notify: false`, so scheduled refreshes or inventory updates cannot repeatedly
raise alerts for an unavailable session. Their diagnostics are still retained.
Unrelated operations and live/background stream failures notify normally.
This is contextual, not a blanket exclusion of a service or method.

Attended read messages use `ErrorNotice local` so capture by the center cannot
hide the failure behind a loading screen. Other captured API feedback keeps the
existing deduplication behavior. Notification policy never marks a failure
resolved, drops its count, removes history, or changes its recovery identity.

## Recovery

For unary RPCs a successful retry of the same method and reference resolves the
error. A stream resolves on the first valid response, including a heartbeat,
rather than when it opens. Client-detected stream stalls also recover when
valid data resumes. Project runtime errors recover when a later status clears
the error. Terminal readiness, completed provider login and successful sign-out
resolve their corresponding operations.

Starting a retry, clearing a temporary error label, switching views, or canceling
a request does not establish recovery. An error for another target or method
does not resolve the original. Recovery is an observation of that operation;
it does not certify the entire server is healthy or guarantee an earlier write
was applied. Uncaught JavaScript errors have no reliable automatic recovery
signal: use **Dismiss**, which records a distinct dismissed status.

## Stale history

An active record becomes **Stale** one hour after its last failure was observed.
A repeat occurrence renews this deadline; opening a panel or retrying without a
new failure does not. Stale means no recent observation, not confirmed recovery.
It leaves the active count and appears in History with the last failure time,
stale timestamp and occurrence count. A later failure starts a new active record
while retaining the previous stale episode.

The sidebar's store subscription keeps expiration running even while the dialog
is closed. Restoring persisted records and subscribing after a suspended view
also expire old records, so reopening the browser does not reset the hour.
Resolved, Dismissed and Stale remain distinct statuses. Clear history removes
all three while retaining current active failures.

## Storage

Up to 200 summaries are stored under `cxz.client-errors.v1` in this browser's
localStorage, shared across connections with the connection retained on each
record. Active entries are retained ahead of older history. This is a bounded
diagnostic list, not an audit log; older entries can be evicted at the limit.
Resolved, dismissed and stale entries survive reloads. **Clear history** removes only
closed entries. Browser storage failure falls back to the current tab's memory.

This list is separate from Settings → Logs, whose detailed client logs rotate
in OPFS within a 32 MiB budget. Error summaries contain diagnostic messages and
resource references, never serialized request bodies, response bodies or login
output. The existing diagnostic redactor strips recognized credentials and URL
credentials/query strings before storage. Error messages themselves should
never contain secrets.

`Workspace/Errors → Recovery` in Storybook exercises the real sidebar and RPC
transport, including delayed/canceled retries, target isolation, stream recovery,
editable fields and persistent history. `Workspace/Errors → Page` opens the
independent view directly. The previews use an isolated storage key.

## Structured presentation

`ClientError.origin` is an allowlisted, discriminated union in `error-origin.ts`:

- `rpc`: service and method (the Connect error code remains on the record).
- `http`: HTTP method, path without query/fragment, and response status when known.
- `connection`: terminal or provider-login purpose, transport and optional close code.
- `runtime`: project runtime, inventory or conversation stream health operation.
- `client`: script exception, unhandled rejection or explicit client operation.

`subject` carries a resource kind with optional name, alias, public ID and runtime
ID. RPC instrumentation extracts only known reference fields; Connection enriches
project/session references from its current inventory. Names are captured when a
failure is reported. No extra lookup RPC is sent to describe an error. Name or
alias is preferred in the row, then a short identifier, with `—` for no target.
Kind-specific values identify the failure directly in each row. Full additional
references and request IDs remain available in Details.

`error-presentation.ts` exhaustively maps kinds to common columns and details.
Translated operation names supplement the diagnostic values; they must never
replace or hide them. RPC rows show the full service/method even without a label
mapping. HTTP rows show method/path/status, including unfamiliar endpoints. Error message text is never parsed
to infer a kind or resource. Add new operations to these adapters and their locale
messages rather than branching in either view.

`source` is the older diagnostic provenance field, retained for compatibility and
transport/health-check deduplication; render `origin.kind` as the error category.
Records stored before this schema are upgraded from source/method during restore.
Invalid or unknown metadata falls back without dropping the error. New fields
are validated, redacted and copied individually on report and restore, so arbitrary
objects or credentials cannot be persisted as additional properties. Stable
operation/reference keys and the existing recovery/stale lifecycle are unchanged.

`Workspace/Errors/Rows` catalogs every registered operation label: generic RPC
method mappings, session-specific operations, project editor/terminal, known HTTP
paths and fallback, both connection purposes/transports, all runtime health
operations and all client operations. All shows the 40 examples together; RPC,
HTTP, Connection, Runtime and Client provide focused tables; Dialog uses the
headerless two-line error panel with an outside scrollbar. RPC details opens a
page modal to show additional metadata for cxz.SessionService/Transcript; the full
RPC name is also visible in every matching summary row. Dismiss is usable in all fixtures
and details are usable in the page variants. The fixtures are nonpersistent. The fixtures enumerate the actual
operation registries so newly registered RPC/runtime/client labels also appear.


Kind adapters supply a shared visible diagnostic value to both page and dialog:

| Kind | Visible values, when recorded |
| --- | --- |
| RPC | Full service/method, RPC code and target |
| HTTP | Method, path, response status and target |
| Connection | Purpose, WebSocket/RPC transport, close code and target |
| Runtime | Observed operation/RPC, container state, failing step, container name or short ID, target |
| Client | Exception name, source filename and line/column, client operation |

The dialog keeps these on its second line before time/count and has no Details
button. Page rows show the diagnostic values with the translated operation label
as supplementary text. Never require opening Details to learn the RPC name,
HTTP endpoint or other identifying diagnostic evidence. Missing values are
omitted rather than fabricated. Old records remain readable with their available
metadata. Runtime/client fields are allowlisted and redacted on report/restore;
request bodies, socket query strings and arbitrary objects remain excluded.

RPC dialog, HTTP dialog, Connection dialog, Runtime dialog and Client dialog
stories preview these values directly. Catalog contracts verify both page and
dialog rows expose each kind's evidence without expanding Details.


RPC target cells and dialog summaries omit repeated resource kind prefixes. Real
names/aliases are retained; ID-only references show exactly the first 7 characters,
including legacy stored references. Full identifiers remain in page Details.
Error row ages use Nox RelativeTime compact: English examples include "5m ago"
and "in 2h"; other locales use their narrow relative units. The public component
and formatter keep their existing long default. Nox Components/RelativeTime has
fixed-clock compact, future, Korean and per-unit examples.


The sidebar dialog is 420px wide (viewport-limited on small screens). Its heading
uses the composer menu surface's `--ui-control-inset` (2px) on every side. Its
close button retains the standard elongated toolbar shape (48×24px). Its top,
bottom and right inset is equal; do not make it square to achieve equal gaps.
The button radius is the actual outer card radius minus border thickness and
control inset, matching the Task card header's concentric-corner rule. Heading
height remains 28px with the current token.
The Dismiss button is 44px wide inside a 2px-padded gradient overlay, with an 8px
inner radius (section radius 10px minus inset 2px). The overlay width derives from
button 44px + fade 28px + both side insets; it reserves no separate column. The
header title has an extra four ui-space units (16px) left inset without moving the controls.

In the page and open dialog, newly reported episode IDs slide down from above over
240ms. Existing rows, count updates, initial opening and tab switches do not
replay entrances. Each row owns its animation until completion; the marker is
cleared afterward so later reordering cannot restart it. Reduced motion disables
the slide. The known-ID baseline includes both Active and History, stays bounded
to the store and resets on a fresh mount. Workspace/Errors/Rows/Incoming errors
and Incoming page provide Add error and Repeat latest error controls for this behavior.


Active/History body views use Nox SlidingPages inside the shared ScrollArea.
The indicator and body share --error-tabs-duration (140ms) and
--error-tabs-easing (ease-out); the latter drive the SlidingPages timing tokens.
Both directions slide and respect reduced motion. The outgoing page remains
mounted for the transition and is inert/aria-hidden immediately; focus stays in
external navigation. The outside scrollbar is a sibling of the clipped pages and
stays usable. Inactive tables skip new-entry entrances while their known-ID
baseline remains current, so switching tabs does not replay row animations.
