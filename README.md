# cxz

Claude Code and Codex in **cxz-owned devcontainers**, with durable history and a
reconnecting TUI. Go + xli CLI + payday gRPC + SQLite + Bubble Tea. Linux first.
The workflow succeeds [cld](https://github.com/lesomnus/cld), following
[cxz's ownership architecture](docs/architecture.md): foreign containers are
detected, never adopted.

## Start

Linux release binaries and checksums: [v0.1.0-rc.1](https://github.com/lesomnus/cxz/releases/tag/v0.1.0-rc.1).
This is a prerelease; Claude's second repeated-recreation memory check and native
arm64 runtime acceptance remain unverified. See its release notes before use.

```sh
CGO_ENABLED=0 go build -o bin/cxz ./cmd/cxz
bin/cxz install --workspace-root /absolute/directory/containing/your/projects
bin/cxz account add codex personal-codex
bin/cxz account login personal-codex   # central Codex login; no project needed
bin/cxz up .                          # project dashboard; n creates a session
```

`install` builds and starts a background Docker manager, waits for its API, and
returns. The host needs Docker and cxz; Node, devcontainer CLI and agent binaries
are managed in containers. `manager serve` is a **foreground, blocking development
server**, not the normal installation command.

The directory must exist under the installed root. `session new .` uses its
`devcontainer.json`, not an existing VS Code container. Missing configuration gets
a base Debian devcontainer with non-root `vscode` user. Multiple configurations
prompt in a terminal; scripts pass `--config`. Image, Dockerfile, Compose,
features and hooks delegate to the official devcontainer CLI.

Register separate profiles for personal/company subscriptions:

```sh
bin/cxz account add --name "Company Codex" codex work-codex
bin/cxz account login work-codex
bin/cxz account ls
bin/cxz backend ls             # supported agent/auth workflow mappings
bin/cxz binding ls work-codex   # metadata only; no tokens
bin/cxz session new --account work-codex .
```

Account is an agent authentication profile, not a cxz user/tenant. The manager's
resource DB holds profile metadata. Codex defaults to central login: log in once
per account, then supply access tokens to its connected projects. Claude keeps
independent logins for **every session**, including sessions using the same account.
Interactive session creation opens the login flow. To log in again, stop only that
session and use `account login --session SESSION_ALIAS ACCOUNT`.
Rotating refresh tokens are never copied across sessions or projects.
Host credentials are never imported implicitly. Account and agent are fixed for each session;
resume/recreate preserves them. Missing login fails instead of falling back to
environment credentials. Interactive `session new` and TUI Ctrl+N offer account selection;
scripts must pass `--account` when creating a session. Multiple Claude/Codex sessions
may run in the same workspace. Each has independent configuration, history, HOME,
cache and temporary directories; cxz does not create worktrees or coordinate edits.
See [Account design and boundaries](docs/accounts.md).

`Account.auth_backend` selects the authentication strategy; `Session.auth_binding`
fixes the concrete authentication association. Codex defaults to
`brokered-access-token`; Claude defaults to `project-local-oauth`. Codex also
supports explicitly selected `project-local-oauth`. Existing accounts keep their
selected backend. API keys remain unsupported. Central OAuth login and refresh
are delegated to official Codex; cxz does not implement OAuth endpoints.

The account determines the agent; a conflicting explicit `--agent` is rejected.
Use `cxz config set codex-model MODEL_ID` or `session new --model MODEL_ID .` for a new
session's model; reconnect/resume preserves it. `cxz manager doctor`, `cxz project logs PROJECT`
and `cxz version` provide diagnostics. See [operations and releases](docs/operations.md)
for retry behavior, model settings, versioned installation, updates and rollback.

The remote user must have write permission on the host workspace. Explicit
`remoteUser` settings are respected; cxz does not silently change repository file
ownership or remap an existing user's UID. Adjust the devcontainer for a host UID
other than the default image's 1000 when necessary.

```sh
bin/cxz up .                               # project information + session list
bin/cxz up --no-attach --format json .      # prepare project only; no Account required
bin/cxz it .                               # newest session in this workspace
bin/cxz session new --account work-codex .  # new conversation; stop an active one first
bin/cxz session attach PROJECT             # or SESSION_ID
bin/cxz tui                        # also: watch
bin/cxz project exec PROJECT -- go test ./...
bin/cxz project shell PROJECT
bin/cxz project down .                     # remove owned containers; preserve workspace/volumes
bin/cxz down .                             # remove project + sessions from use; retain source/volumes
bin/cxz project up .                       # recreate + resume; never replay old prompts
bin/cxz project recreate --yes .           # writable layer lost; editors disconnect
bin/cxz install --recreate         # replace manager, keep project processes/data
bin/cxz uninstall                  # remove manager only; projects/data remain
```

Inside an owned project, `cxz session attach`, `cxz session ls` and session controls are scoped to that
project. No manager Docker socket or credential directory is mounted there.
Global options precede commands: `cxz --state /private/client-state project up .`.
Use the same client state for all host commands. Default: `$XDG_STATE_HOME/cxz`
or `~/.local/state/cxz`; this stores an installation locator, not the named volumes.

The CLI uses `lesomnus/xli`: command flags must precede positional arguments.
Use `cxz session new --account work-codex .`, not `cxz session new . --account work-codex` (the old ordering is
now rejected). Each command has generated `--help`; help and completion need no
running manager. Enable zsh completion with `source <(bin/cxz completion zsh)`.
`project exec PROJECT -- COMMAND...` preserves everything after `--` as command arguments.
For local development, use `cxz manager serve --agent /path/to/claude` (default: `claude`).
Root-level `--agent`, the unused `--claude-config`, and the disabled project-wide
`login` command are removed; authenticate with `cxz account login ACCOUNT`.
See the [complete CLI argument audit](docs/cli.md).

### Remote Docker

Binds resolve on the engine. In this development environment install with
`--workspace-root /workspaces`; the known `/workspace` alias is translated to the
shared `/workspaces/...` path. Unknown nonshared paths fail closed. The installer
supports Unix Docker sockets and reachable non-TLS TCP engines. SSH/TLS endpoint
setup and cross-OS client builds are not implemented. Never expose Docker's
unauthenticated API to an untrusted network.

## TUI and scripts

`cxz purge` opens an irreversible-cleanup checklist for the installation selected
by `--state`. All categories start checked: containers, project data/login
volumes, manager database/account volume, tool volumes, networks and local state.
Use Space to toggle, arrows/Tab to navigate, PgUp/PgDn to inspect targets, then
the final **Confirm irreversible deletion** button. Esc cancels.
`cxz purge --dry-run` lists targets without modifying anything.

Unlike `uninstall`, purge can delete conversations and credentials permanently.
Back them up first. Run from the host/client; stop native cxz servers/agents first.
Owned Docker containers are removed before volumes; owner labels and the inventory
are rechecked. Partial failures retain the installation locator. If Docker data
is unchecked, keep local state too so its ownership identity remains available.
Workspace sources, personal Claude/Codex login directories, the CLI executable,
shared images/build caches, and unlabeled Docker resources are never removed.
Unknown files inside the state directory are reported and preserved; only known
cxz children are deleted, not the state directory itself. This does not clean
other cxz installations or revoke OAuth tokens at the provider.

Projects have a display `name` and a unique short `alias`. The default name is
the workspace directory name. Like cld, short names remain short, multi-word
names use initials (`my-web-app` → `mwa`), and long single words are truncated.
Collisions get a stable ID-derived suffix; existing aliases are not reassigned.
Explicit aliases follow payday's lowercase-letter/alphanumeric/hyphen grammar
and are case-normalized. A duplicate explicit alias is rejected.

```sh
cxz project add --name "My Web App" --alias web .  # register only; no container
cxz project up --no-attach web
cxz project set --name "Production Web" --alias prod web
cxz project exec prod -- pwd
cxz project logs prod
cxz session attach prod
cxz project down prod
# Name/alias can also be supplied during new/up/recreate:
cxz session new --name "My Web App" --alias web .
```

`cxz project ls` exposes both fields; the TUI shows alias and display name. Project
arguments accept an exact ID/path, then an alias, then an unambiguous display
name, in that order. Display names can contain spaces and need not be unique.
Shell completion offers handles for project/up/new/down/recreate/attach/exec/shell/login/logs.
Renaming does not change the project/session/container identity, and changing a
display name does not implicitly change its alias. Metadata lives in the payday
resource database and survives server/container restarts; back up state volumes.

| Key | Action |
|---|---|
| n / Ctrl+N (project) | Select Account and create a session; first project login runs if needed |
| a (project) | Open account view; n adds, l logs in, / searches, Esc returns |
| ↑/↓, Enter (project) | Select a session and open its view |
| s (project) | Stop selected session before starting another (one live session per project) |
| d / Delete, then y (project) | Stop and remove selected session; archived journal retained |
| Ctrl+Q (session) | Return to project view without stopping the agent |
| Tab / Shift+Tab (session) | Forward / reverse: pending approvals → input → bottom session selector |
| Enter / Backspace (approval focus) | Allow / deny selected request; Enter opens a dialog for questions |
| Ctrl+S | Send message (Ctrl+Enter also works with compatible terminal encoding) |
| Enter / Alt+Enter / Ctrl+J | Insert newline (multiline paste stays in the editor) |
| Ctrl+X (session) | Clear the current draft |
| F2 / F3 | Allow / deny pending approval |
| `/answer` | Open/reopen the selected pending question dialog |
| F4 | Interrupt active turn |
| Esc twice within 3 seconds | Confirm interruption of the active turn |
| Ctrl+R | Explicitly resume stopped/offline session |
| PageUp / PageDown / mouse wheel | Scroll; Ctrl+Home first line, Ctrl+End follow latest |
| `/approval` | Inspect the selected request's complete payload |
| `/permission full`, `/permission ask` | Auto/manual tool approval for the current attached session run |
| `/stop` | Terminate selected agent |
| `/restart` | Confirm/Cancel dialog to restart this session's agent; Tab/arrows select, Enter applies, Esc cancels |
| Ctrl+C | Detach; agent continues |

The dashboard groups workspace details and session cards; the conversation view
keeps tool activity above a growing, rounded message editor. Drafts are retained
per session while this TUI is open, including trips back to the project view.
Escape does not discard a conversation draft. The composer and its borders keep
the terminal's background (including the current input line). Use at least
40 × 14 cells. The composer spans the terminal width. User messages show local
timestamps instead of a YOU label; Claude/Codex speaker badges and pastel tool
colors distinguish output. State events never accumulate in the transcript. Only
`working` animates a braille spinner in the live conversation. `idle` is silent;
`waiting_input` uses the pending-request box; starting/stopping/stopped/interrupted/
failed conditions remain visible in notices. All state events remain in the journal.
Reply text is white; only speaker labels retain agent branding. User timestamps
are dimmed and the first-line `>` starts at the left edge. Completed-turn JSON is
replaced by a dim metrics footer after a blank line: duration, cost, then tokens.
Duration starts immediately after the two indicator cells and uses `00:00:00 ◷`
(zero units are darker). Cost is compact: `$0.1`; below $0.10 it uses cents (`¢1.2`).
Nonzero amounts below 0.05 cents show `¢<.1`; `/usage` retains four-decimal USD totals.
Token symbols follow the values: ↑ input / ↓ output / ↺ cache read / ⊕ cache write /
∑ total. Missing values are omitted; ≈◷ denotes measured request-to-completion time.
Codex metrics use per-turn usage, never cumulative thread totals.
Apart from the clock, metrics use five-cell slots (three numeric cells and two
unit cells) with one separating cell, e.g. `1.2k↑`. Numeric counts are right-aligned;
cost keeps its leading currency symbol. The first two columns
are reserved for indicators; other content is indented (except the composer).
Session information occupies a single
bottom line; persistent shortcut rows are hidden. Type `/help` to display local
shortcut help in the conversation without sending a prompt to the agent.
Enter inserts a newline; Ctrl+Enter sends on terminals emitting CSI-u or xterm
modified-Enter sequences. Ctrl+S is the portable send alternative (legacy
terminals cannot distinguish Ctrl+Enter from Enter). Paste never submits.
Typing `/` overlays fuzzy-matched command hints above the editor: arrows select,
Tab completes, Ctrl+Enter/Ctrl+S executes, Esc dismisses. A blank row separates
the overlay from history. It shows at most seven items, with a two-item scroll
margin on either side where available. `/context` invokes Claude's native report
or displays Codex's latest reported context footprint/window. `/compact` invokes
native Claude compaction or Codex `thread/compact/start`; it requires an idle
session and preserves the cxz journal. `/usage` reads the full session journal and reports
tokens, cost and elapsed time with per-metric coverage. Claude cumulative costs
are counted once per run, not repeatedly per turn. This is not account quota or billing.

The status bar starts with one reserved selection cell and a seven-cell session
alias, followed by agent/model, ◉ account and title (no state badges). The right
side shows provider-reported **remaining** account quota, eight-cell bars, window
labels and reset countdowns, with one cell of right padding. Missing quota shows
`quota waiting`, `unsupported`, `unavailable` or `error`; `/usage` explains the
reason. Old snapshots carry `~`,
and expired windows show `refresh` rather than assuming they reset to 100%.
Telemetry uses the already authenticated provider process: Codex rate-limit RPCs
and Claude's experimental `get_usage`/rate-limit events. It refreshes at startup,
after turns and every minute; unsupported versions degrade without failing turns.
Aliases are globally unique,
random 3–7-letter English words, stored in payday/SQLite and assigned to existing
sessions on reconciliation. In session selection mode (Tab), press `r` to edit;
Enter saves and returns to selection, Esc cancels. Custom aliases accept 3–7
lowercase letters. Session commands also accept aliases. Deleting a session
releases its alias while retaining the journal. The curated word pool is finite;
exhaustion is reported explicitly without falling back to numeric identifiers.
The composer starts with `>` on row 0 and dim single-digit line numbers afterward:
`1 … 9, 0, 1 …`. The two-cell gutter never grows.

Pending approvals have their own box above the composer. Tab follows physical
order (approvals → composer → bottom session bar); Shift+Tab reverses it. Arrows
select, Enter allows and Backspace denies. PgUp/PgDn, Ctrl+Up/Down, Ctrl+Home/End
and the mouse wheel scroll the focused request without truncating its content.
Provider-specific titles/commands/reasons appear before the full native payload.
`/approval` shows the full selected
payload in the conversation. Questions open a focused dialog automatically;
`/answer` (or Enter on a pending question) reopens it after dismissal. Arrows/Tab
move, Space/Enter selects an option, and Other accepts free text. Claude supports
multiple selections and option previews; Codex questions use its native question
IDs and Other policy. Next/Back navigates questions; Submit sends all answers
together (Ctrl+S is next/submit). Esc/Cancel closes without rejecting the request.
PgUp/PgDn or the wheel scroll long content. JSON is not required; the advanced
`/answer {"question text or id":"answer"}` form remains available.
The provider-neutral question model lives in `internal/agentview/questions.go`;
original provider payloads stay in the journal and remain accessible via `/approval`.
After a decision, focus returns to input to avoid approving the next request with
a repeated Enter. A blank row separates the conversation from the notice area.
Resolved approvals update the original checkbox row in place, with separate
allowed/denied/canceled colors. Tool results are a single-line summary;
`/details` shows the latest full result and `session events` retains all events.
Reply metrics begin with completion time (`MM-DD HH:MM / duration …`). The working
spinner includes elapsed time and an Esc interrupt hint; press Esc twice within
three seconds to confirm. F4 remains a direct interrupt shortcut.

Claude uses its default tool set (`--tools` is omitted). This does not enable
automatic approval. Configurable agent profiles are tracked in [TODO.md](TODO.md).

`/permission full` immediately allows existing and future **known tool, command,
file and permission requests** for the current run while viewing this session in
this TUI. It does not invent question answers or allow unknown protocol requests.
`/permission ask` restores manual approval; decisions already sent cannot be
retracted. Disconnect, run replacement, approval failure, returning to the project
or exiting the TUI disables this local mode. It is not persisted or a change to
the vendor's sandbox/permission configuration. Decisions still use normal Reply
RPCs with run/request identities; ambiguous failures are never automatically retried.

While scrolling, the notice row displays rendered line range/total and the source
event timestamp in local time. Line numbers start at the session's first loaded
event and change when the terminal width changes; a multiline event shares its
timestamp. The TUI retains all streamed events instead of dropping the oldest
2,000-event overflow, so large sessions consume more client memory. New output
does not pull you away from history. If your latest prompt is above the viewport,
up to two prompt lines are pinned over its top edge with a full-width dark brand
background. Ctrl+End follows new output. The TUI fills the terminal and follows
terminal size changes.

Assistant text is parsed as CommonMark/GFM when structural Markdown syntax is
found; plain text remains plain. Headings, lists, emphasis, tables and code are
rendered locally; inline/fenced code uses a black background. HTML is not rendered,
images are not fetched and escape sequences are stripped. The current
[Claude text block](https://github.com/anthropics/claude-agent-sdk-python/blob/main/src/claude_agent_sdk/types.py)
and [Codex agentMessage schema](https://github.com/openai/codex/blob/main/codex-rs/app-server-protocol/schema/typescript/v2/ThreadItem.ts)
do not provide a Markdown MIME discriminator, so detection is heuristic.

The alternate-screen output adapter repositions the physical terminal cursor at
the input widget's Unicode-aware cursor cell, including wrapped/scrolled drafts.
This fixes the bottom-row cursor anchor used by IME preedit/candidate windows;
actual composition behavior still depends on the terminal and OS IME. OAuth's
temporary terminal ownership is left untouched.

Assistant headers start with `• CLAUDE` / `• CODEX` in the indicator gutter;
answer text stays indented by two cells. On Unix, cxz requests Kitty keyboard
disambiguation and queries the enabled flags while the TUI owns the alternate
screen. Ctrl+Enter sends alongside Ctrl+S; Enter remains a newline. Keyboard
mode is restored before exit or an external login and requested again on return.
Terminals/multiplexers that do not deliver extended keys may still send the same
CR for Enter and Ctrl+Enter; use Ctrl+S there. The actual terminal/IME still needs
local verification; automated tests exercise the protocol bytes through a PTY.

Only quota bars change color as remaining quota drops: at or below 50% pastel
peach, 30% coral, and 15% pink-red. Above 50% the existing muted color remains;
percentages, reset times and labels keep their existing styling.

`cxz up` prints elapsed time and observed server provisioning checkpoints to stderr:
configuration, resources, devcontainer image/build/hooks, selected agent installation,
runtime boot/readiness. Fast steps may pass between polls; long steps repeat every
ten seconds. This is stage reporting, not a percentage or raw Docker build log.
`--no-attach --format json` keeps stdout clean.

Project `a` opens accounts without leaving the app. New-session `n` opens the same
view in selection mode; an empty list offers account creation. Add a provider
(Claude/Codex), alias and optional display name, then confirm Create account.
Provider names use their brand colors in a six-cell slot, keeping selector arrows fixed.
`/` focuses search by alias/name/provider/number; Enter selects in new-session mode.
`l` runs the existing login workflow and returns to accounts (Claude: current
project; Codex default: central). Registration alone does not authenticate.

Data commands (table by default; use `--format json` for scripts): `session ls`, `project ls`, `session get ID`, `session send ID TEXT`,
`session reply ID REQUEST_ID allow|deny [ANSWERS_JSON]`, `session interrupt ID`, `session resume ID`,
`session stop ID`, `session events ID [AFTER_SEQ]`. TUI event connections retry by cursor;
mutations are never blindly retried. Session APIs accept idempotency keys.

## Resource API

The server uses payday's generated resource framework, not only its config
packages. Definitions are in `proto/cxz`; lifecycle extensions are in
`proto/ext/cxz`. `go tool pd gen .` generates `resource/`, `internal/ent/`,
`server/bare/` and `server/pd/`. The handwritten application layer is
`server/lifecycle/` (generated Sink → publish interceptor → lifecycle → audit/gate).

- `ProjectService.Add/Get/List/Watch` registers and reads workspace resources;
  `Up/Down/Recreate` controls their owned containers. Up does not create a session.
- `SessionService.Add/Get/List/Watch` manages conversation resources;
  `Resume/Send/Reply/Interrupt/Stop` controls their runs.
- Resource `Watch` subscribes to explicit resource refs. `Events/History` is the
  separate, durable conversation journal with sequence cursors.

Both resources are payday `global` entities: no fabricated tenant or user.
Project Patch permits only name/alias/description with optimistic version checks.
Other general Patch/Apply/Erase is closed; runtime status is not caller-writable.
CLI/TUI and manager-to-project traffic use `cxz.ProjectService` and
`cxz.SessionService`. The API has no version suffix or compatibility aliases.
`api/` and `internal/runtimeproto/` define internal runtime view models in the
separate `cxz.runtime` namespace; that service is not registered publicly.
Runtime session/vendor IDs and journals are separate from payday resource IDs.

`resources.db` stores ent resources and payday audit rows; `cxz.db` retains the
runtime registry/event cache during this migration. Resource state is imported
from existing manifests/journals. Custom resource metadata and audit history cannot be rebuilt from them:
back up the full state volumes, not just transcripts.

## Persistence and boundaries

- One active agent per canonical workspace. Project supervisors are independent
  of the manager and UI. Manager replacement reconnects project networks.
- Host client → manager's private socket through Docker exec stdio → project
  network + capability → scoped runtime → supervisor. No host port is published.
  Project TCP is authenticated, not encrypted; the Docker operator is trusted.
- Project state/transcripts use named volumes; checksum-verified tools are shared
  read-only. Back up these volumes **and workspace contents**. SQLite alone is not
  a backup. The fsynced journal/manifests are authoritative for runtime recovery;
  SQLite resource/audit history should also be backed up.
- Container loss ends its processes. `project up`/`session resume` creates a new run and resumes
  the vendor conversation. Stale approvals fail and old prompts are not replayed.
  Codex may give a previously empty thread a new vendor ID: no transcript exists
  until its first turn. This exception requires no recorded send intent.
- A crash may leave `delivery_unknown`; there is no exactly-once side-effect
  guarantee. Inspect history before repeating a task. `project down` collects final
  history; abrupt loss can leave manager history incomplete until `project up` recovers.
- Foreign `project recreate` requires confirmation: writable layer lost, editors detached,
  workspace/named volumes retained. Host initialization/elevated settings require
  `--trust-config`. That flag trusts the configuration; it is not a hostile-code sandbox.
- Codex uses `untrusted` approval policy without a nested sandbox inside its owned
  devcontainer. Vendor permission requests are manually surfaced in the TUI.
  Raw journals may contain private source and secrets. Do not publish them.

## Verification and scope

### Docker Bake and edge images

CI follows cld's two-stage Bake flow: `build` exports static amd64/arm64 binaries,
then `app` packages them with `internal/installer/image.Dockerfile`. The same
Dockerfile is embedded in the standalone install command; releases use Bake too.

After tests pass, main pushes publish `ghcr.io/lesomnus/cxz:edge` and
`ghcr.io/lesomnus/cxz:sha-COMMIT_SHA` for `linux/amd64` and `linux/arm64`.
PRs build without registry login or publication. `edge` is a moving development
tag; use a recorded image digest when you need an exact build.

```sh
TAG=edge BUILD_HASH=$(git rev-parse HEAD) docker buildx bake build
# Local native smoke test (the example assumes an amd64 engine):
TAG=local docker buildx bake app --set app.platform=linux/amd64 --load
docker run --rm ghcr.io/lesomnus/cxz:local version
# Explicit publication, with registry credentials and both binaries prepared:
TAG=edge BUILD_HASH=$(git rev-parse HEAD) docker buildx bake app --push
```

The build context excludes workspace state and credentials. Tests run in CI on
the runner, not inside the Docker build. Root `Dockerfile` compiles source;
`docker-bake.hcl` defines output, platforms, tags and image metadata.

### Tests

```sh
go test ./...
CXZ_TEST_RACE=1 go test -race ./internal/... -count=1
go vet ./...
go run ./tools/genproto
go tool pd gen --check .
# Uses an explicitly selected disposable, authenticated owned project and live usage:
node scripts/probes/owned-session-live.mjs CLIENT_STATE PROJECT codex ACCOUNT
```

Pinned: Claude 2.1.267, Codex 0.154.0, devcontainer CLI 0.89.0. Codex integration
follows its generated schema and official [app-server protocol](https://learn.chatgpt.com/docs/app-server).
See [progress](docs/progress.md) and [cld-parity acceptance](docs/plans/cld-parity.md)
for results. Web/IDE UI, roster authentication, central login brokering, automatic
release updates, dotfile/SSH forwarding and encrypted off-host backup remain
separate work. This is not every cld convenience feature or the full web MVP.
Large journals need future segmentation/indexing and retention policy.
