# Conversation statistics API

`cxz.SessionService/ConversationStats` is a read-only typed gRPC API, also
available through the existing Connect web transport. No CLI/TUI/web screen is
added. It does not start an agent or generate an auxiliary AI task.

Request:
- `session`: SessionRef (UUID, alias, or runtime ID), for one session resolved by the API.
- `project`: ProjectRef, for usage associated with one project.
- `account`: AccountRef, for one account.
- Omit scope to include all retained usage on this server. Supply at most one scope.
- `from_ms`, `to_ms`: UTC half-open interval. End defaults to now; start defaults
  to 30 days before end. Maximum interval is 366 days. Negative/empty/reversed
  intervals are invalid. A zero value means default.

Response:
- `total` and ascending `days` (UTC YYYY-MM-DD; only days with events).
- Event/input-message/assistant-event/finished-turn counts.
- `conversation_bytes`: UTF-8 text bytes of input and assistant events.
- `event_content_bytes`: text plus payload bytes of all journal events. Raw and
  derived events may contain duplicate information. This is not JSONL, SQLite,
  WAL, index, attachment, memory, or provider transcript disk usage.
- Input/output/cache-read/cache-write tokens. Input excludes cache tokens:
  Codex's inclusive input count is normalized by subtracting cached input.
  Cache counters are separate and should not be added to inclusive provider
  input counts outside this API.
- `token_reported_turns`: completed turns with at least one recognized token
  counter. Missing individual counters remain zero; this is not a claim that all
  counters were reported.
- `reported_cost_usd` and `cost_reported_turns`: provider-reported amounts, not
  account billing. Cumulative cost is differenced per run (including baselines
  before the requested period); direct turn costs are summed. After a history
  gap, the first cumulative report establishes a baseline without charging its
  potentially lost history. A cumulative counter reset starts at its new value.
  Zero cost with zero reported turns means unknown, not free. No model price
  table or unreported cost estimate is invented.
- Per-session canonical UUID and coverage. The history path also supplies
  current alias and snapshot/first/last seq. `complete` describes backend
  coverage as detailed below; it does not assert complete provider token
  reporting or account billing.

## Managed analytics

An installed Manager with telemetry enabled queries its private ClickHouse
store. See [managed telemetry](managed-telemetry.md) for the data path, settings,
security, deployment and retention.

- Metrics survive display-history pruning and are retained for the configured
  rolling period (90 days by default). Existing history is not backfilled.
- Stream metadata is summed at durable turn/state boundaries; its events/bytes
  are attributed to the flush day. Native metrics use their export timestamp.
- Native cumulative counters include the baseline before the requested range;
  resets start at the new value. Delta points are summed. Identical retries are
  deduplicated. Input excludes cache; overlapping reasoning/total counters are
  not added again. Auxiliary usage is attributed to its actual selected account
  and parent project/session.
- Collection is best effort: old running agents, native export delay, bounded
  queues and retention can leave gaps. `complete` is always false. Session
  coverage carries canonical UUIDs; retained-history aliases and sequence fences
  are unavailable in this path. A native token/cost series can establish that at
  least one turn reported a value when a Supervisor reporting counter is missing;
  reporting counts are therefore partial, not a billing reconciliation.
- Permanently purged sessions are excluded even if an old queued batch arrives
  later. Soft deletion/archiving preserves usage until permanent purge or TTL.
- Queries are bounded to 15 seconds, 256 MiB query memory and 8 MiB of results.
  Database failures propagate instead of silently replacing analytics with a
  smaller history-derived total. Narrow scope/time when a query exceeds limits.

## Retained-history fallback

Telemetry-disabled installations, standalone runtimes and source binaries
without the packaged Collector retain the bounded history-scan path. Statistics
cover available retained history; they are not a permanent lifetime ledger.
Session snapshots fence each scan. All sequences through each snapshot tail are
required for `complete=true`, which still does not prove provider token or
billing completeness. Deleted sessions are excluded.

In-progress usage is counted when a turn ends, on that turn's UTC completion
day. Events/bytes use their own event day. Stream chunks count as events rather
than unique model replies. Older retained events establish cost baselines and
detect gaps. Scans are limited to 1000 sessions, 100000 events and 64 MiB of
text/payload; exceeding a bound returns ResourceExhausted. Narrower dates do not
reduce baseline scan work. Runtime errors/cancellation propagate. No existing
conversation DB reset is needed for either path.
