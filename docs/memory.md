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
