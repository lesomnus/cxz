# Retained agent data

Select a session in the project list and press **m**, or submit `/memory` in
its conversation. This opens a full-screen file browser. It shows the selected
session, provider/account, project and retained storage path.

- Up/Down and PgUp/PgDn select entries or scroll text; Home/End jump.
- Enter or Right opens a file/folder; Left or Backspace returns to its parent.
- Mouse clicks open entries; the wheel browses/scrolls.
- `r` refreshes, `c` copies the selected file/folder, and Esc returns.

The browser reads files, with syntax highlighting for text. It does not start
or resume an agent. The manager mounts owned project state volumes in a temporary,
network-isolated helper, read-only for browsing. Stopped project containers are
not restarted. A missing volume is reported rather than recreated.

Profiles are stored under `session-profiles/<creation-key-hash>/accounts/<account>/config`
within the project's retained state volume. Names and aliases are labels rather
than storage keys, so renaming a project does not move its retained profile.
Each session has independent data; selecting a different account does not
automatically transfer memory.

The browser includes existing `CLAUDE.md` / `AGENTS.md`, memory directories,
instructions/skills/rules, Claude `projects`, and Codex `sessions` /
`archived_sessions`, plus other supported agent history/planning directories.
It displays actual stored paths, including old workspace names. It does not
infer a provider's project-path encoding or claim that every listed file is
automatically loaded by the agent. Credential files, symlinks and binary files
are excluded. User-written notes/history can themselves contain sensitive text.

A preview is bounded to the first 256 KiB and a directory to 1000 entries; limits
are shown. This is a current snapshot of stored files, not a filesystem backup
or a reconstruction from conversation events. Legacy sessions without a
session-local profile, deleted session records, and purged volumes are not
recoverable through this view.

## Copy to another session

Select a file or folder (or open a file), then press **c**:

1. Select an initialized, stopped session using the same agent. The account or
   project can differ.
2. Edit the destination path relative to that session's agent profile. It starts
   with the source relative path; adjust old workspace path components when
   appropriate.
3. Review source, target session/account and destination, then choose Copy.
   Cancel is selected by default.

The destination must not exist. No existing file is overwritten or directory
merged. A copy is staged and atomically published, and preserves the target
profile's file ownership. The server acquires the target supervisor and login
locks so running agents or login cannot race the copy. Stop the target first;
initialize it once if it has no profile/lock files yet. New parent directories
can remain empty if a copy fails.

Copy limits are 1000 entries, 16 MiB total and 256 KiB per text file. Truncated
sources are rejected rather than silently copying only their previews. Excluded
files and symlinks are not copied. Read snapshots are bounded but not transactionally
consistent with an actively writing source agent; stop the source too when you
need a stable snapshot.

Copying memory/instructions does not switch accounts, replace credentials,
change provider conversation IDs, or resume a copied native conversation.
Native history remains inspectable data; making the provider adopt another
conversation is a separate operation. Apply new memory on the target's next
start and verify its provider-specific location.

Update the host cxz and run `cxz install --recreate` to install the manager and
helper binary that support this view. Existing project containers do not need
to be recreated solely to browse/copy their retained volumes.
