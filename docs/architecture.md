# Architecture

## Ownership

Everything cxz creates carries a label identifying the installation that owns it.
cxz acts only on what it owns.

A devcontainer VS Code started is **detected and never adopted**. Operating on a
foreign container requires an explicit command and confirmation, and says plainly
what will be lost.

This is the difference that shapes the rest of the design: cxz is not a client for
containers that already exist, it is the owner of containers it made.

## The chain

```
client → manager's private socket (Docker exec stdio)
       → project network, with a capability
       → project runtime
       → session supervisor
       → agent
```

No host port is published. Each hop narrows what the next one can reach: the
capability is scoped to a project, the runtime is scoped to its own project, and the
supervisor to one session. A project container has no manager socket and no
credential directory mounted — an agent cannot reach your other projects or
accounts.

Project TCP is authenticated, not encrypted. The Docker operator is trusted.

## Independence

The supervisor owns the agent, and it is not a child of the manager or of any
frontend. That is what makes the TUI disposable:

- Closing the TUI does nothing to the conversation.
- Replacing the manager reconnects to project networks; agents keep running.
- Two frontends can watch one session, because pending state is server state.

The manager is replaced only when every running session in the installation is
safely idle, which is why updates wait rather than interrupt. See
[updates](updates.md).

## Durability

Each session has a **journal**: an fsynced, append-only, ordered record of inputs,
replies, tool activity, approvals and state changes. With the manifests beside it,
that is what recovery is driven from.

SQLite holds projections built from journals — the runtime registry and event cache
in `cxz.db`, resources and audit rows in `resources.db`. Projections can be rebuilt
from journals; resource metadata and audit history cannot. Back up the volumes.

Journal reads are incremental: the runtime remembers the byte offset and sequence it
last projected and reads only complete appended records, so loading a long history
never replays the whole file. The cursor advances only after the database
transaction commits, and an incomplete trailing record waits for the next read. A
journal shorter than what the database has already recorded is an error, not a
silent truncation.

## Resource API

The server is built on payday's generated resource framework. Definitions live in
`proto/cxz`, lifecycle extensions in `proto/ext/cxz`, and `go tool pd gen .`
generates `resource/`, `internal/ent/`, `server/bare/` and `server/pd/`. The
handwritten layer is `server/lifecycle/`: generated sink → publish interceptor →
lifecycle → audit and gate.

| | |
|---|---|
| `ProjectService` | `Add/Get/List/Watch` for registration; `Up/Down/Recreate` for containers; `Paths` streams container directory entries for path completion, as the project's remote user. `Up` does not create a session. |
| `SessionService` | `Add/Get/List/Watch` for conversations; `Resume/Send/Reply/Interrupt/Stop` for runs. |

Resource `Watch` subscribes to explicit resource references. The conversation
journal is separate and durable, read through `Events`/`History` with sequence
cursors — do not confuse the two.

Both resources are payday `global` entities: there is no fabricated tenant or user.
Project `Patch` accepts only name, alias and description, with optimistic version
checks. General `Patch`/`Apply`/`Erase` is closed, and runtime status is not
caller-writable. The API carries no version suffix and no compatibility aliases.

`api/` and `internal/runtimeproto/` define internal runtime view models in a
separate `cxz.runtime` namespace, which is not registered publicly. Runtime session
and vendor IDs are distinct from payday resource IDs.

## Keeping the UI cheap

The TUI subscribes rather than polls. The one-second animation timer never issues a
list RPC.

- Project and session IDs are subscribed through payday `Watch`, in filter batches.
- Change notifications coalesce over 150 ms into a single refresh; notifications
  arriving during one collapse into a single follow-up.
- Changing the subscription set cancels the previous stream, and messages from a
  stale generation are ignored.
- Failures back off from one to thirty seconds and resubscribe with a snapshot.
- A 30-second safety refresh catches what `Watch` cannot see: resources created by
  another client, and account or binding changes.
- The session list returns its related project, account and binding together, so
  there is no per-session `Get`.

On the server side, a Linux project runtime watches journals with inotify and
refreshes only the affected session, coalescing file events over 100 ms. Token
output that only moves `LastSeq` does not produce a list-change notification. A
two-second local liveness check on known PIDs catches deaths that wrote no final
journal record — it is not a Docker or API call. Other platforms rely on the
30-second reconcile.

The manager keeps the project event stream open and caches events before forwarding
them, so a page of 128 contiguous cached events is served from SQLite without
touching the runtime.

## Failure modes worth designing around

- **Container loss** ends its processes and its writable layer. `project up` or
  `session resume` starts a new run and resumes the conversation. Stale approvals
  from the old run fail; old prompts are never replayed.
- **A crash mid-tool-call** can leave `delivery_unknown`. There is no exactly-once
  guarantee; inspect the history before repeating a task.
- **Abrupt loss** can leave manager history incomplete until `project up` recovers
  it. `down` and `project down` collect final history first, which is why they are
  preferable to killing containers.
- **A version gap** between manager and project runtime is expected during updates.
  Preference pushes a runtime does not understand are skipped rather than failing —
  otherwise the project would be unusable until it updated, and it only updates when
  its sessions are idle.
