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
`internal/transcripthistory/history.go` and `ts/src/session-history.ts`.

A tool's row stays at its original call sequence. Output, results and correlated
approval decisions update that row's status rather than inserting more transcript
rows. Matching successful turn completions attach their metrics to the original
assistant response. Commentary, questions, failures, interrupted turns and
unmatched completions remain readable. Model/effort snapshots and event times
retain their original provenance.

`SessionService.EventDetails` returns native records for a selected row. A tool
request includes its call, output, result and approval lifecycle, matched within
its run. Retention can make a previously visible row unavailable. Opening a
preview loads these records; closing or replacing it aborts an outstanding read.
Historical tool bodies do not fill the transcript cache.

The project runtime lazily maintains disposable SQLite indexes over its original
event table. Row versions preserve snapshot consistency. Ordinary updates index
only a suffix; retention and cache backfills rebuild the affected session. A host
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

`internal/transcripthistory/history_test.go` covers snapshot paging, lazy details,
identity isolation, completion metadata, backfills, retention and cancellation.
`internal/server/transcript_test.go` verifies the native journal path. Optional
`CXZ_REPLAY_EVENTS` snapshots also drive the HTTP browser fixture and
`ts/e2e/history.spec.mjs`; private recorded content is never checked into the
repository. The WASM sandbox uses the same reducer with an in-memory page adapter.
