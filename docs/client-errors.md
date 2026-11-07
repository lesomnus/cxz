# Browser error center

The button immediately above Profile opens active errors and their history.
Its header has an Open errors page link to `/c/_/errors`; the standalone page
uses the same live store and status tabs. It follows the Projects/Tasks resource
layout: an aligned top toolbar with history controls and a compact Nox table
with status, message, occurrence count and last-seen columns. Each row expands
its full message, timestamps, operation/target and resource/request references.
Dismiss is a row action and Clear history is in the page toolbar. The page does
not embed or stretch the dialog card. Narrow screens move status/count/time
into the message row while keeping details readable.
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
