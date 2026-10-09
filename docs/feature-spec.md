# Feature specification

This document defines user-visible capabilities and their behavioral contracts,
independently of a client's layout or implementation schedule. Stable IDs connect
it to the [client support matrix](client-support.md) and
[mobile web plan](plans/mobile-web.md).

A requirement here is **not a claim of implemented support**. Current support,
partial implementations and pending PRs belong only in the support matrix. Mobile
priorities and unresolved delivery choices belong only in the plan. The detailed
linked guides explain existing behavior; correct these documents when code changes.

## Projects, sessions, and navigation

| ID | Capability | Behavioral contract |
|---|---|---|
| NAV-01 | Projects and sessions | List project/session identity, aliases, agent and state. Distinguish stopped or down resources from permanently deleted data. Web panels keep their heading outside the scrolling list, highlight the current session and reveal a handle-only scrollbar on panel hover. Panel handles share the conversation handle's color and enlarge on pointer approach. Hidden list content fades into the panel background at each obscured edge; faster scrolling lengthens the outgoing fade, then it settles. The heading and handle remain above the fade. |
| NAV-02 | Session navigation | Open and switch sessions, retain each draft, and navigate session-only back/forward history. |
| SES-01 | Session creation | Choose workspace, account and supported model; complete required provider authentication before starting a session. |
| SES-02 | Names and aliases | Change supported project/session metadata without changing resource identity. Validate aliases and report ambiguous targets. |
| SES-03 | Session lifecycle | Distinguish interrupting a turn, stopping an agent, resuming, confirmed restart, and client detach. Detach must leave server work running. Interactive clients show when a resume request is pending. |
| SES-04 | Session removal | Distinguish ordinary session removal/archive from permanent purge; explain retained data and confirm destructive scope. |
| PRJ-01 | Project lifecycle | Register, provision, start, down, recreate and purge owned projects with explicit workspace, container and data consequences. |
| PRJ-02 | Devcontainer configuration | Resolve project configuration, default templates and project-name variables; expose the effective rendered configuration. |

## Conversation, input, and files

| ID | Capability | Behavioral contract |
|---|---|---|
| CHAT-01 | Conversation events | Show assistant output, tool activity and turn state from recorded/streamed events without duplicating events on reconnect. Keep protocol raw records, polling and control bookkeeping out of the transcript while retaining them for replay and metadata. Represent successful turn completion in its loaded final response footer without a duplicate status row; preserve failure, interruption and unmatched notices. Fill sparse rendered history without requiring overflow first and preserve reading anchors during paging and resize. |
| CHAT-02 | Formatted responses and copying | Render Markdown/GFM, highlighted code and safe links; copy code without display padding while retaining source indentation. |
| CHAT-03 | Tool inspection | Inspect recorded tool input and output separately, including raw content when a normalized preview is unavailable. Group a tool call, correlated approval, output chunks and result by run and execution ID at the original call position; preserve unrelated requests and history-edge results. |
| CHAT-04 | History navigation | Load retained history in a bounded window, preserve browsing position and provide follow-latest, pinned prompt and jump-to-bottom navigation. Web history uses larger indexed pages and prefetches in the direction of travel before reaching the loaded edge, with a larger bounded cache and a virtualized DOM. |
| CHAT-05 | Usage and quota | Distinguish turn metrics, session totals, account quota and auxiliary usage. Show only provider-reported metrics; absent data is not zero. |
| CHAT-06 | Context report | Provide normalized Summary and original Raw context views while preserving provider availability and idle requirements. |
| CHAT-07 | Agent context compaction | Invoke native provider compaction on an eligible idle session. This does not prune the cxz journal or compact shared memory. |
| CHAT-08 | Model and effort | Discover provider model/effort choices and apply supported selections; show unsupported operations without inventing catalog entries. |
| CHAT-09 | Background tasks | Inspect background tasks reported by the provider; do not infer support from unrelated session state. |
| INPUT-01 | Draft editing and send | Edit multiline drafts, move/select text, retain per-session drafts and explicitly send. Paste and dictation must not submit automatically. |
| INPUT-02 | Action discovery | Expose local help and discoverable actions without sending help requests to the agent. UI controls may replace slash commands. |
| INPUT-03 | Pasted text | Represent large pastes as editable-position chips with original-text/file modes, preview, removal and expansion. Preserve original content or report inability to expand. |
| INPUT-04 | Container paths | Complete and preview paths in the selected project container; distinguish these paths from client and Manager-host files. |
| FILE-01 | File attachments | Upload client-selected files to the selected project with progress, retry and chip removal. Removing a reference need not delete a stored file. |
| FILE-02 | File downloads | Download a regular container file, report transfer progress and allow cancellation. Destination handling is client-specific. |
| INPUT-05 | Secret input | Collect a secret separately from ordinary prompt text and send a protected file reference. Enforce transport rules; do not degrade silently to plain prompt text. |
| INPUT-06 | External text input | Accept text produced by OS keyboard/dictation facilities where the environment supports them. cxz does not promise its own STT engine. |
| INPUT-07 | Session mentions | Discover other listed sessions within the current project, filter by alias and insert a plain-text mention without sending the draft or reading conversation bodies. |

## Approvals, questions, and authentication

| ID | Capability | Behavioral contract |
|---|---|---|
| AUTH-01 | Manager connection | Select and identify the connected Manager using a supported authenticated transport; inspect saved connections without contacting servers. Support explicit client-loopback SSH port forwarding with a visible foreground lifetime, preserving remote work on disconnect; do not imply that forwarding changes HTTPS identity. Distinguish local-client state from server state. |
| AUTH-02 | Agent accounts | Register, list, authenticate and inspect accounts while respecting provider-specific central/project/session credential bindings. |
| APPROVAL-01 | Approval decisions and inspection | List pending approvals, inspect original requests, and allow/deny the intended session/run/request. Retain inspectable completed approval records; approval is not execution success. |
| APPROVAL-02 | Questions | Support provider choices, supported multiselect, Other text, navigation and explicit submission. Canceling a dialog must not become denial. |
| APPROVAL-03 | Permission policy | Persist FULL/ASK policy on the server so it continues while clients are closed. Questions and unsupported requests still need their defined handling. |
| APPROVAL-04 | MCP elicitation | Handle consent, structured forms and URL requests, including fieldless decisions. Preserve server identity, message and raw request for inspection. |

## Memory, extensions, and auxiliary AI

| ID | Capability | Behavioral contract |
|---|---|---|
| MEM-01 | Shared memory browsing | Browse/search project Markdown and snapshots; distinguish own/other session memory and frozen copies. |
| MEM-02 | Memory lifecycle | Snapshot, copy, combine, rename, import native Markdown, start from memory, trash, restore and permanently remove with ownership and revision checks. |
| MEM-03 | Native agent data | Browse original provider memory/history separately from shared cxz Markdown. |
| MEM-04 | Memory maintenance | Maintain topic documents, review temporary information against recorded conditions and preserve valid decisions. This is agent guidance, not automatic deletion or a separate compact UI. |
| MEM-05 | Cross-session conversation reference | Resolve project session aliases to UUIDs; search retained events by text/RE2 and time using metadata-only results, then read selected events or export bounded JSONL snapshots. Preserve retention boundaries and secret-file references. |
| MCP-01 | MCP configuration | Register/remove servers; manage global activation defaults and independent project overrides/inheritance. |
| MCP-02 | MCP operations | Inspect server logs and reconnect the selected session MCP without implicitly restarting its agent. |
| SKILL-01 | Skills configuration | Manage skill registrations and global/project activation/inheritance; source files remain on the configured host. |
| AI-01 | Auxiliary profiles | Configure task account, authentication, model, effort and enabled state. Reuse the appropriate auxiliary auth and never silently switch profiles. |
| AI-02 | Summaries and suggestions | Show inline summaries and suggestion drafts with loading/errors. Preserve per-session on/off and one-shot behavior; accepting a suggestion does not send it. Keep a summary per turn for a bounded history, not only for the turn in progress, and deliver derived state as it is produced rather than by polling. |
| AI-04 | Session titles | Generate a draft on first input and finalize after three successful turns; allow manual naming/regeneration and preserve alias/UUID identity. |
| AI-03 | Auxiliary operations | Inspect derived task status/usage and cancel derived work independently of the source agent turn. A task record outlives the turn that produced it, within a bounded per-session history. |

## Settings, operations, and platform differences

| ID | Capability | Behavioral contract |
|---|---|---|
| OPS-01 | Version identity | Identify client, Manager and available upstream versions/channels; do not present one component version as all components. |
| OPS-02 | History budgets | Configure server display-history retention separately from the client rendering window; state when records are permanently pruned. |
| OPS-03 | Docker maintenance | Inspect engine health/cache and explicitly activate, deactivate or clean unused build cache with the appropriate confirmations. |
| OPS-04 | Updates and recovery | Check and configure updates, switch channel/pin and recover via rollback; report component-specific restart consequences. |
| OPS-05 | Host configuration | Edit host settings/shared sources/file mappings and synchronize gh credentials or gitconfig without confusing client files with server files. |
| OPS-06 | Diagnostics | Inspect session/project/Manager diagnostics and provisioning logs with source identity and bounded presentation. |
| OPS-07 | Container terminal | Provide shell/exec access where supported, distinguish shell lifecycle from agent lifecycle and disclose connection requirements. |
| OPS-08 | Frontend integration | Provide platform-specific diagnostic recording, terminal diagnostics, completion and terminal registration; do not treat these as server features. |
| OPS-09 | Installation lifecycle | Install/uninstall Manager and an optional persistent HTTPS web gateway; web settings accept file defaults and CLI overrides. Host self-update/version switching refreshes a running gateway while preserving settings and agent work (version switching still interrupts agents by its own contract). Purge installation-owned resources with explicit destructive scope. |
| WEB-01 | Browser access | Provide HTTPS, trusted certificate handling, browser authentication/logout and expired-session handling. Local Vite development may proxy the installed gateway and exchange its token file for a browser session server-side; never expose that token to UI assets or browser storage. |
| WEB-02 | Browser reconnection | Recover from suspension/disconnection using event cursors and current state; handle history gaps without stopping agents or resolving requests. |
| WEB-03 | Browser mutation recovery | Recover an unknown send/decision outcome without duplicate execution or applying a decision to a new run; do not blindly retry. |
| WEB-04 | Mobile interaction | Support phone viewports, touch, soft keyboard, accessible controls and background/resume without keyboard-only required actions. |
| WEB-05 | Installable web app and notifications | Support home-screen installation and completion/approval push with explicit permissions, subscription lifecycle and notification-content policy. |
| WEB-06 | Additional mobile connection modes | Evaluate multiple Managers, direct SSH and native shell integration separately; desktop connection support does not establish mobile support. |
| WEB-07 | Design sandbox | Preview the shared web UI with isolated simulated sessions and timed fake-agent events, reproducible seeds and reset. No real accounts, tools or project mutations; distinguish design fixtures from production integration tests. |
| WEB-08 | Workspace editor | At 1600px available width, show an 800px conversation and separate read-only project file preview. Explicit Connect opens the selected session project devcontainer through an authenticated IDE tunnel; isolate project state and support fake sandbox connections. See [contract](web-editor.md). |

## Shared contracts

Stateful clients use the same server authority for agent lifecycle, approvals,
auxiliary execution and memory ownership. Equivalent actions may have different
controls, such as touch buttons instead of shortcuts or file pickers instead of
client paths. A raw event stream alone does not implement a visual inspection UI.

- Concurrent clients converge on messages, session state and pending requests.
- Reconnection deduplicates events and explicitly handles expired history cursors.
- Mutation recovery does not blindly resend an operation with an unknown outcome.
- Unsupported operations and absent provider metrics are explicit. Never silently
  substitute an account, model, project or session.
- Preserve ordinary drafts during supported navigation and transient failure;
  keep secrets out of ordinary draft persistence. Persistence across page reload
  is a separate browser design decision in the mobile plan.
- Destructive actions distinguish retained state from irreversible deletion.
- Code copying separates source indentation from presentation padding.

## Detailed references

- [Projects](projects.md), [accounts](accounts.md), [remote access](remote.md),
  [CLI reference](cli.md), [TUI](tui.md).
- [Approvals](approvals.md), [history](history.md), [security](security.md).
- [Memory](memory.md), [MCP](mcp.md), [skills](skills.md),
  [auxiliary AI](auxiliary-ai.md).
- [Settings](settings.md), [Docker](docker.md), [updates](updates.md),
  [installation](installation.md), [architecture](architecture.md).

## Change discipline

Every user-visible feature PR updates the relevant contract here and its current
client cells in the support matrix. Keep IDs stable; describe removal explicitly
instead of deleting a row to hide a gap. Link implementation evidence and tests
where behavior changes. Update the mobile plan only when scope or sequencing
changes, not merely because a desktop implementation landed. Documentation-only
edits do not need artificial runtime tests.
