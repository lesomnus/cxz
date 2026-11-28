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
  WAL, index, attachment, memory, or provider transcript disk usage; for that
  see [what a session takes up](history.md#what-a-session-takes-up).
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
- Per-session canonical UUID and coverage. `complete` is always false: see
  below. It never asserts complete provider token reporting or account billing.

## Managed analytics

Every installed Manager collects usage into its private ClickHouse store, and
statistics are answered from it alone. See [managed telemetry](managed-telemetry.md) for the data path, settings,
security, deployment and retention.

- Metrics survive display-history pruning and are retained for the configured
  rolling period (90 days by default). Existing history is not backfilled.
- Each point carries the session's account when it was produced. A session
  whose account is switched keeps its earlier usage under the earlier account.
- Stream metadata is summed at durable turn/state boundaries; its events/bytes
  are attributed to the flush day. Native metrics use their export timestamp.
- Native cumulative counters include the baseline before the requested range;
  resets start at the new value. Delta points are summed. Identical retries are
  deduplicated. Input excludes cache; overlapping reasoning/total counters are
  not added again. Auxiliary usage is attributed to its actual selected account
  and parent project/session.
- Collection is best effort: old running agents, native export delay, bounded
  queues and retention can leave gaps. `complete` is always false. Session
  coverage carries canonical UUIDs; aliases and sequence fences are not part of
  the reply. A native token/cost series can establish that at
  least one turn reported a value when a Supervisor reporting counter is missing;
  reporting counts are therefore partial, not a billing reconciliation.
- Erased sessions are excluded even if an old queued batch arrives later.
  Stopping a session preserves usage until erasure or TTL.
- Queries are bounded to 15 seconds, 256 MiB query memory and 8 MiB of results.
  Database failures propagate as errors. Narrow scope/time when a query exceeds limits.

## When telemetry is unavailable

There is one source of usage: the managed telemetry store. Conversation history
is never scanned to recompute it, so two paths cannot disagree about what a
session or account spent.

A Manager without the store -- a source binary or `cxz manager serve` without the
packaged Collector, one still bringing the store up after starting, or one whose
store setup failed -- answers `Unavailable` for every scope. Usage produced
while the store could not accept it is missing, not reconstructed later. Scope
and argument validation (`InvalidArgument`, `NotFound`) still happen first.
