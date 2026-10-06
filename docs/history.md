# History and retention

Every input, reply, tool call, approval and state change is appended to a durable
per-session **journal**. The journal is the record — the transcript you read is a
rendering of it, and holds less.

Recovery after a crash or a container replacement is driven by the journal and the
manifests beside it, both fsynced. SQLite holds projections and caches built from
them; it is not the source of truth and not a backup.

## Response settings

New normalized assistant events include an optional `response` snapshot with
`model`, `effort`, their individual sources, and a native turn ID when available.
The snapshot is written with the response in the same durable journal batch;
changing session settings never relabels previous responses. Native payloads are
preserved separately.

Claude's response-reported model takes precedence over its applied settings.
Effort comes from the applied settings captured at input delivery. If those
settings are unavailable, explicitly requested values are stored as `requested`.
Codex snapshots the resolved model and effort actually sent in `turn/start`, also
marked `requested`; steering input keeps that turn's snapshot. This distinction
does not claim the provider confirmed a requested setting. Source values are
`response`, `settings`, or `requested`.

The web transcript displays known values as `model · effort` beside the agent
logo, with their sources in the hover description. Unknown fields are omitted.
Older records are left unchanged, without reconstructing settings from nearby
events or substituting the session's current values.

Assistant events still use the same `assistant` kind for intermediate and final
text. Codex's native `commentary` / `final_answer` phase is preserved in
`response.phase`. Claude's successful `result` closes the turn and identifies its
last main-agent response; nested subagent text is excluded. Failed/interrupted
turns and explicit commentary are not promoted to successful final answers.

A successful `turn_end` now holds an immutable `response.completion_json` summary
referencing the final assistant's journal sequence as a decimal string. This
links the footer without rewriting a previously committed assistant event. Native
turn-end payloads remain unchanged. Every intermediate and final event has
`time_ms`: the time cxz received and recorded it, rather than a claimed provider
generation timestamp.

The web final-response footer shows that timestamp, duration and known metrics,
with an icon-only Copy control on the right and a `copy` hover/focus popover.
Duration uses Claude's `duration_ms` or Codex's `turn.durationMs` when reported;
otherwise cxz measures from actual input delivery to turn completion, excluding
queued-input wait and including tools/approval wait. The duration hover describes
its source. Unknown duration/usage is omitted.

Claude result usage is scoped to the turn, with reported input/output/cache
counts, cost, API duration and API turns. Codex's `tokenUsage.last` is scoped to
the last model call and labeled accordingly; cumulative thread totals are not
displayed as turn usage. Cached/reasoning counts are subsets where the provider
defines them that way, and are not added to totals. Tool-call counts come from
observed journal events. Older records can use available successful turn-end
payloads and timestamps; partial history cannot invent missing values.

## One message

A single message from the agent is bounded at **16 MiB**. A reply is nothing like
that large; what reaches megabytes is an image a tool read, which comes back
base64-encoded on the same stream. The bound is there to stop a runaway stream
from filling the journal, never to trim an answer: anything under it is recorded
whole.

Past it, the bytes that did arrive are kept as the vendor record, the truncation
is reported as a diagnostic in the transcript, and the session stops reading. It
does not resume from the middle of a message it cannot parse.

## Two independent budgets

| | Where | Default | What it bounds |
|---|---|---|---|
| **Server retention** | Manager | 100 MiB per session | Journal on disk |
| **Client window** | TUI | 10 MiB or 200 turns | What one frontend holds in memory |

They are unrelated. Trimming the client window does not delete anything; exceeding
server retention does.

`Ctrl+.` exposes both. Server choices are 25/50/100/250/500 MiB or unlimited, with
a confirmation, and apply to running projects. Client choices are 5/10/20/50 MiB and
100/200/500/1000 turns. `cxz edit` sets the client window directly:

```jsonc
"history": { "window_mib": 10, "window_turns": 200 }
```

### Server retention

Once a session's journal passes the limit, a safe idle check removes its oldest
**completed turns**, down to roughly 80% of the budget, leaving a versioned control
checkpoint at the last removed position. Checks run after completed turns and once a
minute; a stopped session is checked when it resumes.

Removal is on turn boundaries, so a conversation never loses half an exchange.
Nothing is removed while a turn is in flight.

Retention trims a journal; it never unlinks one. Deleting a session does not
either. The one command that does is `cxz session purge` — see
[deleting a session](projects.md#deleting-a-session).

### The client window

The TUI loads the last 128 events and pages backwards as you scroll. The loaded
window is bounded by whichever of bytes or turns is reached first, and it slides:
scrolling back evicts the newest end, scrolling forward evicts the oldest.

Eviction stops at the turn you are looking at. When a fetched page cannot fit beside
it, the surplus page is given up rather than your place in the conversation, and the
next scroll fetches the rest. A single oversized turn is kept whole rather than
split.

Live output is the exception: it must keep the newest events whatever you are
reading.

Because the window slides, **loaded line numbers mean nothing** as a position. The
status row shows a journal coordinate instead, counted from the oldest retained
event, which only moves when the journal does.

## Compaction is not deletion

`/compact` asks the agent to summarise its own context — Claude's native compaction
or Codex's thread compaction. It requires an idle session.

The agent's context shrinks. **The cxz journal does not change.** They are different
things, and `/usage` still reads the whole journal.

## Reading it back

```sh
cxz session events ID [AFTER_SEQ]     # raw journal events
/usage                                 # tokens, cost, elapsed, from the journal
/details                               # the latest full tool input and result
```

The manager caches events as they stream and serves a page from SQLite when it holds
128 contiguous events; a gap or a short final page goes to the project runtime.
Journal reads are incremental — only complete appended records — so a session's
history loading does not replay the whole file.

## Backups

Back up the **state volumes** and your **workspace contents**.

Journals and manifests are authoritative for runtime recovery. The resource
database also holds project and session metadata and audit history that cannot be
rebuilt from journals — so back up all of it, not just transcripts.

**Raw journals contain your source and may contain secrets.** Do not publish them.
See [security](security.md).

## Reduced state the manager keeps

Some questions are about what is true now rather than what happened: the pending
approvals, the background tasks of a run, the model catalog it published. Each is
one record somewhere in a journal that may hold an entire conversation, so the
manager keeps them reduced as events are applied and answers from that — one
request, no scan.

A client that rebuilt them by reading the journal would pay for the conversation
to find a record, and over a remote link that cost is the conversation's size,
not the record's. `cxz session get`, the background overlay and the model
selector all read reduced state for this reason.
