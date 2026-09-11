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
bin/cxz new --agent codex .       # or claude; prepares the project and opens TUI
```

`install` builds and starts a background Docker manager, waits for its API, and
returns. The host needs Docker and cxz; Node, devcontainer CLI and agent binaries
are managed in containers. `serve` is a **foreground, blocking development
server**, not the normal installation command.

The directory must exist under the installed root. `new .` uses its
`devcontainer.json`, not an existing VS Code container. Missing configuration gets
a base Debian devcontainer with non-root `vscode` user. Multiple configurations
prompt in a terminal; scripts pass `--config`. Image, Dockerfile, Compose,
features and hooks delegate to the official devcontainer CLI.

First login is a user action, in another terminal:

```sh
bin/cxz projects
bin/cxz login --agent codex PROJECT    # device login
bin/cxz login --agent claude PROJECT
```

Credentials/transcripts stay on the project's state volume. Host refresh-token
stores are never copied. Only the selected vendor is downloaded. Stop the active
session before `new --agent OTHER_VENDOR`. Interactive `new` offers agent choice;
scripts default to Claude. Agent selection is saved per session.

Client preferences can change that default: `cxz config set agent codex`.
Use `cxz config set codex-model MODEL_ID` or `new --model MODEL_ID .` for a new
session's model; reconnect/resume preserves it. `cxz doctor`, `cxz logs PROJECT`
and `cxz version` provide diagnostics. See [operations and releases](docs/operations.md)
for retry behavior, model settings, versioned installation, updates and rollback.

The remote user must have write permission on the host workspace. Explicit
`remoteUser` settings are respected; cxz does not silently change repository file
ownership or remap an existing user's UID. Adjust the devcontainer for a host UID
other than the default image's 1000 when necessary.

```sh
bin/cxz up .                       # attach existing session; recover if necessary
bin/cxz up --no-attach .            # prepare, return JSON
bin/cxz new --agent claude .        # new conversation; stop an active one first
bin/cxz attach PROJECT             # also: it SESSION_ID
bin/cxz tui                        # also: watch
bin/cxz exec PROJECT -- go test ./...
bin/cxz shell PROJECT
bin/cxz down .                     # remove owned containers; preserve workspace/volumes
bin/cxz up .                       # recreate + resume; never replay old prompts
bin/cxz recreate --yes .           # writable layer lost; editors disconnect
bin/cxz install --recreate         # replace manager, keep project processes/data
bin/cxz uninstall                  # remove manager only; projects/data remain
```

Inside an owned project, `cxz it`, `cxz ls` and session controls are scoped to that
project. No manager Docker socket or credential directory is mounted there.
Global options precede commands: `cxz --state /private/client-state up .`.
Use the same client state for all host commands. Default: `$XDG_STATE_HOME/cxz`
or `~/.local/state/cxz`; this stores an installation locator, not the named volumes.

The CLI uses `lesomnus/xli`: command flags must precede positional arguments.
Use `cxz new --agent codex .`, not `cxz new . --agent codex` (the old ordering is
now rejected). Each command has generated `--help`; help and completion need no
running manager. Enable zsh completion with `source <(bin/cxz completion zsh)`.
`exec PROJECT -- COMMAND...` preserves everything after `--` as command arguments.
For local development, prefer `cxz serve --agent /path/to/claude`; the old
root-level `--agent` / `--claude-config` flags are retained for serve compatibility.

### Remote Docker

Binds resolve on the engine. In this development environment install with
`--workspace-root /workspaces`; the known `/workspace` alias is translated to the
shared `/workspaces/...` path. Unknown nonshared paths fail closed. The installer
supports Unix Docker sockets and reachable non-TLS TCP engines. SSH/TLS endpoint
setup and cross-OS client builds are not implemented. Never expose Docker's
unauthenticated API to an untrusted network.

## TUI and scripts

| Key | Action |
|---|---|
| Ctrl+N, path, Enter | New session; Tab switches Claude/Codex while entering path |
| Tab, ↑/↓, Enter | Select session / return to message entry |
| Enter | Send message |
| F2 / F3 | Allow / deny pending approval |
| `/answer {"question text or id":"answer"}` | Answer question |
| F4 | Interrupt active turn |
| Ctrl+R | Explicitly resume stopped/offline session |
| PageUp / PageDown | Scroll |
| `/stop` | Terminate selected agent |
| Ctrl+C | Detach; agent continues |

JSON commands: `ls`, `projects`, `get ID`, `send ID TEXT`,
`reply ID REQUEST_ID allow|deny [ANSWERS_JSON]`, `interrupt ID`, `resume ID`,
`stop ID`, `events ID [AFTER_SEQ]`. TUI event connections retry by cursor;
mutations are never blindly retried. Session APIs accept idempotency keys.

## Resource API

The server uses payday's generated resource framework, not only its config
packages. Definitions are in `proto/cxz/v2`; lifecycle extensions are in
`proto/ext/cxz/v2`. `go tool pd gen .` generates `resource/`, `internal/ent/`,
`server/bare/` and `server/pd/`. The handwritten application layer is
`server/lifecycle/` (generated Sink → publish interceptor → lifecycle → audit/gate).

- `ProjectService.Add/Get/List/Watch` registers and reads workspace resources;
  `Up/Down/Recreate` controls their owned containers. Up does not create a session.
- `SessionService.Add/Get/List/Watch` manages conversation resources;
  `Resume/Send/Reply/Interrupt/Stop` controls their runs.
- Resource `Watch` subscribes to explicit resource refs. `Events/History` is the
  separate, durable conversation journal with sequence cursors.

Both resources are payday `global` entities: no fabricated tenant or user.
General Patch/Apply/Erase is closed; runtime status is not caller-writable.
CLI/TUI and manager-to-project traffic use these generated services. The old
`cxz.v1.Sessions` service is no longer registered; `api/` and
`internal/legacyproto/` remain internal compatibility view models for the runtime.
Upgrade the manager and explicitly recreate old project runtimes together: old
rc.1 wire clients/servers are not compatible with this API. Existing session,
vendor IDs and journals are retained, mapped to payday domain UUID resource IDs.

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
- Container loss ends its processes. `up`/`resume` creates a new run and resumes
  the vendor conversation. Stale approvals fail and old prompts are not replayed.
  Codex may give a previously empty thread a new vendor ID: no transcript exists
  until its first turn. This exception requires no recorded send intent.
- A crash may leave `delivery_unknown`; there is no exactly-once side-effect
  guarantee. Inspect history before repeating a task. `down` collects final
  history; abrupt loss can leave manager history incomplete until `up` recovers.
- Foreign `recreate` requires confirmation: writable layer lost, editors detached,
  workspace/named volumes retained. Host initialization/elevated settings require
  `--trust-config`. That flag trusts the configuration; it is not a hostile-code sandbox.
- Codex uses `untrusted` approval policy without a nested sandbox inside its owned
  devcontainer. Vendor permission requests are manually surfaced in the TUI.
  Raw journals may contain private source and secrets. Do not publish them.

## Verification and scope

```sh
go test ./...
CXZ_TEST_RACE=1 go test -race ./internal/... -count=1
go vet ./...
go run ./tools/genproto
go tool pd gen --check .
# Uses an explicitly selected disposable, authenticated owned project and live usage:
node scripts/probes/owned-session-live.mjs CLIENT_STATE PROJECT codex
```

Pinned: Claude 2.1.267, Codex 0.154.0, devcontainer CLI 0.89.0. Codex integration
follows its generated schema and official [app-server protocol](https://learn.chatgpt.com/docs/app-server).
See [progress](docs/progress.md) and [cld-parity acceptance](docs/plans/cld-parity.md)
for results. Web/IDE UI, roster authentication, central login brokering, automatic
release updates, dotfile/SSH forwarding and encrypted off-host backup remain
separate work. This is not every cld convenience feature or the full web MVP.
Large journals need future segmentation/indexing and retention policy.
