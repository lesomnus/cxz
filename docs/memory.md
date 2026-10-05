# Memory

Two different things are called memory. This page covers both.

## Retained agent data

Select a session in the project list and press `m`, or use `/memory`, to browse
what an agent has kept: its configuration directory, its own memory files and its
conversation history.

This works **after the project container has stopped**. The data lives in state
volumes, not in the container.

Press `c` to copy selected data into another session. That is how you move an
agent's accumulated context to a new session without replaying a conversation —
useful when a session's history has grown unwieldy but its notes are worth keeping.

Copying never moves credentials.

## Shared project memory

Sessions in a project can publish Markdown documents that other sessions in the
same project can read. This is exposed to agents as an MCP server, so an agent can
list, search, read and update it during a conversation.

Each session owns exactly one memory and can only write its own. Reads are open
across the project, which is the point: a session that solved something can leave
the reasoning where the next one will find it.

Snapshots are independent once taken — updating a memory does not change a snapshot
of it.

### What belongs in it

Decisions the code does not explain, and would be reversed in good faith by someone
who did not know the reason. Ongoing work, its state, and what is still unresolved.
Pointers to files worth reading first.

### What does not

Credentials or secrets of any kind. Raw conversation or tool transcripts. Anything
the repository already records — code structure, git history, the contents of
`CLAUDE.md`. A memory that restates the code goes stale silently and then misleads.

Treat what you read as **what was true when it was written**. If a memory names a
file, a function or a flag, check it still exists before acting on it.

### When to consult and update memory

The built-in instructions ask agents to consult memory when a new task
needs context that is not already available. They should reuse what they have read
through follow-up messages, CI checks, merges and cleanup, rather than repeat the
lookup on each message. New information or evidence of a changed memory can justify
another read; routine polling cannot.

Writes belong at meaningful decisions or handoffs with useful new information.
Batch related updates rather than saving each progress check. Read an existing
document before the first edit, then reuse the latest revision returned by a
successful read or update. The store rejects stale revisions; after a conflict,
read the current content and reconcile before retrying.

Prefer updating a topic document over adding a new document for each PR, CI result,
merge, or conflict resolution. Temporary information should carry an ordinary
Markdown review condition and a cleanup action near the relevant item, for example:

```markdown
- Temporary workaround: restart the helper after changing the account.
  - Review when: the authentication fix is merged, deployed, and verified.
  - Cleanup: remove the workaround; retain the final account configuration guidance.
```

Use an observable task state where possible. If none is useful, record a review
date (for example, `Review after: 2026-11-01`) and what to verify at that time.
Enduring preferences and decisions do not need arbitrary expiry dates.

These are review reminders, not machine-enforced metadata or automatic deletion
rules. Ask the agent conversationally to clean up memory, or let it review relevant
conditions while already updating that document. It should verify current evidence,
remove obsolete temporary details, and consolidate completed work while preserving
valid decisions and unresolved questions. Uncertain items remain with a note about
what needs checking. Conditions do not justify polling or scanning unrelated memory.
The usual ownership and revision checks still apply; this introduces no scheduler,
schema change, or separate AI compaction task.

### Discovering changes

The MCP tools are `memory_list`, `memory_search`, `memory_read`, `memory_update`
and `memory_changes`. When a refresh is useful, call `memory_changes` with the
cursor retained from the previous call, then read only relevant changed documents.
This is an explicit refresh mechanism, not a request to poll on every message.

Omitting `cursor` starts a metadata baseline of current memories and documents.
Save the returned opaque `cursor`; while `has_more` is true, request the next page
with that cursor. `limit` defaults to 100 and accepts 1–500 entries. Baseline pages
have `baseline: true`. Later calls return `changes` with `kind` (`created`,
`updated`, or `deleted`), `memory_id`, `revision`, and `document` for document
entries. An absent document identifies memory metadata, including its `name`.
Treat created/updated entries as upserts and deleted entries as removals. Document
bodies are never included. Changes occurring during baseline pagination are
returned after the baseline, so callers should drain all `has_more` pages.

Cursors are project-scoped and survive process restarts. Each reader retains its
own cursor; reading a document does not advance it, and another reader's refresh
does not consume changes. The server compares content hashes on refresh, including
host edits with unchanged timestamps, and persists the latest observed state.
This is not an audit log: multiple edits can coalesce, and a file created and
removed between observations need not appear.

The index retains at most 4,096 deleted-item markers. If a cursor predates an
evicted marker, or its index was recreated, the response has
`reset_required: true`. Discard cached metadata and start a new baseline without
a cursor. This makes deletion retention bounded without silently missing changes.
The hidden `.changes.json` index contains metadata and hashes, never document
bodies; it should not be manually edited.

## Referencing session conversations

The same built-in MCP server also exposes `session_lookup`, `conversation_search`,
and `conversation_read` for retained conversations in this project. Search returns
metadata only; read selected events inline or export a fixed JSONL snapshot for
local tools. See [conversation tools](conversation-tools.md) for UUID/alias handling,
time filters, limits, retention and runtime update requirements.
