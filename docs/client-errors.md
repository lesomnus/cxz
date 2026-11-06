# Browser error center

The button immediately above Profile opens active errors and their history.
Its symbol and border are red while there are active errors. Each record shows
the failure, operation, target when available, timestamps and repeat count.
Repeated failures of the same operation and reference share an active entry.
A failure after recovery starts a new episode.

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

## Storage

Up to 200 summaries are stored under `cxz.client-errors.v1` in this browser's
localStorage, shared across connections with the connection retained on each
record. Active entries are retained ahead of older history. This is a bounded
diagnostic list, not an audit log; older entries can be evicted at the limit.
Resolved and dismissed entries survive reloads. **Clear history** removes only
closed entries. Browser storage failure falls back to the current tab's memory.

This list is separate from Settings → Logs, whose detailed client logs rotate
in OPFS within a 32 MiB budget. Error summaries contain diagnostic messages and
resource references, never serialized request bodies, response bodies or login
output. The existing diagnostic redactor strips recognized credentials and URL
credentials/query strings before storage. Error messages themselves should
never contain secrets.

`Workspace/Errors → Recovery` in Storybook exercises the real sidebar and RPC
transport, including delayed/canceled retries, target isolation, stream recovery,
editable fields and persistent history. Its data uses an isolated preview key.
