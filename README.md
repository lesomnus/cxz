# cxz

Local Claude sessions with durable history and a TUI. Built with Go, payday's
gRPC runtime and database configuration, SQLite, and Bubble Tea. Linux first.

```sh
go build -o bin/cxz ./cmd/cxz
# Terminal 1: use your existing, already authenticated Claude Code installation.
bin/cxz serve
# Terminal 2:
bin/cxz tui
```

Global options precede the command: `cxz --state /persistent/cxz serve`.
Use the same `--state` for every client. `--agent /path/to/claude` and
`--claude-config /path/to/existing/config` are server options. The inherited
`CLAUDE_CONFIG_DIR` is used by default. Never copy a live refresh-token store
to run it concurrently elsewhere. Tested with Claude Code **2.1.267**.

## TUI

| Key | Action |
|---|---|
| Ctrl+N, path, Enter | Create a session in an existing workspace directory |
| Tab, ↑/↓, Enter | Select a session / return to message entry |
| Enter | Send a message |
| F2 / F3 | Allow / deny the first pending tool approval |
| `/answer {"question text":"answer"}` | Answer an AskUserQuestion request |
| F4 | Interrupt the active turn |
| Ctrl+R | Explicitly resume an offline/stopped session |
| PageUp / PageDown | Scroll conversation |
| `/stop` | Terminate the selected session's agent |
| Ctrl+C | Exit TUI; **agent continues running** |

The TUI reconnects automatically to the daemon and replays from its last event
cursor. Commands are not automatically retried on connection failure. Inspect
history first if delivery is uncertain. The API accepts stable `client_id`s for
safe retries; the same ID with different arguments is rejected.

## Scriptable commands

```sh
bin/cxz new /absolute/workspace 'My session'
bin/cxz ls
bin/cxz get SESSION_ID
bin/cxz send SESSION_ID 'Explain this repository'
bin/cxz events SESSION_ID           # JSONL stream; optional last-seen sequence
bin/cxz reply SESSION_ID REQUEST_ID allow
bin/cxz reply SESSION_ID REQUEST_ID deny
bin/cxz interrupt SESSION_ID
bin/cxz resume SESSION_ID
bin/cxz stop SESSION_ID
```

## Persistence and safety

Default state: `$XDG_STATE_HOME/cxz`, or `~/.local/state/cxz`.

```text
TUI / CLI → private Unix gRPC socket → payday daemon → SQLite projection
                                           ↓ private Unix HTTP socket
                                    detached supervisor → Claude stdio
                                           ↓
                                 fsynced events.jsonl + session.json
```

- One live session per canonical workspace within a state directory. Existing
  workspaces are used as-is; cxz does not provision a devcontainer yet.
- Daemon/TUI termination does not kill the supervisor. Supervisor or container
  loss requires explicit `resume`; this starts a new run with the same Claude
  session ID. Old approvals are invalid; old prompts are never auto-replayed by
  cxz. A resumed model can still propose retrying a previous task: cancel it
  explicitly in the next prompt if desired.
- Persist the **cxz state, workspace at the same path, and original Claude state**
  across container recreation. SQLite alone is not a backup. Reauthentication
  may be necessary; restoring credentials is not implemented.
- SQLite is rebuilt from session manifests and committed journal events when
  missing. A torn final journal record is preserved in a `.partial-*` diagnostic
  file before repair. Corrupt committed records fail closed.
- Requests include a run ID; approvals additionally include their request ID.
  Durable intent precedes delivery. After a crash, a receipt can remain
  `delivery_unknown`: **this is not an exactly-once side-effect guarantee**.
- Directories are `0700`, sockets/files private. There is no TCP listener, web
  UI, tenant, user authentication, or roster integration yet. Only the same OS
  user is trusted. Do not expose these sockets through an unauthenticated proxy.
- Tool approval is manual; shell commands and file edits require approval.
  The agent still runs with the current OS user's permissions: this is not a
  sandbox. Raw logs may contain source code, secrets and personal information.
  They are intentionally not uploaded or committed.

## Verification

```sh
go test ./...
go test -race ./internal/...
go vet ./...
# Opt-in, consumes Claude usage using your existing login:
CXZ_LIVE_TEST=1 go test ./internal/integration -run TestLiveClaude -v -count=1
```

The deterministic integration test builds real daemon/supervisor processes,
exercises approvals/questions/interrupt, kills and restarts processes, verifies
cursor replay and SQLite reconstruction. Live tests retain private evidence in
a printed `/tmp/cxz-live-*` directory. They never copy credentials.

Protobuf generation (no system protoc required): `go run ./tools/genproto`.
The generated API is tracked. See [progress](docs/progress.md) and
[implementation plan](docs/plans/implementation-plan.md) for scope and results.
