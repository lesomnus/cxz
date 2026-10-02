# History and retention

Every input, reply, tool call, approval and state change is appended to a durable
per-session **journal**. The journal is the record — the transcript you read is a
rendering of it, and holds less.

Recovery after a crash or a container replacement is driven by the journal and the
manifests beside it, both fsynced. SQLite holds projections and caches built from
them; it is not the source of truth and not a backup.

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
