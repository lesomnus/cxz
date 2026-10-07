# Conversation statistics API

`cxz.SessionService/ConversationStats` is a read-only typed gRPC API, also
available through the existing Connect web transport. No CLI/TUI/web screen is
added. It does not start an agent or generate an auxiliary AI task.

Request:
- `session`: SessionRef (UUID, alias, or runtime ID), for one listed session.
- `project`: ProjectRef, for listed sessions in one project.
- Omit both to include all listed sessions on this server. Supplying both is invalid.
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
- Per-session canonical UUID, current alias, snapshot/first/last seq, and
  `complete`. Top-level complete means all session snapshots had every seq
  from 1 through their captured tail. It does NOT assert complete provider
  token reporting or account billing.

The supervisor journal is authoritative; SQLite is a projection and Manager
history may fall back to a partial cache when a project is offline. Retention
can remove older events. These are statistics of available retained history,
not a permanent lifetime ledger. Removed/unlisted sessions are excluded.

Each session uses its captured resource tail as a cutoff; this is not a global
transactional snapshot across projects. In-progress usage is only counted when
a turn ends, on that turn's UTC completion day. Events/bytes are counted on their
own event day. Stream chunks are counted as events, not unique model replies.

Scanning includes older retained events to establish cost baselines and detect
gaps. Work is bounded to 1000 sessions, 100000 events, and 64 MiB of text/payload.
Exceeding a bound returns ResourceExhausted, not a silently truncated total.
Narrow the session/project scope in that case; a narrower date interval does
not reduce baseline scan work. Runtime errors and cancellation propagate.
No schema migration or DB reset is needed.
