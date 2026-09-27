# Bounded conversation history

cxz limits its own display/event history without deleting the session or the
provider's conversation files. Codex thread IDs, Claude resume IDs, selected
model/effort, permissions and command deduplication survive pruning. It does not
invoke provider compact, rollback or thread deletion.

## Defaults and controls

- Server: 100 MiB per session journal. Once exceeded, a safe idle check removes
  the oldest completed turns toward 80 MiB. Checks run after completed turns and once a minute in the
  Supervisor. Stopped sessions are checked after they are resumed.
- Client: a loaded window of 200 turns or 10 MiB of encoded event data, whichever
  is reached first. Scrolling up loads retained older pages and evicts the newer
  end of the window. Scrolling down loads newer pages. Ctrl+End returns to the
  latest history. Live reception keeps its own cursor while older pages are open.
- Settings (Ctrl+.) has separate server disk and local client window controls.
  Server choices are 25/50/100/250/500 MiB or unlimited, with confirmation before
  changing the policy. Client choices are 5/10/20/50 MiB and 100/200/500/1000 turns.
  Server changes target the selected connection, apply to running projects and
  are delivered to stopped projects when opened/resumed. A failed delivery is
  reported; the saved Manager policy is retained for retry.
- For other client window sizes, use `cxz edit`:

  ```json
  "history": { "window_mib": 10, "window_turns": 200 }
  ```

A notice marks the oldest retained display history. Removed display records cannot
be reloaded by scrolling or disabling retention. The same agent conversation
continues using its own provider-managed state.

## Budget boundaries

The disk budget measures the session journal, including encoded raw provider
notifications. Runtime and Manager SQLite projections contain additional copies.
They delete the same old range, and SQLite reuses freed pages; an existing large
SQLite file need not immediately shrink. This is not a cap on the entire project
volume or on the Go/build cache, attachments or provider profiles.

Complete turns are the unit of deletion. The newest turn and an oversized single
turn are retained; active turns, outstanding approvals, uncertain model changes,
and active/unknown background work postpone server pruning. These conditions can
exceed the configured budget. Client windows also keep a single oversized turn
intact, including steering inputs and tool pairs. The byte window measures event
payload, not total process RSS or rendered screen cells.

Small control checkpoints and hashed command receipts are retained for safe
restart and duplicate rejection. They are not discarded to meet a display budget.
Atomic journal replacement temporarily needs space for both the old and new file.

## Consistency and compatibility

The journal starts with a versioned control checkpoint at the last removed
sequence. Retained events and new events keep their original, monotonic numbers.
Replacement is fsynced and atomically renamed; live readers detect the new inode.
The checkpoint and retained suffix are published together. The checkpoint's
control payload is never sent to clients as display history.

Runtime and Manager track a persistent retention floor. Delayed history responses
cannot reinsert rows below it. Managers also refresh the Runtime's floor at most
once a minute during session/history reads, covering offline or missed notices.

Before the first checkpoint, the Runtime authorizes the new format and marks its
DB as schema 2. A Manager that receives a retention floor is also schema 2.
**Earlier cxz releases cannot read this format.** Their `cxz use` preflight rejects
switching these installations to those releases; automatic-update recovery leases
also prevent starting compaction during an in-progress Runtime replacement.
The build and release manifest also advertise schema 2, so schema 1 clients
reject this release as an automatic update. Deployment requires an explicit
upgrade path; publishing this as an ordinary compatible edge update is insufficient.

After the schema 2 build is published on main/edge, finish active agent work,
then run the following on each client and on the host managing the installation:

```sh
cxz self-update --client-only
cxz use @edge
```

The first command installs the new client without the automatic updater's schema
equality check. The second uses the new client to replace managed components and
resume sessions; it force-restarts the managed installation on a Linux host.
A Windows frontend updates only its own installation, so its remote Linux host
must be upgraded separately. These commands assume the existing selection is
`@edge`; an exact version pin must be cleared before `self-update`.

Do not delete `cxz.db`, session journals or provider profiles. The new runtime
reads schema 1 and 2. It adds the retention-floor table when needed and marks the
state schema 2 before writing checkpoints. Existing project/session metadata is
preserved and old display records are removed only by the retention policy.

Disabling retention does not reverse the format change. Export/back up state
before adopting the feature if rollback to a pre-retention release is required.

Provider profiles and conversation files are untouched, but this does not promise
that their own disk usage is bounded: their retention is managed by each provider.
