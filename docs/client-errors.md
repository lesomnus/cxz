# Browser error center

The button immediately above Profile opens active errors and their history.
Its header has an Open errors page link to `/c/_/errors`; the standalone page
uses the same live store and status tabs. It follows the Projects/Tasks resource
layout: an aligned top toolbar with history controls and a compact Nox table
with kind/status, operation, target, message, occurrence count and last-seen
columns. Page rows expand their full message, timestamps and original diagnostic
references. The dialog uses ErrorTable variant="dialog": its first line is the
message and its second line has muted small kind-specific diagnostic values on
the left, then elapsed time and count on the right. Dismiss is its only row button. It is hidden at rest and fills the row height on
a right-side gradient overlay on hover or keyboard focus, without reserving a
button column. Touch devices keep it visible with content padding. Long messages and
diagnostic summaries ellipsize to retain two lines, with the full text in native titles.
History status is retained for assistive text and the row title. Details remain
available from the independent page via the header link.
Dismiss is a row action and Clear history is in the page toolbar. The dialog has
no footer, clear button or column-heading row; its body contains only the table.
Its scrollbar sits outside the right edge (ScrollArea uses a positive handle-center
offset to produce a negative CSS right). Card reveal/body wrappers allow the
rail through while the scroll viewport clips the rows. The page does
not embed or stretch the dialog card. Narrow screens move kind/status, operation/target and count/time into the
message cell while keeping details readable.
The current tab is carried into the page, and `/c/_/errors?tab=history` can be
bookmarked or reloaded directly.
Its symbol and border are red while there are active errors. Each record shows
the failure, operation, target when available, timestamps and repeat count.
Repeated failures of the same operation and reference share an active entry.
A failure after recovery or staleness starts a new episode.

All Connection RPC transports, including uploads and in-process connections,
report failures here. Terminal/login WebSocket failures, sign-out failures,
project runtime status errors and uncaught browser errors are included too.
RPC error text is suppressed in workspace pages, forms, chips and action menus.
Local validation, browser capability feedback, retry controls, warnings about
uncertain operation results and errors within the agent transcript remain in
their relevant views. Standalone components keep inline feedback when they are
used without the workspace error center.

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
page row to show additional metadata for cxz.SessionService/Transcript; the full
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
is 24px tall with no vertical padding and 4px side padding, matching its controls.
The Dismiss button is 44px wide and stretches through the entire row height on a
72px gradient overlay; it does not reserve a separate column.

While the dialog is open, newly reported episode IDs slide down from above over
240ms. Existing rows, count updates, initial opening and tab switches do not
replay entrances. Each row owns its animation until completion; the marker is
cleared afterward so later reordering cannot restart it. Reduced motion disables
the slide. The known-ID baseline includes both Active and History, stays bounded
to the store and resets on a fresh mount. Workspace/Errors/Rows/Incoming errors
provides Add error and Repeat latest error controls for this behavior.
