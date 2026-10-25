# Browser client logs

**Settings → Logs** (`/setting/logs`) shows diagnostics observed by this
browser. The sign-in page also links there: inspecting retained logs needs no
working Manager, network connection, or authenticated browser session. These
are client diagnostics, not Manager, project runtime, agent, or conversation
journals.

The page uses shared Nox controls, Table/ScrollArea and DateTime formatting. It
shows the newest 100 matching records per page, with keyboard-accessible
expandable JSON details. Filter by level/category, exact connection URL or
public project/session UUID; free-text search matches the entire structured
record, case-insensitively. Live updates can be paused; paging pauses updates.
Refresh explicitly reads storage. Download exports every matching retained
record as JSONL, independently of the displayed page. Clear deletes retained
logs for this origin, including records from other tabs.

## Collection

`ts/src/shared/logs` provides a shared client logger and Connect interceptor.
Records contain an ID, wall-clock timestamp, tab ID, UI build revision, level,
category, message, and available connection/resource/request identifiers.

- Uncaught JavaScript errors and unhandled promise rejections.
- React caught, uncaught and recoverable errors, and voice-input failures.
- RPC failures with method, error code, bounded error text and elapsed time.
- Stream connection/end, failures during **response iteration**, retry attempts,
  and recovery, with elapsed disconnection time.
- Plaintext RPC WebSocket failures retain their close code, reason and clean
  close flag through the ConnectError cause chain. Local cancellation is not
  logged as a failure. HTTPS uses the same RPC interceptor over Connect fetch.

Primary references use public UUIDs. Legacy session handles/aliases are resolved
through the already loaded resource inventory when available; the logger does
not perform extra RPCs to discover identities. Request IDs correlate one call's
connection and failure; method/resource identity correlates its retries.

Successful unary calls, individual stream frames, conversation/request/response
bodies, headers, cookies, audio and recognized text are not collected. This is
not a console/network recorder. Error strings/stacks are bounded and scrubbed
for credential-like text and URL credentials/query/fragment values; this is
best-effort redaction of arbitrary third-party error messages. Only explicitly
selected diagnostic metadata is recorded.

## Storage and rotation

The default backend writes JSONL in OPFS directory `cxz-client-logs-v1` through a
dedicated Worker. It batches for 500 ms, with at most 256 buffered records while
storage is busy. If overflow drops records, a warning records the dropped count.
Reads/exports first flush this tab's queued entries. Visibility/pagehide request
a flush, but browser termination can still lose an uncommitted batch.

Files rotate at **2 MiB**; all retained files together are limited to **32 MiB**.
Rotation uses UTF-8 byte counts and removes the oldest segments, without an age
limit. A shared Web Lock serializes appends, reads, pruning and deletion across
tabs. Handles are closed between operations, and no shared mutable manifest is
needed. On reopen, malformed final JSONL lines are ignored; an incomplete final
line is truncated before appending. The limit includes files from closed tabs.

BroadcastChannel notifies other open log pages after committed writes/deletion;
there is no storage polling timer. OPFS/Web Locks/Workers being unavailable,
storage denial/quota errors, or a failed Worker switches to a **2 MiB bounded
memory store**. The page explicitly displays Memory only and the effective
capacity. Logging failure must not fail RPCs or recursively produce diagnostics.

Logs belong to this origin and browser profile, independently of connections.
Changing scheme/host/port or browser profile gives a different log store.
They survive ordinary reloads/restarts, but browser storage eviction, site-data
clearing and private-browsing lifetimes apply. Signing out does not erase logs.
The backend is separate from settings.json and does not request persistent
storage permission. It has no Manager RPC, schema migration or server storage.

## Verification

Unit tests cover byte rotation/reopen, simultaneous writers, torn writes,
bounded pages/exports, redaction, memory fallback/backpressure, real generated
RPC clients, response-iteration errors, cancellation and recovery. Storybook's
**Settings/Logs** stories provide persistent, memory-only and empty views,
manual failure injection and a rotation burst. Browser tests exercise actual
Worker/OPFS persistence, multiple tabs, filters/export/clear, JS errors,
Worker-denied fallback and 2 MiB segment rotation. The existing routing suite
also exercises the new settings route.
