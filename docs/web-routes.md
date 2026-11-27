# Web addresses

Connection-owned pages live under `/c/@alias/` or `/c/<connection UUID>/`.
Generated links use the connection alias. The UUID survives an alias rename.
Opening `/` selects the current default connection, then redirects to its
explicit address. Unavailable aliases and UUIDs show an unavailable page,
including the retired `/c/_/` form.

A directly served installation defaults to `@local`. Set `connection_alias`
and `connection_id` in the Web gateway configuration to assign an explicit
identity. The development hub lists its configured connections and accepts both
forms on its scoped API paths.

Resource path segments use singular names. Generated links prefer a resource's
alias and fall back to its UUID. The same segment accepts either form; a UUID is
always interpreted as an ID. Resolving an address does not change the stable IDs
used for conversation state, drafts, subscriptions or mutations.

| Page                  | Address                                                 |
| --------------------- | ------------------------------------------------------- |
| Session library       | `/c/@local/session`                                          |
| Session               | `/c/@local/session/oak`                                      |
| Session by UUID       | `/c/@local/session/00000000-0000-8000-8007-000000000001`     |
| Conversation position | `/c/@local/session/oak?seq=120`                              |
| Project dashboard     | `/c/@local/project/cxz`                                      |
| Account               | `/c/@local/account/work1`                                    |
| Project task table    | `/c/@local/project/cxz/task`                                 |
| Project task          | `/c/@local/project/cxz/task/fix-login`                       |
| Project memories      | `/c/@local/project/{project alias or ID}/memory`             |
| Memory document       | `/c/@local/project/{project alias or ID}/memory/{memory ID}` |
| Search                | `/c/@local/search`                                           |
| Auxiliary AI          | `/c/@local/auxiliary`                                        |

Memory paths accept project aliases or UUIDs. Generated links prefer the project
alias and fall back to its UUID; the document segment always uses its UUID.
The project collection shows its document list without automatically selecting a document. Selecting a document
updates the address; deep links, reload and browser Back/Forward restore that
document. Unsaved edits require confirmation before leaving. Unknown projects or
documents show an unavailable state instead of opening another document. The old
`/memory` and `/c/@local/memory` addresses have been removed.

Conversation usage reports are available on the project and account pages.

Project, account and task references accept UUIDs as well. Task storage is
project-scoped, so both the project and task references live in the path; each
accepts an alias or UUID and generated links prefer aliases. Session and run
filters remain query parameters. With several projects,
a task address lacking that scope asks the user to choose a project. Collection
addresses omit the resource reference.

The workspace sidebar carries the currently viewed project into Projects, Tasks
and Shared memory links. A session's project becomes current when its
conversation is opened; choosing a project on another page updates that scope.
The workspace remembers its stable ID while visiting Accounts or Settings, and
generates links using its latest alias. Explicit addresses and browser history
still determine the displayed selection. An unscoped task detail still requires
a project choice when several projects exist.

Browser-wide settings live at `/setting/general`, `/setting/editor` and
`/setting/connection`, outside the connection namespace.

Old plural paths and project/account/task query selections redirect to the new
addresses. Existing runtime session handles remain readable for old bookmarks.
A UUID link continues to identify its resource after an alias is renamed; an old
alias bookmark is not a permanent identifier and may stop resolving after a
rename or alias reuse. Renaming the currently open project or session updates its
alias address without changing the selected resource.

The HTTP server serves the SPA shell for HTML navigation under `/c/` and
`/setting/`; API, authentication, attachment and static asset endpoints retain
their existing paths and error behavior. URL scoping does not add cross-origin
proxying or change authentication.
