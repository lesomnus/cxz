# Client feature inventory

This is the living inventory of user-visible cxz capabilities across clients.
Use it when changing the CLI/TUI and when building the mobile web client so that
missing functionality and intentional differences remain visible. It describes
behavior, not identical layouts or keyboard shortcuts.

Initial source audit: main `df35808` (2026-10-03). This is an inventory baseline,
not a claim that the web client exists. Detailed linked guides and implementation
remain the authority for individual behaviors; correct this inventory when they
change. Internal process entry points are not user-facing features.

## Mobile direction and status

Confirmed direction: start with a web client accessed through **HTTPS inside a
VPN**. Native mobile apps and public Internet exposure are not initial targets.
The web client will use the existing Manager/project/session authority. Closing a
page must not stop an agent or resolve a pending request.

All web features below are **Not built** at this baseline. The Web column records
proposed scope, not implementation status or an agreed release commitment:

- **First**: proposed first usable mobile release.
- **Later**: tracked gap, proposed follow-up; not silently dropped.
- **Adapt**: needs a browser-specific interaction or design before scheduling.
- **Host**: intentionally remains in the installed CLI/TUI or host configuration.

When implementation lands, change the cell to `Done (PR/link)` or
`Partial (PR/link; exact missing behavior)`. Keep the original feature ID. A
capability is not done merely because its button or endpoint exists.

## Projects, sessions, and navigation

| ID | Capability and current entry points | Web proposal / gap |
|---|---|---|
| NAV-01 | Project/session lists, names, aliases, agent and running/pending state; TUI list, `project ls`, `session ls/get` | First; down projects stay out of the ordinary TUI-style active list, without making kept data appear deleted |
| NAV-02 | Open/switch sessions; session-only back/forward history in TUI | First; touch navigation and browser history must not discard drafts |
| SES-01 | Create session with account selection and required login; TUI creation, `session new` | Later; initially use sessions/accounts prepared on desktop |
| SES-02 | Rename sessions/projects and set aliases; TUI and project/session metadata commands | Later |
| SES-03 | Interrupt current turn, stop agent, resume session, confirmed restart; TUI controls, `/stop`, `/restart`, session commands | First; interrupt, stop, restart, and detach remain distinct |
| SES-04 | Delete/archive session versus permanent `session purge`; TUI deletion and CLI purge | Later; preview scope and retain existing confirmation semantics |
| PRJ-01 | Register/provision/up, down, recreate and project purge; project CLI and available TUI lifecycle controls | Later; show workspace/container/data consequences before mutation |
| PRJ-02 | User devcontainer, default template overrides, project-name substitution and rendered configuration; host files, `devcontainer render/docker-compose` | Host; Later for read-only inspection of effective configuration |

References: [projects](projects.md), [CLI](cli.md), [TUI](tui.md),
[project commands](../cmd/cxz/projects.go),
[session navigation](../internal/tui/session_navigation.go).

## Conversation, input, and files

| ID | Capability and current entry points | Web proposal / gap |
|---|---|---|
| CHAT-01 | Stream assistant output and tool activity; show working, completed, failed and interrupted state | First; stream updates without duplicating events |
| CHAT-02 | Markdown/GFM, code highlighting, links, code copy and text selection | First; copy source code without display padding; touch selection may differ from TUI drag |
| CHAT-03 | Tool detail Input/Output tabs, raw request/result inspection; `/view`, `/details`, TUI tool preview | First; tap opens details instead of desktop double-click |
| CHAT-04 | Load retained history, scroll without losing position, pinned prompt, follow latest and jump-to-bottom | First; respect server retention and render a bounded window |
| CHAT-05 | Turn metrics and session usage; `/usage`, quota display | First; provider-reported fields only, missing data is not zero; auxiliary usage stays separate |
| CHAT-06 | Unified context Summary/Raw view; `/context` | Later; preserve provider availability and idle requirements |
| CHAT-07 | Native agent context compaction; `/compact` | Later; not journal retention or shared-memory compaction |
| CHAT-08 | Provider model/effort discovery and selection; `/model`, `/effort` | Later; catalog-driven choices, report unsupported capabilities |
| CHAT-09 | Provider background task inspection; `/background` | Later |
| INPUT-01 | Multiline editable draft, explicit send, session switching, selection/cursor editing; composer | First; preserve draft on disconnect/error; paste and dictation must not send |
| INPUT-02 | Local slash-command discovery/help; `/help` and command hints | First for supported actions; equivalent buttons are allowed, unsupported actions must be explicit |
| INPUT-03 | Paste chips, original text/file mode, preview, removal and `r` expansion; `/paste`, Ctrl+P | Later; preserve original content and insertion location, do not silently truncate |
| INPUT-04 | Container path completion and file previews in the composer | Later; paths refer to the selected project container |
| FILE-01 | Client file attachment, upload progress/retry/removal; backtick-! host file references | Adapt; browser file picker replaces arbitrary client filesystem paths; attach to the selected project |
| FILE-02 | Download container file, completion, cancel and transfer progress; `/download` | Adapt; browser download behavior replaces the CLI's local Downloads directory contract |
| INPUT-05 | Secret input via `@redact`; send protected file reference rather than secret prompt text | Adapt; current local/SSH transport rules do not establish a browser implementation; do not substitute a normal text field |
| INPUT-06 | OS keyboard dictation and ordinary text input | First as standard browser input; no cxz STT engine or custom microphone UI planned |

References: [TUI](tui.md), [history](history.md), [security](security.md),
[command registry](../internal/tui/commands.go),
[command help](../internal/tui/help.go), [Markdown renderer](../internal/tui/markdown.go),
[clipboard](../internal/tui/clipboard.go).

## Approvals, questions, and authentication

| ID | Capability and current entry points | Web proposal / gap |
|---|---|---|
| AUTH-01 | Connect to a Manager and distinguish client/remote identity; named CLI/TUI connections | First via VPN + HTTPS; browser login/session mechanism still needs design, VPN membership alone is not an application login |
| AUTH-02 | Account list, add/login/status, provider-specific central/project/session bindings; Accounts TUI and account CLI | Later; show authentication that the selected session actually needs |
| APPROVAL-01 | Pending request list, inspect payload, allow/deny; approval box, `/approval`, `session reply` | First; bind decisions to session/run/request; stale or already-resolved replies must not apply elsewhere |
| APPROVAL-02 | Questions with choices, multiselect where supported, Other, navigation and submit/cancel; `/answer` | First; cancellation is not denial; retain unsent answers during reconnect/navigation |
| APPROVAL-03 | Persisted FULL/ASK permission policy; `/permission` | First; policy remains server-owned and active while clients are closed |
| APPROVAL-04 | MCP consent, structured forms and URL elicitation, including fieldless direct decisions | First; preserve supported request semantics and FULL behavior; unknown forms must not become implicit approval |

References: [approvals](approvals.md), [accounts](accounts.md), [remote](remote.md),
[security](security.md), [question UI](../internal/tui/questions.go).

## Memory, extensions, and auxiliary AI

| ID | Capability and current entry points | Web proposal / gap |
|---|---|---|
| MEM-01 | Browse/search shared project Markdown and saved snapshots; `/memory` | Later; distinguish another session's memory, own memory and frozen copies |
| MEM-02 | Snapshot/copy/combine/rename, start session from memory, trash/restore/permanent delete, native Markdown import | Later; preserve source ownership and existing confirmation semantics |
| MEM-03 | Browse original provider memory/history; native-file view | Later; distinguish native files from shared cxz memory |
| MEM-04 | Conversational maintenance using review conditions, topic updates and revision checks | Shared agent behavior; no separate AI compact UI planned ([#36](https://github.com/lesomnus/cxz/issues/36) closed) |
| MCP-01 | MCP registration, global default activation, project overrides/inheritance; Settings and `mcp` CLI | Later; project override never changes another project |
| MCP-02 | MCP logs and restart; `mcp logs/restart` | Later |
| SKILL-01 | Skills library, registrations, global/project activation/inheritance; Settings and `skill` CLI | Later; host-side skill files remain host-managed |
| AI-01 | Task profiles: account/login/model/effort selection, enable/disable; Settings AI tasks and `ai` CLI | Later; reuse per-account auxiliary auth, no silent account/model fallback |
| AI-02 | Inline Markdown summaries, ghost suggestions, loading/error states; `/summary`, `/suggest` | Later; retain session on/off and one-shot semantics; accept suggestion into draft without sending |
| AI-03 | Auxiliary job usage/status/cancel; `ai status/cancel` | Later; cancel derived work without interrupting the agent |

References: [memory](memory.md), [MCP](mcp.md), [skills](skills.md),
[auxiliary AI](auxiliary-ai.md).

## Settings, operations, and platform differences

| ID | Capability and current entry points | Web proposal / gap |
|---|---|---|
| OPS-01 | Client/Manager versions, channels and upstream release status; Settings, `/settings` | First read-only; web asset version must be distinguished from Manager/runtime version |
| OPS-02 | History retention and client scroll-window budgets; Settings | Later; web can choose its rendering window but cannot override server retention silently |
| OPS-03 | Docker engine health/status/cache, activation/deactivation/cache cleanup; Settings and Docker CLI | Later; separate read-only status from disruptive maintenance |
| OPS-04 | Update check/policy, pin/channel switch, rollback; Settings and update/use/manager CLI | Host for installation/replacement; Later for explicitly scoped Manager operations |
| OPS-05 | Host configuration editing, file mappings/shared sources, gh credentials and gitconfig synchronization | Host; browser must not pretend mobile files are Manager-host configuration |
| OPS-06 | Session/project diagnostics and provisioning logs; `/logs`, project/manager CLI | Later; bounded, scrollable output with source identity |
| OPS-07 | Container shell/exec and embedded terminal; `/terminal`, project shell/exec | Adapt; current TUI local Docker access does not provide a browser terminal transport |
| OPS-08 | TUI diagnostic recording (`/record`), terminal-info, shell completion, Windows Terminal integration | Host; a web diagnostic facility would be a separate capability |
| OPS-09 | Install/uninstall Manager and installation-wide purge | Host; not part of the initial mobile application |
| WEB-01 | HTTPS endpoint, trusted certificate, browser authentication/logout and expired-session handling | First; VPN-only deployment, transport/auth design pending |
| WEB-02 | Reconnect from event cursor, handle retained-history gaps, refresh state and pending requests | First; page suspension/disconnection must not affect agent lifetime |
| WEB-03 | Safe send/decision recovery after a lost response; duplicate prevention across clients | First; do not blindly resend mutations or assume a disconnected send failed |
| WEB-04 | Mobile viewport, touch targets, soft keyboard, accessible controls, background/resume | First; no keyboard-only actions in the first-release scope |
| WEB-05 | Home-screen installation and web push for completion/approval waiting | Later; permission, subscription lifecycle and notification content need design; not required for initial foreground web use |
| WEB-06 | Multiple Managers in one web client, native SSH connection management, native mobile shell | Deferred design; do not infer support from existing desktop named connections |

References: [settings](settings.md), [Docker](docker.md), [updates](updates.md),
[installation](installation.md), [development](development.md),
[architecture](architecture.md). WEB rows are proposed capabilities, not existing
endpoints or implementations.

## Shared behavioral contract

The web client must reuse server-side operations and persisted state, rather than
reimplement agent lifecycle, approval policy, auxiliary task execution, or memory
ownership in browser code. Presentation can differ: a touch action can replace a
shortcut, and a file picker can replace a local-path marker.

Validate each shipped capability against the same scenario in both clients:

- Observe the same session from TUI and web; status, messages and pending requests
  converge. A reply resolved in one client disappears or becomes resolved in the other.
- Disconnect or suspend during streaming; reconnect without duplicate output and
  with an explicit fallback when the requested cursor is no longer retained.
- Lose the response to a send or approval; recover without duplicating work or
  applying a decision to a new run. Distinguish pending, accepted and failed states.
- Preserve drafts on session navigation, transient errors, reconnect and frontend
  refresh. Do not persist secrets with ordinary drafts.
- Copy code as content, not presentation padding. Raw provider payloads remain
  available where the current feature offers them; absent metrics remain absent.
- Show unavailable/unsupported operations explicitly. Do not silently invoke a
  different account, model, session or project as a fallback.

Before calling the First scope complete, agree the proposed rows, record the
chosen authentication/API transport and draft storage policy, and run the mapped
scenarios on an actual target phone plus the desktop TUI. Notification delivery,
file transfer, secret input and terminals require their own acceptance criteria
when scheduled; a generic chat smoke test does not establish their parity.

## Keeping the inventory current

For every user-visible feature PR:

1. Add or update the relevant stable ID, including behavior changes and removals.
2. State which clients implement it. Record intentional gaps as Later, Adapt or
   Host with a reason; do not equate an available server RPC with a completed UI.
3. Link the detailed guide/source and implementation PR or issue. When a proposal
   is split into an issue, retain its ID here and link the issue rather than
   maintaining a second conflicting checklist.
4. Update the shared behavior scenario or the documented platform exception.
   Add appropriate implementation tests when behavior changes; documentation-only
   edits do not need artificial runtime tests.
5. Review this page in the same PR. Never mark web support complete based only on
   a mockup, an open PR, or terminal support for a similarly named command.

A row marked Later is a tracked difference, not a promise of a release date.
Deleting a capability requires an explicit decision and migration/compatibility
notes where applicable, rather than removing its row to make the matrix look complete.
