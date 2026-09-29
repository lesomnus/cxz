# Projects and sessions

A **project** is a registered workspace directory plus the container cxz owns for
it. A **session** is one conversation inside that project, with its own agent,
account, configuration and history.

Removing containers removes neither.

## Lifecycle

```sh
cxz project add --name "My Web App" --alias web .   # register; no container
cxz up .                                            # prepare the container
cxz session new --account personal .                # start a conversation
cxz down .                                          # remove containers
cxz project up web                                  # recreate and resume
cxz project recreate --yes web                      # rebuild from configuration
cxz project rm web                                 # preview permanent removal
cxz project rm --yes web                           # permanently remove this project
```

What survives what:

| | Containers | Writable layer | History | Volumes | Registration |
|---|---|---|---|---|---|
| `down` | gone | gone | kept | kept | kept |
| `project up` | new | new | kept | kept | kept |
| `project recreate` | new | **gone** | kept | kept | kept |
| `project rm --yes` | gone | gone | **gone** | owned project volumes **gone** | **gone** |
| `uninstall` | kept | kept | kept | kept | kept |
| `purge` | gone | gone | **gone** | **gone** | **gone** |

`up` prepares and returns — it never starts an agent and never replays an old
prompt. Use `Ctrl+R` in the TUI or `cxz session resume` to continue a conversation.

Files that exist only in a container's writable layer die with the container.
`recreate` detaches any editor attached to the old container.

`cxz up` prints provisioning checkpoints to stderr — configuration, resources,
image build, hooks, agent installation, runtime readiness — with long steps
repeating every ten seconds. It is stage reporting, not a percentage. `--format
json` keeps stdout clean.

## One live session per project

A project runs one agent at a time. Stop the current session (`s` in the project
list, or `cxz session stop`) before starting another. Sessions you are not running
stay resumable indefinitely.

## Names and aliases

Projects have a display **name** and a short unique **alias**. Sessions get an
alias too.

Default project names come from the directory. Aliases are derived: short names
stay short, multi-word names become initials (`my-web-app` → `mwa`), long single
words are truncated, and collisions get a stable suffix. Existing aliases are never
reassigned behind your back.

Explicit aliases are lowercase letters, digits and single hyphens, and are
case-normalised. A duplicate is rejected.

```sh
cxz project set --name "Production Web" --alias prod web
```

Session aliases are random three-to-seven-letter English words, globally unique.
Select a session in the project list and press `r` to change one; custom aliases
are three to seven lowercase letters. Deleting a session frees its alias and keeps
its journal. The word pool is finite — exhaustion is reported rather than silently
falling back to numbers.

Project arguments resolve in order: exact ID or path, then alias, then an
unambiguous display name. Display names may contain spaces and need not be unique.

Renaming changes nothing about identity — not the project, not the session, not the
container.

## Data commands

Table output by default, `--format json` for scripts:

```sh
cxz project ls
cxz session ls
cxz session get ID
cxz session send ID "text"
cxz session reply ID REQUEST_ID allow|deny [ANSWERS_JSON]
cxz session interrupt ID
cxz session resume ID
cxz session stop ID
cxz session events ID [AFTER_SEQ]
```

Event streams reconnect by cursor. Mutations are never blindly retried; session
APIs accept an idempotency key so you can retry them yourself safely.

## Inside a project container

Inside an owned project, `cxz` and `cxz session ls` are scoped to that project
only. No manager socket and no credential directory is mounted there — an agent
cannot reach your other projects or accounts.

## Purge

`purge` is the only command that deletes conversations and credentials.

```sh
cxz purge --dry-run                                  # list what would go
cxz purge --yes                                      # delete all categories
cxz purge --groups containers,projects,state --yes    # or some of them
```

Groups: `containers`, `projects`, `state`, `tools`, `networks`, `local`. Omitting
`--groups` means all of them.

Run it from the host, with native cxz servers and agents stopped. Containers are
removed before volumes, and ownership labels are rechecked as it goes — a partial
failure keeps the installation locator so you can retry.

Never removed: your workspace sources, your personal Claude and Codex login
directories outside cxz, the `cxz` binary, shared images and build caches, and any
Docker resource cxz did not label. Unknown files inside the state directory are
reported and left alone.

Purge does not touch other cxz installations and does not revoke tokens at the
provider. **Back up before running it.**

## Permanently removing one project

On the manager host, run `cxz project rm PROJECT` to review the target, then
`cxz project rm --yes PROJECT` to confirm. Flags come before the project argument.
Use an alias, runtime ID, unambiguous display name, or workspace path. Archived
projects can also be removed by ID or path without bringing them up again.

Removal stops the project's agents by removing its owned containers, then deletes
its cxz-owned volumes and dedicated networks. It also removes project/session
registry rows, related audit records, cached conversations, memories and snapshots
inside the project volume, attachment references, derived summary/suggestion
content, project MCP/skill overrides, and project token-supply grants.

Host workspace sources, global accounts and logins, global MCP/skill definitions,
other projects, the shared workspace network, shared Docker engine/images/build
cache, and unlabelled user-managed volumes/networks are preserved. Docker resources
must have both matching `cxz.owner` and `cxz.project` labels. Foreign endpoints or
volumes still in use cause an error; fix the reported conflict and repeat the same
command. Cleanup is not atomic: some containers or volumes may already be gone,
but registration remains available for retries until cleanup succeeds. Small
content-free guards can remain to reject late events/auxiliary work.

Registering the same directory after successful removal starts a fresh project;
old sessions and memory do not return. Existing backups are unaffected. This is
logical deletion, not secure erasure of free disk blocks.
