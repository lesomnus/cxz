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

The current MCP tools are `memory_list`, `memory_search`, `memory_read` and
`memory_update`. There is no changes-since cursor or per-reader last-read tracking.
Memory listings include `updated`, and document listings include `modified` and
`revision`. A caller can compare document listings it has retained, but must do
that comparison itself. The memory-level timestamp is derived from the remaining
documents, so it is not a reliable deletion/change-feed cursor.
