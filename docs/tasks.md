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
the caller's identity**. The public API uses cxz's trusted operator/project
transport boundary. The built-in Task MCP derives its actor from the authenticated
agent launch rather than accepting a model-supplied owner.

## Agent tools

The built-in `cxz_tasks` MCP is enabled by default, with the same global and
per-project activation controls as `cxz_memory`:

```sh
cxz mcp list --project PROJECT
cxz mcp disable --project PROJECT cxz_tasks
cxz mcp enable --project PROJECT cxz_tasks
```

New or explicitly restarted agents receive its tools and short usage instructions.
Updating a running agent's desired MCP settings does not replace its launch
snapshot. Use matching Manager/project-runtime versions; no task migration or
project recreation is required.

| Tool | Arguments and result |
|---|---|
| `task_list` | Paginated summaries; optional `mine` or `unassigned`, `limit` and `after`. Defaults to 20 rows, caps at 100; follow `next` for more. No descriptions, labels or source details. |
| `task_get` | `ref` is a task UUID or alias in this project. Returns details and a `version`. |
| `task_add` | Required `client_id` and `name`; optional alias, description, priority and labels. Returns unassigned Todo work. |
| `task_claim` | `ref` and last-read `version`; assigns this session/run atomically. |
| `task_patch` | `ref`, `version` and explicit edits. Status accepts `in_progress`, `waiting`, `done` or `canceled`. Use Claim to start and Release to return to Todo. |
| `task_release` | `ref` and `version`; releases this run's work. |

Project, session and run are not tool arguments. The runtime binds them using
the private launch capability. The run remains fixed for that connection, and
an old launch cannot silently adopt a replacement run. Reads are project-scoped;
mutations also require an active, listed session in the bound run. A missing
launch run requires restarting the agent. Each tool request has a deadline.

`version` is the exact update timestamp returned by a read or successful change.
It is passed through to the API's atomic comparison, never replaced with a newer
version behind the agent's back. A conflict requires rereading and deciding
whether to retry. Interrupted mutations are not automatically replayed by the
bridge.

Creation keys are scoped to the bound session before calling Add. Reuse a
`client_id` only to retry the same creation; different sessions using the same
short key do not accidentally share a creation. New tasks record the bound
source session and run. MCP agents may edit unassigned work or their own run's
work; they cannot change another owner's task or release it. Execution status
changes require ownership. Canceling unassigned work is allowed. Operators can
still release abandoned work through the public API's explicit owner/version
contract.

The tool layer calls the existing public Task RPCs. It adds no task database,
scheduler or background polling. The same MCP protocol contract is tested over
native SQLite and sandbox Task services; the browser sandbox itself does not
launch provider agents or stdio bridges.

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
with the sandbox transport too. Agents use the built-in MCP tools above;
dedicated CLI/TUI task commands remain a follow-up.

## Web board

Open **Tasks** in the resource sidebar and choose a project. The board groups
work into Todo, In progress, Waiting and Done. Recent completed work appears
first, with older completed and canceled work available explicitly. Cards show
the title, alias, owner, last update and ownership warnings. Owner and source
links open the corresponding session, with the source event selected when its
reference is available.

The project catalog is live through TaskService.Watch. The board and session
toolbar share a subscription for the same project; leaving the view retains its
in-memory snapshot, and returning reconciles it with a fresh server snapshot.
There is no browser-persisted authoritative Task store or polling loop. If the
runtime is stopped or does not implement Tasks, the view reports that condition
and offers Retry.

Use Add to register work or open a card to edit its title, alias, description,
priority and permitted status. The dialog reads the current Task, then saves
against that observed version. Incoming updates do not overwrite an open edit.
After a version conflict, the draft remains visible and an explicit Reload
latest task action lets the operator start from the new record. Stable Task IDs
keep an open detail view associated with its record across alias changes.

Stopped, unavailable and previous-run owners are flagged rather than silently
reassigned. Release requires confirmation and clears the stored owner/run under
the version precondition, including after the owning session was removed. It
does not stop an agent. Manual claim/reassignment is left to the agent tools;
the browser provides explicit operator recovery without bypassing the API's
ownership rules.

The session composer toolbar shows active Tasks assigned to that session's
current run, separately from provider background jobs. It opens the board with
that owner/run filter, which can be cleared to see the whole project. The URL
preserves project, selected Task and owner/run selection.

## Verification

`internal/taskapitest` runs the same public gRPC contract against SQLite and
the sandbox. It covers claim races, creation retries, version/run fencing,
forbidden write paths, project isolation, pagination, alias naming/lookup,
standard payday header reads and filtered Watch removal. A schema regression
checks payday-reserved field slots and standard labels types.

`internal/taskmcptest` exercises real MCP clients over both Task backends,
covering discovery, session-bound identities, creation retries, compact pages,
version conflicts, ownership, release/completion, project isolation and stale
runs. `internal/taskmcp/builtin_test.go` also checks activation, usage instructions,
the private runtime handshake, the stdio bridge and launch replacement fencing.
Native tests additionally verify persistence after rebuilding the service,
session projection removal, Manager forwarding with independent aliases, live
Watch forwarding and absence of manager-side shadow rows or offline fallback.

Web tests cover shared project subscriptions, reconnect snapshots, ownership
warnings and filtering. Storybook browser tests exercise creation, editing,
confirmed Release, concurrent updates and narrow-screen project selection. A
WASM browser test verifies the real Task service through session navigation,
registration and editing. `Resources/Tasks` supplies interactive board fixtures
without starting a project runtime.
