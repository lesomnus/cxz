# Web addresses

Connection-owned pages live under `/c/<connection>/`. The reserved connection
reference `_` means the installation serving the current web origin. It does
not follow a mutable default connection selection. The browser currently
supports only this self connection; other names show an unavailable page and
never resolve against the local installation.

Resource path segments use singular names. Generated links prefer a resource's
alias and fall back to its UUID. The same segment accepts either form; a UUID is
always interpreted as an ID. Resolving an address does not change the stable IDs
used for conversation state, drafts, subscriptions or mutations.

| Page                  | Address                                             |
| --------------------- | --------------------------------------------------- |
| Session library       | `/c/_/session`                                      |
| Session               | `/c/_/session/oak`                                  |
| Session by UUID       | `/c/_/session/00000000-0000-8000-8007-000000000001` |
| Conversation position | `/c/_/session/oak?seq=120`                          |
| Project dashboard     | `/c/_/project/cxz`                                  |
| Account               | `/c/_/account/work1`                                |
| Project task board    | `/c/_/task?project=cxz`                             |
| Project task          | `/c/_/task/fix-login?project=cxz`                   |
| Shared memory         | `/c/_/memory?project=cxz`                           |
| Search                | `/c/_/search`                                       |
| Activity              | `/c/_/activity`                                     |
| Auxiliary AI          | `/c/_/auxiliary`                                    |

Project, account and task references accept UUIDs as well. Task storage is
project-scoped, so task links include a project reference. With several projects,
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
`/setting/connection`, outside the connection namespace. `/` opens
`/c/_/session`.

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
