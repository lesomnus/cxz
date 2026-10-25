# Agent CLI preparation and updates

Implementation plan: [cxz#301](https://github.com/lesomnus/cxz/issues/301).

A project owns its container and runtime metadata. The installation owns the
shared `/cxz/tools` cache, mounted read-only in project containers. Preparing an
agent is independent of creating or recreating a container.

## Selecting an executable

The Manager selects a release from `/cxz/tools/releases.json`. Without a valid
selection it uses the provider version bundled with cxz. Versions left in the
cache for older sessions do not participate in selection; directory order is
not a release policy.

The release layout includes the container's architecture and, for Claude, its
libc variant. Codex uses the complete musl package, including its sibling helper
executables. Cached entry points must be regular executable files. A download
uses the existing checksum verification, installation lock and atomic publish.

Session creation, Claude session login, and starting an existing project with
`--agent` all prepare that provider through the Manager. The Manager verifies the
binary with `--version` as the container's remote user, then atomically records
the path in `runtime.json`. Project identity, token, workspace and the other
provider's path are preserved. Preparation uses the project lock and does not
repeat provisioning checkpoints, restart an agent or recreate the container.
The lock is released before an interactive login waits for a person.

If a selected release cannot be prepared, an existing registered executable may
still be used when it runs successfully in the container. The failure is logged;
the working registration remains unchanged. Failed or canceled preparations do
not publish a new path.

The project runtime also has a read-only fallback for direct callers: use its
registered executable if available, otherwise resolve the selected compatible
release already in `/cxz/tools`. It never downloads, writes the shared cache, or
chooses an unrelated cached version. When nothing is usable, the error directs
the caller to preparation through the Manager.

## Existing sessions

A session records its own executable in its manifest. Changing a project
default affects future sessions; it does not replace a running process.

The existing update queue remains responsible for replacements:

- Check upstream releases every 24 hours and consider rollouts every 30 seconds.
- Require five continuous idle minutes by default, with no active tools,
  background tasks, pending approvals/questions or other readiness blockers.
- Fence new input and recheck readiness when stopping, preserve the conversation
  identity, and recover or roll back using the durable update transaction.
- Promote a project's default path only after a successful agent restart.
  A failed or rolled-back attempt does not promote its target.

Both provider-only updates and combined cxz/provider updates publish successful
agent defaults. A later observation of the selected binary already running also
repairs a missed default write, without restarting it again. A slower session's
update cannot downgrade a newer project default.

## Validation and compatibility

Regression tests cover selected-release cache reuse, architecture/libc layout,
read-only fallback, missing and stale registrations, offline fallback, failed
execution/downloads, manifest identity, concurrent provider preparation and
successful versus rolled-back default promotion. Existing idle gates, update
rollback and resource lifecycle tests remain applicable.

`runtime.json` keeps its existing shape, and conversation databases are
unchanged. No database reset, new RPC, container recreation, or manual CLI
installation is required. An old Manager can benefit from a newer runtime's
read-only cache fallback, but automatic downloads and registration repair need
the updated Manager.
