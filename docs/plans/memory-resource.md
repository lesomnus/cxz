# Document Memory resource transition

Authorized 2026-11-02: one Memory is one Markdown document, scoped to Project and
owned by a retained session identity. Labels replace collection/directory grouping.
MemoryService replaces all SessionService.Library* RPCs. MemoryVersion retains
immutable full bodies; monotonic per-memory revisions are separate from hashes.
Changes is incremental metadata discovery; Read can return a bounded contextual
diff from a retained base revision or explicitly fall back to current full text.

List supports project, owner session, label predicates, and all projects visible on
one connection. Memory deletion uses a future date_deleted: absent is active,
future is restorable trash, elapsed is logically gone regardless of GC progress.
Restore is allowed only before the deadline. Audit records management operations;
body versions live in MemoryVersion rather than duplicated audit payloads.

Migration must be deterministic/idempotent, preserve every live, saved/snapshot,
and deleted document with origin labels and ownership, validate before completion,
and leave the legacy source recoverable. Existing data is not to be discarded.
No new Snapshot/Fork/Merge resource types or mandatory Editing-as context.
MCP binds project/session server-side and uses MemoryService, preserving independent
conversation lookup/search/read tools. Native file browsing is a separate feature.

Implementation phases:
- [x] Resource schema/generation and transactional versioned service.
- [x] Project routing, retained-volume access and resumable migration.
- [x] API-backed MCP, ownership and launch/scope validation.
- [x] Flat web/TUI/native clients and fixtures.
- [x] Remove old RPCs/storage entry points; integration/migration/race/browser tests.
- [x] Migrate real data with backup/verification.
- [x] Locally merged /workspace after final checks.

Worktree: /tmp/cxz-memory-resource-v2, branch feat/memory-resource-v2. Shared task:
memory-resource-v2. Do not restart user dev servers 5173/6006; own test servers must
be stopped. Do not push unless requested. Keep the shared root branch and WIP.

## Verified data migration (2026-11-02)

Current project: 55 documents, 1,076,875 bytes at initial import. Source-file,
Memory and MemoryVersion hashes matched for all 55. SQLite integrity_check passed;
foreign_key_check returned no violations. Repeated import is marker-based and
idempotent. Original files remain under memories/; they are recovery sources.

Consistent SQLite backups of resources.db and cxz.db, a locked legacy file archive
and per-document hash manifest are retained in:
`/cxz/state/data/backups/memory-resource-20261102T033941Z`.
This path belongs to this development installation, not a repository fixture.

Validation: TypeScript 721 unit tests, 14 Memory Storybook browser tests and the
real web client conflict-save test; Linux/Windows builds; full Go suite; race checks for lifecycle/MCP/server/workspace/TUI;
generator check; go vet; TypeScript check/build/unit tests; browser contracts for
flat selection, editing conflicts, metadata-only revision stability, history/diff,
old-body restoration, labels, trash/restore/purge, project scope and narrow screens.
Docker validates stopped volumes owned by different non-root users, source
migration, project isolation, native file copying and credentials preservation.

Source/API changes are separate from runtime rollout. No release/push or running
runtime replacement is performed here. Existing runtime and MCP bridge processes
keep their old binary until upgraded; new binaries remove the old RPCs and register
the new tools. Legacy source edits during that interval are picked up on next
migration, preserving conflicts rather than overwriting new API work. User dev
servers are not restarted.
