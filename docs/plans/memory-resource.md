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
- [ ] Resource schema/generation and transactional versioned service.
- [ ] Project routing, retained-volume access and resumable migration.
- [ ] API-backed MCP, ownership and launch/scope validation.
- [ ] Flat web/TUI/native clients and fixtures.
- [ ] Remove old RPCs/storage entry points; integration/migration/race/browser tests.
- [ ] Migrate real data with backup/verification, locally merge /workspace.

Worktree: /tmp/cxz-memory-resource-v2, branch feat/memory-resource-v2. Shared task:
memory-resource-v2. Do not restart user dev servers 5173/6006; own test servers must
be stopped. Do not push unless requested. Keep the shared root branch and WIP.
