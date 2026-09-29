# Shared project memory

`/memory` opens provider-neutral project memory. A memory is a named collection
of UTF-8 Markdown documents. Working memories belong to individual sessions;
saved memories are independent copies. Claude and Codex use the same format.
No AI compaction, automatic summarization, or background model account is used.
Auxiliary AI task profiles are tracked separately in
[issue #27](https://github.com/lesomnus/cxz/issues/27).

## Use from the TUI

- Enter or click a row to open a memory/document; Left returns to its parent.
- `/` searches memory names, document names, and document content (substring search).
- `s` saves this session's current shared memory under a chosen name.
- `c` saves a copy of the selected memory; `e` renames it.
- `n` freezes the selected memory, opens the normal account/provider selection,
  and copies it into the new session's working memory. The original keeps its content.
- Space selects memories; `m` combines 2–8 selections into a named collection.
  Documents are prefixed by source position, and the manifest records the source IDs.
  This preserves conflicting statements; it does not semantically reconcile them.
- `d` moves a selected document or memory to trash after confirmation.
- `t` opens trash (within a memory, deleted documents); `u` restores the selection.
- `x` permanently removes a trash item after confirmation and releases its storage.
- `i` explicitly imports native Markdown memory/instructions from this session.
  Claude `CLAUDE.md` and `projects/*/memory`, Codex `AGENTS.md`, and `memory`/
  `memories` directories are supported. Transcripts, credentials, skills, and
  arbitrary settings are excluded. Imports retain a source-path comment and use
  content-addressed filenames; repeated identical imports do not duplicate files.
- `o` opens the existing native agent-profile file browser.
- `r` refreshes; an open shared-memory view also refreshes every five seconds.

Saving captures **written memory**, not the whole live model context. New sessions
receive instructions to read relevant memory and update their own after meaningful
work. Tool availability does not guarantee the model will record every fact.
Existing agents must restart with the updated supervisor to receive the MCP tools.
For an older session, native import can bring its saved files into shared memory.

Deleting a memory does not erase context already read by a running agent. If that
agent publishes new notes later, its working memory may be created again. Restoring
an older collection creates a separate saved copy if that identity already exists;
it never overwrites the newly published memory.

## Storage and external editing

Each project runtime's state contains:

```text
memories/<project-key>/
  session-<session-key>/
    manifest.json
    overview.md
    handoff.md
    decisions.md
    ...other Markdown documents...
  saved-<id>/
    manifest.json
    ...copied Markdown documents...
  trash/
    saved-<id>/...
```

Keys derive from project/session identities, not display names or filesystem paths.
The manifest records format version (`1`), name, origin
session/provider, creation time, and source memory IDs. Document revisions are
SHA-256 hashes of their current contents. Suggested filenames are conventions, not
required documents. Keep document names flat, start with a letter/number, and use
`.md` (letters, digits, spaces, underscores, hyphens and dots are accepted).

The TUI shows the actual runtime location. With the standard Docker manager this
is retained project storage, displayed as `volume:<name>/data/memories/...`;
it is **not a local client Downloads/config directory**. On a Linux Docker host,
`docker volume inspect <name> --format '{{.Mountpoint}}'` identifies the volume's
host path. An editor must have permission to access it. Docker Desktop volumes
live in its VM; use a container/volume editing workflow there. With a remote
connection, editing happens on that remote Docker host. cxz does not silently
copy or synchronize the memory to the frontend machine.

Edit the Markdown files using an external editor; maintain the runtime user's
ownership/permissions. Changes are read on the next API/MCP request and the next
TUI refresh. Existing read revisions become stale, so an agent must re-read before
replacing an externally edited document. Metadata is managed by cxz; avoid editing
`manifest.json` or temporary/lock files. External editors do not participate in
cxz's lock, so simultaneous edits at the exact time a write commits should be
avoided. Snapshots are independent copies, not filesystem-level immutable files.

Limits: 256 KiB per document, 256 live documents / 16 MiB per collection, and
1,000 listed collections. Exceeding a write limit gives an error; no content is
silently compacted or deleted. Purge unnecessary trash to recover disk space.

## Agent integration

`cxz Memory` is a built-in provider in the [common MCP manager](mcp.md), enabled
by default. Global defaults and project on/off overrides work exactly as for
external MCP servers. Disabling it omits both tools and usage instructions from
new agent launches, but preserves stored memories and the TUI memory browser.

The provider runs inside the shared project runtime using the
[official Go MCP SDK](https://github.com/modelcontextprotocol/go-sdk). Each agent
connects through the generic stdio-to-Unix-socket bridge; there is no separate
memory server process per session. It exposes:

- `memory_list`: list this project's working memories and saved copies.
- `memory_search`: search names and content.
- `memory_read`: list a memory's documents, or read one document and its revision.
- `memory_update`: create or replace a document in the calling session's own memory.

The runtime binds the provider to the authenticated launch and retained session manifest.
MCP callers cannot select a different project, update another session's memory,
or invoke TUI-only delete/restore/import/merge operations. These are API boundaries,
not a filesystem sandbox between agents that already run with full access as the
same container user. A project-scoped filesystem lock serializes cxz writers across
the runtime and retained-volume helper processes. Documents are staged and renamed, so cxz readers see complete
writes. Saved copies are taken under that same lock.

Claude and Codex receive only the activated cxz-managed servers in their launch
configuration, with Claude retaining strict MCP configuration. Both are told to treat other sessions' notes as reference data, verify
stale claims, preserve current user/project instructions, and omit secrets.
No raw transcripts are automatically copied or broadcast into another session.
The model reads selected documents on demand, so total stored memory need not all
enter its context. MCP changes do not push new text into an already running turn.

The TUI uses a new `SessionService.Library` RPC through the selected manager.
Older managers need updating; older project helper binaries need the runtime
update before library operations work. There is no database schema migration or
need to delete the existing database/native agent profiles.
