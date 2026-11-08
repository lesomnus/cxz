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

A Memory is one Markdown document in a project, with an optional owner session and
labels. A session can publish any number of memories. Published documents survive
session purge. Each web document has an address at
`/c/_/project/{project alias or ID}/memory/{memory ID}`. Project aliases and UUIDs
both resolve; generated links prefer the alias with UUID fallback. Memory IDs
remain UUIDs, and API calls use the resolved stable project ID. The parent path
`/c/_/project/{project alias or ID}/memory` shows the project document list. Reload and
browser history restore selection, and links can open in new tabs.
The web Memory collection is a dense responsive grid of borderless portrait cards. Each card
shows its owner's agent logo and session name at the top left, relative update
time at the top right, the title at conversation body size below, and the
revision at the bottom left. The collection loads only paged metadata; opening a card loads its
separate document page, without a catalog sidebar. Clicking Memory in the application sidebar returns to the current project
collection; the document toolbar has no separate back button. A searchable session picker beside the project picker filters by owner
using the complete project metadata cached on the authenticated connection.
Session changes filter that metadata locally, including the first selection,
without a list request. The cache expires after one minute; returning to a
project or changing session then refreshes in the background while retaining
existing cards. Save, copy, delete, restore, purge and explicit reload invalidate
the affected project list. Refresh failures retain cached cards and offer retry.
The cache is bounded and cleared when the connection is disposed, with no
browser storage persistence. All sessions includes unowned and
retained-owner documents. Project changes reset the session filter; returning
from a document preserves the selected filter. Changing it while editing guards
unsaved copied-document changes. Selecting an editing session is unnecessary.
The detail page groups an Account-style information section, project-style
Copy/Reload/Delete action panels and a Markdown preview section. Information uses
the shared ResourceFields label/value layout: a title-only first row, then Project,
Session, Created and Updated in an equal-width metadata row. Project and Session
omit visible labels. Other metadata is omitted.
A revision selector uses the space beneath the action buttons without increasing
the information row’s height. Its trigger itself has the section panel appearance.
Narrow layouts place the selector beside the action buttons. It lists retained
versions through `Versions`; choosing one loads its body through `Read`, while
Latest follows the current document. Title and resource dates remain current.
The selector and choices place the revision number above its creation date on the
left and compact added/removed line counts together on the right, above the
humanized age of the revision. The selector uses the
action buttons’ corner shape. First revisions show Created; unavailable counts
remain unknown.
The preview has roomier content padding and a Preview/Changes switch. Changes
requests the selected revision against its immediate predecessor with `Read` in
DIFF format, on demand through the same Payday query cache. Added and removed
lines use the same subdued colors as revision counts. The full selected body is
kept separately, so Copy never copies the patch. First revisions, missing
predecessors, empty diffs and failed comparison reads have explicit states;
failed reads can be retried. Changing revisions keeps the chosen preview mode.
Clicking the larger title opens the shared
inline value editor; confirming patches only the name under its observed version,
preserving the body, revision and labels. Existing document bodies remain preview
only. Copy opens a new draft with source, labels and owner controls and Save/Cancel
panel actions; copying an older revision uses its displayed body. Additional
document metadata is omitted. Loading retains the grouped frame
with its actions disabled until the document is available.
The TUI `/memory` view offers direct browsing, copying,
rename, trash/restore/purge, and starting a session seeded from a selected document.

`MemoryService` is independent of `SessionService`:

- `List(project=ref)` scopes to a project; `List(session=ref)` to an owner;
  `List()` covers all projects accessible on the connection. Each label map is an
  AND expression, and repeated filters are OR expressions. List/search returns
  metadata only; `Read` retrieves the body.
- `Add` accepts `client_id` for idempotent retries. `Patch` requires the last read
  `date_updated`; body edits also require `base_revision`. Labels can be cleared
  explicitly with `clear_labels`. Apply is intentionally unavailable.
- Each changed body gets an increasing `revision` and an immutable full-body
  MemoryVersion with a SHA-256 hash. Its optional `lines_added` and
  `lines_removed` fields count changes against revision − 1, excluding diff headers,
  context and EOF marker lines. Zero is a known count; absence means the first
  revision or a missing predecessor. New counts commit with the body, and schema
  migration backfills retained adjacent bodies without rewriting their contents,
  hashes or timestamps. Changing metadata does not advance the body revision or
  recompute its counts. Reverting A → B → A creates three versions; the two A
  hashes match.
  The current row caches the latest body. Versions and the resource commit in one
  transaction. Audit keeps management metadata, without duplicating body payloads.
- `Read` defaults to full text. An explicit `revision` reads a retained version;
  `Versions` lists retained revisions and their stored line counts. The web
  revision selector uses these retained revisions. With `base_revision`,
  `format=DIFF` requests
  a contextual unified diff. `AUTO` chooses a smaller diff, otherwise full text.
  A missing base returns full text with `fallback_reason`. Smaller byte size is a
  heuristic, not a tokenizer or a guarantee of model comprehension.
- `Erase` sets `date_deleted` 30 days ahead. An unset value is active, a future
  value is restorable trash, and an elapsed value is logically absent even before
  garbage collection. `Restore` requires the last metadata version. `Purge`
  requires trash and permanently removes all body versions.

There are no Snapshot/Fork/Merge resource types. A copy is a new Memory. Restoring
an old body means reading that version and patching it as a new revision. Existing
saved snapshots migrate to ordinary, independently editable documents.

The manager routes requests to the project's `resources.db`, including a disposable
helper for stopped project volumes. The helper verifies volume ownership and
project identity and opens the DB as its owner. Memory operations cannot run inside
Batch database transactions. The separate native agent-data browser uses
`SessionService.ReadAgentData` and `CopyAgentData`, not MemoryService.

### Migration

Startup and retained-volume access import legacy collections idempotently. Each
Markdown file becomes a Memory; `legacy.collection`, `legacy.collection-name`,
`legacy.filename`, and `legacy.agent` labels retain provenance. Saved copies carry
`legacy.snapshot=true`. The first retained body becomes revision 1; the old
metadata-only Changes index cannot recover historical bodies. Deleted documents and collections enter trash with a fresh
30-day recovery window. Missing source sessions retain a stable owner identity.

A migration marker and the imported document/version commit together. Unchanged
sources are skipped. A late legacy edit can advance the imported body if the new
API has not changed it; otherwise it creates a separate `legacy.conflict=true`
document instead of overwriting new work. Legacy files remain untouched and are
recovery material, not the live store. Deletions in the old tree do not delete new
resources. Retire old writers when deploying the new API. Back up SQLite using a
consistent backup, including committed WAL contents, before upgrading.

The retired `SessionService.Library*`, `SessionService.Memory`, and
`SessionService.CopyMemory` RPCs are removed; clients must upgrade together.

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

The MCP tools are `memory_list`, `memory_search`, `memory_read`, `memory_add`,
`memory_update`, and `memory_changes`. They bind project/session/run on the server:
reads cover the project, while mutations are limited to the caller's own documents.
Use IDs returned by list/add. Updates carry the body revision; metadata edits also
carry the returned metadata version. Conversation tools remain independent.

Call `memory_changes` only when a refresh is useful. Without a cursor it starts a
paginated metadata baseline; retain the cursor and drain `has_more` pages. Later
calls return created/updated/deleted metadata, never bodies. Changes during baseline
pagination are drained afterward. Treat entries as upserts/removals, then read only
relevant documents. A document read does not advance the changes cursor.

Changes is a discovery feed, not recoverable history. MemoryVersion supplies body
history. The DB retains the newest 10,000 change records; a pruned or foreign-epoch
cursor returns `reset_required`. Discard cached metadata and start without a cursor.
Project cursors survive restarts; connection-wide cursors combine project cursors.

## Referencing session conversations

The same built-in MCP server also exposes `session_lookup`, `conversation_search`,
and `conversation_read` for retained conversations in this project. Search returns
metadata only; read selected events inline or export a fixed JSONL snapshot for
local tools. See [conversation tools](conversation-tools.md) for UUID/alias handling,
time filters, limits, retention and runtime update requirements.
