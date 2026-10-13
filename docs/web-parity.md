# TUI features in the web workspace

The web uses the Manager's existing resource APIs. Browser login bridges only
ephemeral provider input and output; it does not add a second session database,
authentication store, search index, or memory library.

## Resource views

- **Sessions**: create a session in a registered project, choose an account and
  model, archive a conversation, and restore it. The session panel's management
  link opens this view even while another conversation is selected. Titles and
  aliases remain editable in session details; runtime controls stay in the
  composer menu.
- **Accounts**: register isolated Codex or Claude authentication profiles and log
  in to a central Codex account. Claude credentials belong to each session.
  The panel shows agent logo, display name and alias. Its add button opens a
  form beside the panel; selecting a profile shows its metadata, listed sessions
  and recorded usage in the main area. `AccountService.Status` reports saved
  login readiness and its scope; a logged-in central account does not offer an
  active login action. `AccountService.Logout` removes the central saved login
  and future token-supply authorizations after confirmation. Session-scoped
  profiles direct login management to their sessions.
  Missing session login opens the provider workflow before creation is retried
  with the same creation key. Other authentication errors are not retried.
  An existing Claude session can log in again from its menu after stopping it.
  Credentials and login output are not persisted in browser storage or journals.
- **Search**: search the server's retained conversation index across projects,
  filter by project, match type, tools, case and UTC date window, continue result
  pages, and open the matched event. Index progress, unavailable projects and
  trimmed conversations are reported instead of appearing complete.
- **Shared memory**: select a session's project library, search memories, read
  documents and snapshots, edit this session's own documents with revision
  checks, create documents, snapshot, fork, merge, rename, import, move to trash,
  restore and purge. Other sessions' documents and snapshots are read only.
  A save conflict preserves the editor's draft. Destructive actions and discarding
  document changes require an explicit confirmation.
- **Auxiliary AI**: configure the account, model and effort for summary,
  suggestion and title tasks, including the helper's provider login. Each session
  has automatic execution preferences, manual execution, cancellation, results
  and task usage. A suggestion can be inserted into the composer.
- **Activity**: inspect active sessions and pending requests, retained conversation
  statistics with coverage information, and provider-reported background work.
  The statistics window is explained in the view; token and cost coverage are
  separate from billing and account lifetime usage. Background work is refreshed
  only while its view is open and the tab is visible.

## Composer support

The composer menu and local slash commands open shared memory, usage, background
work and auxiliary controls without sending these commands to the agent.

`@alias` suggestions contain only other listed sessions in the current project.
The caret remains in the editor; the suggestion matrix supports directional
navigation, Enter/Tab acceptance and Escape dismissal. The inserted alias is
ordinary prompt text, just as in the TUI.

`/@redact` and the composer menu's secret action create a secret file through the
existing Manager API. A chip replaces the typed directive and only the file
reference enters the prompt and saved draft. Secret bytes are never saved in
browser storage. Chips are bound to the session and run; an earlier run's chip
cannot be sent. The agent can read the referenced file.

A queued message stays outside the transcript until it is delivered. Its current
text is recovered from the session snapshot and kept up to date by live events.
Cancel removes it; Recall returns it to an empty editor after cancellation is
confirmed by the server. A queued receipt does not animate a new conversation
card or force a scroll jump.

## Remaining TUI-only interfaces

- Permission-policy selection and provider catalog refresh/provenance controls.
- Docker/devcontainer, MCP and skills registration, history retention and version
  maintenance settings.
- Session/Manager logs, MCP reconnection, and debug recording.
- MCP requested-schema forms and their dedicated accept/decline/cancel workflow.
- Composer multicursor, line movement and line duplication shortcuts.
- TUI host-side filesystem workflows and multiple Manager connections.

This list describes interfaces still missing from the web, not absent Manager
capabilities. Normal approval questions, uploads, paths/downloads, terminal panes,
model/effort selection and conversation summaries already have web interfaces.

## Development and rollout

The frontend needs the updated web server for `/login/` WebSockets and the direct
Manager Connect structure. A Vite refresh alone does not update an installed Go
web server. Provider login requires a real Manager; isolated resource stories
use typed in-memory clients and do not contact providers. The WASM simulation
still covers its original conversation scenarios and reports unavailable APIs
for resource features it does not simulate.

Resource components are grouped under `ts/src/features`, route views under
`ts/src/pages`, and shared presentation under `ts/src/shared/components`.
They reuse the UI package's menus, buttons, dialogs and source editor. CSS tokens
and component implementations remain the source of truth for layout and timing.
