# Web conversation history

The native journal remains the source of truth. The web transcript reads a
separate projection so scrolling does not require downloading provider protocol
traffic or tool output. Live `Events` still delivers every native event.

`SessionService.Transcript` counts readable rows. Without a cursor it returns the
latest page; exclusive `before_seq` and `after_seq` bounds page in either
direction. These bounds are mutually exclusive. `snapshot_seq` fences both the
membership and state of rows while new records arrive. The reply includes retained
older/newer flags, the preceding user input for the pinned preview, and recent
model/quota/context metadata. Defaults and limits live in
`internal/transcripthistory/history.go` and `ts/src/features/session/model/session-history.ts`.

A tool's row stays at its original call sequence. Output, results and correlated
approval decisions update that row's status rather than inserting more transcript
rows. Matching successful turn completions attach their metrics to the original
assistant response. Commentary, questions, failures, interrupted turns and
unmatched completions remain readable. Model/effort snapshots and event times
retain their original provenance.

File operations carry structured paths, actions and recorded counts in
`ToolSummary.files`. A Codex completion can fill an initially empty change list
without moving its original row. The web shows each file with its own operation
and status, matching the TUI's display parser in `internal/toolview`: actual diff
hunks use added/removed counts, Claude Write uses supplied content lines, and
Claude Edit uses replacement spans (including the per-match qualifier).
Unreported counts stay unknown. Long file lists indicate omitted entries and
retain the complete native records for on-demand details. The browser derives
the same presentation from native live events and legacy History pages.
Summary API support requires updated runtime and forwarding components; updating
only Vite does not add file metadata to an older server's summary replies.

`SessionService.EventDetails` returns native records for a selected row. A tool
request includes its call, output, result and approval lifecycle, matched within
its run. Retention can make a previously visible row unavailable. Opening a
preview loads these records; closing or replacing it aborts an outstanding read.
Historical tool bodies do not fill the transcript cache.

The project runtime lazily maintains disposable SQLite indexes over its original
event table. Row versions preserve snapshot consistency. Ordinary updates index
only a suffix; retention, cache backfills and outdated projection formats rebuild
the affected session from retained original records. Upgrading the summary
format does not require recreating a project. A host
can use its retained cache while a project is unavailable. The browser switches
to native `History` paging only when the selected runtime reports
`Unimplemented`. Its native stream resumes at the summary's snapshot fence, not
at the last visible row. Reconnects replay native updates, including results whose
historical row is anchored earlier than the cursor.

Browser cache size and DOM size are separate. The cache holds multiple pages;
virtual messages mount the visible range and an overscan buffer. Reading anchors
survive prepending history and resize. Protocol-only live records update the
stream cursor and metadata without rebuilding the transcript layout. Model
choices use the dedicated `Models` projection rather than scanning history.

The browser requests larger pages and warms history several viewports before
the loaded edge. Its authenticated in-memory cache can hold more pages than the
virtualized DOM. Prefetching follows the direction of travel to avoid fetching
and evicting opposite edges. Page, cache and prefetch limits remain in code.
Native `History` accepts a bounded `limit`; omitting it preserves the original
page size for existing TUI clients. Older runtimes may ignore this field, so the
browser continues paging until the requested sequence window is complete.

If scrolling uses `History` after `Transcript` returns `Unimplemented`, one of
the gateway, Manager or project runtime has not been updated. Updating only Vite
does not upgrade these components. On the cxz host, `cxz use @edge` switches all
managed components while retaining project containers and data; this interrupts
active agent work. Reload the browser afterward to repeat capability detection.
Occasional native `History` reads during reconnection are expected even with a
supported summary API.

`internal/transcripthistory/history_test.go` covers snapshot paging, lazy details,
identity isolation, completion metadata, backfills, retention and cancellation.
`internal/server/transcript_test.go` verifies the native journal path. Optional
`CXZ_REPLAY_EVENTS` snapshots also drive the HTTP browser fixture and
`ts/e2e/history.spec.mjs`; private recorded content is never checked into the
repository. The WASM sandbox uses the same reducer with an in-memory page adapter.
