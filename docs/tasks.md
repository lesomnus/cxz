# Project Task API

Tasks are a project's shared TODO records. They describe planned, claimed,
waiting, completed or canceled work across independent agent sessions. They are
not provider background jobs, auxiliary AI runs, a scheduler or a replacement
for conversation history.

## Authority and persistence

`cxz.TaskService` uses the normal payday resource protocol, exposed through the
same authenticated gRPC/Connect transport as other resources. Go and TypeScript
clients are generated from `proto/cxz/task.proto` and its service overlay.

The project's **runtime** owns Task rows in its persistent `resources.db`, in
the existing project state volume. Manager resolves the project and forwards
the public RPC to that runtime over the private authenticated connection. It
does not keep a second Task catalog or answer from stale manager-side rows. A
stopped/unreachable project must be started before its Tasks can be read or
changed. Down/recreate preserves Tasks when the state volume is retained;
project purge removes them with that volume.

The WASM sandbox registers the same service with an in-memory storage adapter
and the same validation/claim policy. Resetting the sandbox resets its Tasks.
No Docker, provider account or external database is needed there.

## Resource

| Field | Meaning |
|---|---|
| `id` | Server-assigned payday resource ID |
| `alias` | Human-readable project-local identifier; generated when omitted, normalized and unique |
| `client_id` | Required creation retry key, unique within the authoritative project |
| `project` | Immutable owning project |
| `name`, `desc` | Work title and description |
| `status` | `TODO`, `IN_PROGRESS`, `WAITING`, `DONE`, `CANCELED` |
| `priority` | Ordered priority; lower is more important |
| `labels` | payday's standard string-to-string label map |
| `assignee`, `assignee_run` | Owning session's runtime ID and run ID; only Claim/Release write them |
| `source_session`, `source_run`, `source_seq` | Optional immutable reference into the retained conversation |
| `date_created`, `date_updated` | Server timestamps; `date_updated` is the optimistic concurrency token |

Entity numbers follow payday's schema contract: header slots retain their
standard meanings, application fields use 8–12 and 16 onward, and 13–15 remain
the update/erase/create timestamps. `client_id` is an application field, not a
payday-provided header. Service-overlay fields are kept outside the generated
request numbers; standalone helper messages do not use entity header numbering.
This revises the unreleased draft wire schema; regenerate clients and use a
matching runtime when testing this branch.

Ownership and source references are snapshot IDs, not session foreign keys.
Session purge retains Tasks and those references. A reference does not prevent
journal retention from removing the referenced event. Purge does not silently
complete, cancel or release the session's work.

## Operations

All operations require a project scope. Add uses `project`; Get/Patch/Claim/
Release use `ref.project`; List/Watch use their top-level `project`. A Task ID
alone is not a routing instruction. Manager canonicalizes project/session
aliases before forwarding; the runtime independently checks the project scope.

- **Add** creates unassigned Todo work. Supply a fresh `client_id`; retries
  return the first record with that key, including after its mutable fields
  changed. Reusing the key is not an upsert.
- **Get** reads one Task by ID, alias or `client_id`, with payday field selection.
- **List** pages by creation time and ID. Filters are OR-ed; fields within one
  filter are AND-ed. Every filter is constrained to the required project. Ref
  and assignee filters are supported, including an explicitly empty assignee
  for unowned work. Status filtering is currently client-side over loaded rows.
- **Watch** starts with current matching state, including an empty frame for
  an empty project, then emits committed full-row replacements. Absence of a
  previously delivered row's value removes it from the subscription's view.
  Reconnect for a fresh snapshot; this is not a replayable event journal.
- **Patch** updates alias, name, description, priority, labels and status using the
  `date_updated` returned by a read. Force updates and direct ownership edits
  are rejected. Ordinary unassigned work cannot enter In progress/Waiting
  through Patch. Owned work returns to Todo through Release. A nonempty labels
  map replaces the labels; `clear_labels: true` clears them with the same
  version check. An empty map alone means no edit, since protobuf maps have no
  presence. Combining a nonempty map and `clear_labels` is rejected.
- **Claim** compares the observed version and atomically assigns unowned Todo
  work to the supplied session/current active run, moving it to In progress.
  Two competing claims cannot both win. Retrying for the same current owner
  run returns its current In progress/Waiting record.
- **Release** compares the version and expected stored owner/run, clears
  ownership and returns work to Todo. An operator can release stopped,
  restarted or purged owners by naming their stored runtime ID/run. A stale
  Release cannot clear a newer claim, even by the same session/run.

In progress, Waiting and Done transitions on owned work require `actor` to
match the owner and its currently active run. Operators can cancel abandoned
work or edit descriptive metadata without impersonating that owner. Completion
retains ownership for attribution. There are no automatic lease expirations,
automatic task completion or automatic reassignment when a session stops.

The generated generic Apply/Erase methods are deliberately closed. Use the
versioned operations above and cancel Tasks rather than deleting shared work
history. Invalid input is `InvalidArgument`; stale versions, competing owners
and inactive/stale runs are `FailedPrecondition`.

The actor identifies the intended assignee; it is **not independent proof of
the caller's identity**. This API currently uses cxz's trusted operator/project
transport boundary. A future session-bound MCP facade must derive its actor
from the bound session rather than accepting an arbitrary model-supplied owner.

## Go client example

`conn` is an existing authenticated Manager or project gRPC connection. Use the
actual session's current run ID, not its provider conversation ID.

```go
tasks := resource.NewTaskServiceClient(conn)
project := resource.ProjectRef_builder{RuntimeId: proto.String(projectID)}.Build()
work, err := tasks.Add(ctx, resource.TaskAddRequest_builder{
    Project: project,
    ClientId: creationKey,
    Alias: "task-board",
    Name: "Implement project task board",
}.Build())
if err != nil { return err }

ref := resource.TaskRef_builder{Id: work.GetId(), Project: project}.Build()
actor := resource.TaskActor_builder{
    Session: resource.SessionRef_builder{RuntimeId: proto.String(sessionID)}.Build(),
    RunId: proto.String(runID),
}.Build()
work, err = tasks.Claim(ctx, resource.TaskClaimRequest_builder{
    Ref: ref, Actor: actor, DateUpdated: work.GetDateUpdated(),
}.Build())
if err != nil { return err }

_, err = tasks.Patch(ctx, resource.TaskPatchRequest_builder{
    Ref: ref, Actor: actor, DateUpdated: work.GetDateUpdated(),
    Status: resource.TaskStatus_TASK_STATUS_DONE.Enum(),
}.Build())
return err
```

For a browser client, create a Connect client from
`ts/gen/cxz/task_svc_pb.ts` using the existing connection transport. It works
with the sandbox transport too. A Task board, CLI/TUI task commands and an
agent-facing Task MCP are not part of this API implementation yet.

## Verification

`internal/taskapitest` runs the same public gRPC contract against SQLite and
the sandbox. It covers claim races, creation retries, version/run fencing,
forbidden write paths, project isolation, pagination, alias naming/lookup,
standard payday header reads and filtered Watch removal. A schema regression
checks payday-reserved field slots and standard labels types.
Native tests additionally verify persistence after rebuilding the service,
session projection removal, Manager forwarding with independent aliases, live
Watch forwarding and absence of manager-side shadow rows or offline fallback.
