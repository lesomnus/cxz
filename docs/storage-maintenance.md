# Storage maintenance

Manager and project runtimes perform eventual background cleanup. Maintenance
never evicts workspaces, conversations, provider history/credentials, live
memories or referenced uploads to satisfy a disk-space target.

## Policy

Put an optional `maintenance.json` in the server's data directory, normally
`/cxz/state/data`. Missing fields use defaults; explicit zero disables an age
rule. Invalid settings stop cleanup and produce an operational error.

```json
{
  "disabled": false,
  "audit_days": 90,
  "notification_days": 30,
  "trash_days": 30,
  "temporary_hours": 24,
  "binary_days": 7,
  "recent_versions": 2,
  "interval_minutes": 30,
  "runtime_log_mib": 32,
  "component_log_mib": 8,
  "pressure_low_percent": 5,
  "pressure_high_percent": 10,
  "minimum_free_mib": 0
}
```

The Manager propagates an explicitly configured policy to projects on its
shared state volume, including `disabled`, without a server restart. Legacy
projects with separate volumes use their own local file. Removing the Manager
file stops propagation; it does not erase a project's last policy. Policy is
non-secret and propagated files are readable by the project's remote user.

`disabled` stops all automatic removal. Zero `audit_days`, `notification_days`
or `trash_days` keeps that data indefinitely. Zero `temporary_hours` disables
temporary-work cleanup. Zero `binary_days` disables ordinary age-based binary
cleanup; unreferenced versions remain eligible under disk pressure. Set both
`pressure_low_percent` and `minimum_free_mib` to zero to disable pressure GC.
Log sizes, the ordinary interval and recent-version reserve must be positive.

## Opportunities and limits

Cleanup runs after startup and periodically (30 minutes by default). Journal
activity and update metadata changes request an earlier pass, debounced to at
most once every five minutes. Between passes, one-minute filesystem probes
detect disk pressure without recursive directory-size scans. A pass has a
20-second cancellation budget and examines at most 128 file candidates. Database
and memory retention use their own bounded batches within that time budget.

- **Audit:** payday `trail.Policy` deletes rows older than `audit_days`, without
  an archive. There is no audit byte quota. SQLite may reuse freed pages without
  shrinking its file. No emergency or startup-blocking VACUUM is introduced.
- **Logs:** runtime logs keep four files total, component logs three. Each
  archive holds at most its configured size. Known logs include supervisor,
  agent/MCP stderr, project provisioning and the current project user's editor
  log. Rotation is copy-and-truncate so inherited append descriptors continue
  writing to the current file across Manager restarts. Diagnostics are best
  effort: concurrent lines at that boundary can be lost. Durable journals and
  audit never use log rotation. Oversized pre-upgrade logs retain a bounded tail.
- **Temporary work:** installer staging and stopped session-profile temporary
  files expire after 24 hours. Normal operations already remove their staging
  directories on completion. New supervisors reset the stopped-profile grace
  marker. Profile locks held by supervisors and process-group guards protect
  running agents and background children.
- **Versions:** all known session, runtime, selection, version-pin and rollback
  references win over expiry. Two recent *unreferenced* versions per tool group
  also remain. Publication/launch leases serialize new references with GC;
  references are checked again before removal. A Manager with inaccessible
  legacy project state skips shared-tool GC rather than assuming it is unused.
  A live older runtime without an immutable identity, or a live session without
  a supervisor reference, also blocks executable cleanup until it is updated.
- **Notifications:** retain recent read history for 30 days and the latest
  unread notification per session/kind. Completion, failure and question remain
  separate. Retention preserves source/read cursors and pending questions,
  removes private receipts with their notifications, and invalidates pages
  through payday. Counts describe retained items, not lifetime completions.
- **Memory trash:** only deleted documents/bundles expire after 30 days. New
  deletions use deletion time rather than original document modification time.
  Existing trash gets a full upgrade grace period. Cleanup shares the memory
  library lock with restore/edit operations. Live documents and saved snapshots
  are unaffected.
- **Legacy journals:** a stopped session's `events.jsonl` is removable only
  after a completed conversion, the current compatibility fence, and validation
  that the store covers its final complete batch. Missing/unknown metadata,
  oversized or torn tails, and active writers prevent removal.

Conversation retention (default 100MiB/session, vendor raw 20MiB), browser/RPC
log rotation, auxiliary output retention, secret TTL and asset reference GC
continue to own their respective data. There is no new upload quota or aggregate
cache byte limit. Provider-native retention remains the provider's concern.

## Disk pressure

Available space is checked per filesystem. Enter below 5% available (or below
the optional absolute minimum); recover at 10% and at least the absolute minimum.
The hysteresis avoids repeating work around one threshold. Space on another
filesystem is not counted toward recovery.

Priority is:

1. Verified migration residue (existing asset GC handles unreferenced uploads).
2. Finished/interrupted temporary work.
3. Rotated log archives.
4. Unreferenced executable versions, bypassing the normal seven-day grace but
   preserving the recent-version reserve and every known reference.
5. Reproducible Go, pip and npm caches in stopped session profiles.

Recheck actual available space before each pressure action and after each
filesystem pass. Cache cleanup requires the same profile lock as an agent;
even an idle live agent is protected. Arbitrary HOME caches, credentials,
provider state, active temporary work and workspace files are never swept.
Symlinked cleanup paths are skipped. The deletion walk honors cancellation.

When safe candidates are exhausted or a pass reaches its budget, cleanup stops
and retries no sooner than the next filesystem probe. It does not widen the
deletion policy. Database retention is deferred while pressure remains because
WAL writes may consume space without reducing the physical database file.

`maintenance-status.json` in each server data directory reports the last pass,
actions, errors, filesystem available/target bytes, remaining shortfall and
pressure state. An unresolved shortage is also logged. No expensive protected-
data size inventory is required to produce this report.

## Deployment

Update the Manager and project runtimes to activate the maintenance loop. UI
updates alone do not activate it. Supervisors already running still hold the
profile leases the collector respects; no agent restart is needed for safety.
New supervisors additionally reset the temporary-file grace marker on launch.
