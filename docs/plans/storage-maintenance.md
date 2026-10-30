# Opportunistic storage maintenance

Tracking: https://github.com/lesomnus/cxz/issues/320

## Contract

Maintenance is eventual background work. Use payday's existing age-based audit
retention; do not add audit byte quotas, recursive quota scans, or emergency
SQLite VACUUM. Protect conversations, provider state and credentials, live memory,
referenced uploads, workspaces, active work and every known executable reference.
Unknown activity or references prevent destructive cleanup.

Ordinary opportunities are startup, periodic checks and observed idle/update
transitions. Per-filesystem disk pressure enters below 5% available and exits at
10%; optional minimum available bytes supplements the percentages. Recheck free
space between ordered candidate groups and stop when recovered or exhausted.

Order: verified unreferenced/migration residue; finished temporary work; rotated
logs; unreferenced old executables; reproducible caches of inactive profiles.
Pressure may bypass disposable-data age grace, never reference/activity checks.
An exhausted pass records the remaining shortage and backs off.

## Implementation stages

1. Common validated policy, bounded/debounced runner, filesystem-pressure engine
   and payday audit age retention. Unit tests cover hysteresis, cancellation,
   ordering and isolation between filesystems.
2. Safe file adapters: append-log rotation preserving inherited open descriptors,
   age-based memory trash and read notification retention, temporary/cache
   cleanup under the same profile locks used by agents, and verified conversion
   residue cleanup. Keep current conversation/asset/browser policies.
3. Reference-aware immutable executable cleanup, installation/runtime integration,
   persisted operational status, documentation and integration tests.
4. Merge the worktree into /workspace, update its existing PR if present, pass
   CI and merge. Commit each coherent stage independently.

## Proposed defaults

- Audit: 90 days, deleted without archive (payday trail).
- Runtime log: 32MiB, four files total; component logs: 8MiB, three total.
- Unreferenced executable versions: two recent versions plus seven-day grace.
- Abandoned cxz temporary work: 24 hours, with inactivity/ownership proven.
- Read notification history and memory trash: 30 days.
- Ordinary checks: 30 minutes; cheap filesystem probes: one minute; all loops
  cancel on shutdown and use non-blocking cross-process maintenance locks.

The implementation documentation records the supported configuration and any
conservatively skipped candidates. Actual row deletion and filesystem reclaim
are separate; freeing database pages does not itself shrink a SQLite file.
