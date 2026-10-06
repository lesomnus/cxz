# Client support matrix

Current implementation status for the stable IDs in [the feature specification](feature-spec.md).
Scheduling belongs in [the mobile web plan](plans/mobile-web.md), not this table.

Baseline audited against main `df35808` on 2026-10-03. This is source/documentation
coverage, not a claim that every combination has been exercised on every device.
All web cells are **Not implemented**: an existing server RPC is not a web UI.
Open PRs remain pending until merged. In particular,
[#79](https://github.com/lesomnus/cxz/pull/79) is not counted as shipped here.

- **Supported**: the surface implements the stated capability, subject to noted
  provider, host or transport limits.
- **Partial**: only the listed subset is implemented; the missing portion is named.
- **Not implemented**: no corresponding user-facing implementation was found.
- **N/A**: the interaction does not apply to that client, not a claim of support.
- **Shared / External**: behavior comes from agent/server guidance or the OS,
  respectively, rather than a dedicated client feature.

CLI means a user-facing `cxz` subcommand. Internal entry points, raw RPC access,
and sending slash text to an agent are not dedicated CLI support. TUI means the
interactive `cxz` frontend, including slash commands. Linux host operations are
not automatically supported by the Windows frontend. A deliberate exclusion is
explained in the mobile plan; it remains Not implemented in the web column.

## Projects, sessions, and navigation

| ID | CLI | TUI | Web |
|---|---|---|---|
| NAV-01 | Partial: `project ls`, `session ls/get`; includes down/foreign projects, no interactive active-list filtering | Supported: project/session lists, active-project filtering, hover + creation shortcut and ? help | Not implemented |
| NAV-02 | N/A: one-shot commands have no view history | Supported: session navigation and Alt+Left/Right history | Not implemented |
| SES-01 | Supported: `session new`, account flow | Supported: new-session account/login flow | Not implemented |
| SES-02 | Partial: `project add/set`, creation-time session metadata; no standalone session rename command | Partial: session alias rename; no project metadata editor | Not implemented |
| SES-03 | Partial: `session interrupt/stop/resume`; restart requires explicit stop then resume | Supported: lifecycle controls, `/stop`, `/restart`, detach; pending resume indicator | Not implemented |
| SES-04 | Partial: `session purge` with preview/confirmation; no ordinary removal command | Partial: stop/delete and retained journal; permanent purge remains CLI | Not implemented |
| PRJ-01 | Supported: `project add/up/down/recreate/purge` | Partial: provisioning through session creation; no complete project lifecycle menu | Not implemented |
| PRJ-02 | Supported: host configuration, `devcontainer render/docker-compose` | Partial: consumes provisioning configuration; no template editor/render command | Not implemented |

## Conversation, input, and files

| ID | CLI | TUI | Web |
|---|---|---|---|
| CHAT-01 | Partial: `session events` raw stream and `session get`, no conversation renderer | Supported: streamed transcript and state | Not implemented |
| CHAT-02 | N/A: raw command output, no Markdown conversation UI | Supported: Markdown and copy controls | Not implemented |
| CHAT-03 | Partial: raw `session events`; no input/output inspection UI | Supported: `/view`, `/details`, tool preview tabs | Not implemented |
| CHAT-04 | Partial: `session events ID AFTER_SEQ`; no viewport navigation | Supported: history window and navigation controls | Not implemented |
| CHAT-05 | Partial: recorded event telemetry; no dedicated session usage report command | Supported: turn metrics, `/usage` and quota display | Not implemented |
| CHAT-06 | Not implemented: no dedicated context command | Supported: `/context`, Summary/Raw | Not implemented |
| CHAT-07 | Not implemented: no dedicated compact command | Supported: `/compact`, provider/idle constraints apply | Not implemented |
| CHAT-08 | Partial: session creation model and configuration defaults; no session catalog/effort command | Supported: `/model`, `/effort` where provider supports them | Not implemented |
| CHAT-09 | Not implemented: no dedicated background-task command | Supported: `/background` where reported | Not implemented |
| INPUT-01 | Partial: `session send` text argument; editing belongs to caller/shell | Supported: composer, selection, per-session drafts, line move/duplicate/indent, auto-indent, Undo/Redo, multi-click selection, cursor-preserving wheel scroll, logical-line gutter and Alt+W whitespace display | Not implemented |
| INPUT-02 | Supported: generated `--help` and completion for CLI actions | Supported: `/help` and slash hints | Not implemented |
| INPUT-03 | N/A: no composer/chips | Supported: `/paste`, Ctrl+P, text/file modes and expansion | Not implemented |
| INPUT-04 | N/A: no conversation composer | Supported: path hints and previews | Not implemented |
| FILE-01 | Not implemented: no user-facing attachment command | Supported: host-file references and upload chips | Not implemented |
| FILE-02 | Not implemented: no user-facing download command | Supported: `/download` and transfer progress/cancel | Not implemented |
| INPUT-05 | Not implemented: no user-facing redact command | Supported: `/@redact` on supported local/SSH transports; plaintext TCP refused | Not implemented |
| INPUT-06 | External: shell/terminal text input; no cxz STT | External: OS/terminal text input; Win+H observed working, no cxz STT | Not implemented |
| INPUT-07 | N/A: shell supplies text | Supported: `@` alias grid with filtering, keyboard/mouse selection and title preview | Not implemented |

## Approvals, questions, and authentication

| ID | CLI | TUI | Web |
|---|---|---|---|
| AUTH-01 | Supported: endpoint/named-connection flags, `connection ls/enroll/check/forget`, mutual-TLS connections and foreground SSH `connection tunnel` on Windows/Linux; HTTPS identity remains unchanged | Supported: same connection routing and identity display | Not implemented |
| AUTH-02 | Supported: account/backend/binding commands; provider/platform limits apply | Partial: Accounts and login workflows, no full backend/binding administration UI | Not implemented |
| APPROVAL-01 | Partial: `session get/events/reply`; raw inspection only | Partial: pending `/approval` and decisions; completed row inspection pending #79 | Not implemented |
| APPROVAL-02 | Partial: `session reply` structured JSON, no question UI | Supported: `/answer` structured questions | Not implemented |
| APPROVAL-03 | Not implemented: no dedicated permission-policy command | Supported: `/permission`, server-owned policy | Not implemented |
| APPROVAL-04 | Partial: raw request/reply flow, no dedicated elicitation UI | Partial: consent/form/URL handling; transcript identity/message fixes pending #79 | Not implemented |

## Memory, extensions, and auxiliary AI

| ID | CLI | TUI | Web |
|---|---|---|---|
| MEM-01 | Not implemented: no public memory-library command | Supported: `/memory` browsing/search | Not implemented |
| MEM-02 | Not implemented: no public memory-library lifecycle commands | Supported: library lifecycle and session-from-copy flow | Not implemented |
| MEM-03 | Not implemented: no dedicated native-memory browser | Supported: original agent file browser from memory view | Not implemented |
| MEM-04 | Shared: agent/MCP guidance, no compact command for shared memory | Shared: agent/MCP guidance, no separate compact UI | Not implemented |
| MEM-05 | Shared: built-in MCP tools; no dedicated CLI command | Shared: agent can invoke MCP tools; no dedicated transcript search view | Shared: agent can invoke MCP tools; no dedicated transcript search view |
| MCP-01 | Supported: `mcp list/add/remove/enable/disable/inherit` | Partial: Settings list/add/toggle/inherit; no removal action | Not implemented |
| MCP-02 | Supported: `mcp logs/restart` | Not implemented: no dedicated MCP log/restart controls | Not implemented |
| SKILL-01 | Supported: `skill list/add/remove/enable/disable/inherit` | Not implemented: no Skills Settings page in audited source | Not implemented |
| AI-01 | Supported: `ai list/models/login/set/disable` | Supported: Settings AI tasks account/login/model/effort flow | Not implemented |
| AI-02 | Partial: `ai status` returns derived data; no inline/ghost or per-session command UI | Supported: `/summary`, `/suggest`, inline Markdown and editable ghost acceptance | Not implemented |
| AI-04 | Supported: `ai title`, `ai title --text`, `ai set title` | Supported: AI tasks title profile, `/title`, `/title set`, two-row sidebar | Not implemented |
| AI-03 | Supported: `ai status/cancel` | Partial: loading/errors and cancellation via turning tasks off; no full usage/status/cancel panel | Not implemented |

## Settings, operations, and platform differences

| ID | CLI | TUI | Web |
|---|---|---|---|
| OPS-01 | Supported: `version`, `use`, `self-update status/check` | Supported: Settings / `/settings` versions and channels | Not implemented |
| OPS-02 | Partial: host configuration; no equivalent interactive history budget controls | Supported: Settings server retention/client scroll budgets | Not implemented |
| OPS-03 | Partial: `docker status/up/down/sync`; no dedicated cache-prune command | Supported: engine status, activation/deactivation and build-cache cleanup | Not implemented |
| OPS-04 | Supported: update/use/manager commands; host/platform restrictions apply | Partial: frontend update toggle/status; pin/switch/rollback remain CLI | Not implemented |
| OPS-05 | Supported: `edit`, `config`, `github sync`, `gitconfig sync` on appropriate host | Not implemented: no host configuration/file-sync editor | Not implemented |
| OPS-06 | Supported: Manager/project logs and doctor commands | Supported: `/logs` session/project reports | Not implemented |
| OPS-07 | Supported: `project shell/exec` with required host Docker access | Supported: `/terminal` with required local Docker access; no persistence after detach | Not implemented |
| OPS-08 | Partial: `terminal-info`, completion, Windows integration commands; no TUI recording command | Partial: `/record`/F9; shell completion and terminal registration remain CLI | Not implemented |
| OPS-09 | Supported: Manager and optional web gateway installation/uninstallation; web JSON settings and running-gateway refresh on host self-update/use; purge on supported host | Not implemented: no installation-wide administration UI | Not implemented |
| WEB-01 | N/A: browser-specific requirement | N/A: browser-specific requirement | Not implemented |
| WEB-02 | N/A: browser-specific requirement | N/A: browser-specific requirement | Not implemented |
| WEB-03 | N/A: browser-specific requirement | N/A: browser-specific requirement | Not implemented |
| WEB-04 | N/A: browser-specific requirement | N/A: browser-specific requirement | Not implemented |
| WEB-05 | N/A: browser-specific requirement | N/A: browser-specific requirement | Not implemented |
| WEB-06 | N/A: browser-specific requirement | N/A: browser-specific requirement | Not implemented |
| WEB-07 | N/A: developer npm workflow, not a cxz CLI command | N/A | Supported: separate payday WASM design sandbox with shared UI, scenario sessions, seed/pace and reset; production server not required |

## Evidence and updates

Audit entry points in [CLI reference](cli.md), [CLI command construction](../cmd/cxz/commands.go),
[TUI slash registry](../internal/tui/commands.go), [TUI help](../internal/tui/help.go),
[Settings actions](../internal/tui/settings_page.go),
[MCP Settings](../internal/tui/settings_mcp.go), [skill commands](../cmd/cxz/skill.go),
[memory library](../internal/tui/memory_library.go),
[auxiliary AI](auxiliary-ai.md), and [remote platform restrictions](remote.md).

For a changed cell, include the implemented entry point and a merged PR/source
reference; describe the exact remainder if Partial. Do not replace implementation
status with First/Later or treat an open PR as complete. Re-audit the affected
cell when behavior changes, and update its contract only when the meaning changes.
