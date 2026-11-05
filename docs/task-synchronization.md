# Task list synchronization

The web Task catalog combines `TaskService.List` with a changes-only
`TaskService.Watch`. The project runtime owns Task storage; the Manager relays
both calls to that runtime. The WASM sandbox implements the same contract.

## Subscription barrier

The client opens Watch with `skip_snapshot: true`. After installing the
subscription, the server sends the response header `cxz-watch-ready: 1`, even
when the project has no Tasks or nothing changes. An empty control frame follows
the header so Connect transcoders flush it immediately. It contains no initial
rows and is not a snapshot. The Manager forwards the header and control frame.
Receiving the header is the barrier after which
the client may start List without leaving a gap between its baseline and Watch.

Watch emits current rows for committed changes, coalescing queued changes to
the same Task. An item without a value removes that ID from the query. In
changes-only mode, a removal may refer to a row that Watch has never emitted:
the row may have been obtained through List before it left a filter or was
erased. Clients ignore removals for unknown IDs. Removals are restricted to the
authorized project, including after erasure; other projects' IDs are not sent.

Leaving `skip_snapshot` unset retains the existing snapshot-then-changes behavior
for other clients. Payday's generated Watch defaults are unchanged.

## Publishing a complete list

While waiting for the barrier and all List pages, the catalog retains its
in-memory or IndexedDB snapshot. With no cache, it displays the loading state.
It buffers Watch changes and local mutation results separately, and collects
reverse List pages without publishing partial pages or replacing the cache.

After List completes, the catalog merges buffered changes into the collected
rows. Full rows are compared using the exact `date_updated` seconds and
nanoseconds so an older List page cannot overwrite a newer event or local
mutation. Removals delete their IDs. The catalog publishes and caches the
merged list once; the view sorts newest-created first with Done rows last.
Subsequent Watch batches update the live list and cache normally.

Watch is a live state stream, not a durable event log. On disconnect the catalog
retains the visible list, opens a new Watch, and repeats the barrier and full
List reconciliation. Canceling the last subscriber cancels both requests.

The readiness marker requires updated Manager and project runtime binaries.
An old or quiet server that cannot confirm readiness produces an update error
rather than leaving the page loading indefinitely. The catalog also bounds List
requests and rejects repeated pagination cursors.

## Regression coverage

The shared native/WASM contract tests cover empty-stream readiness, changes-only
delivery, filtered removals, project isolation and reverse pagination. Manager
tests exercise header forwarding and native erasure, and a Connect proxy test
verifies readiness before any changes. Catalog tests cover
multi-page buffering, version races, late disk hydration, cancellation and
reconnection. Storybook's `Resources/Tasks/PagedLoading` delays individual List
pages to demonstrate that rows appear together rather than being inserted into
the middle of a partially rendered table.
