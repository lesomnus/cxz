# cxz

Run **Claude Code and Codex in devcontainers cxz owns**, with durable history and
a TUI you can walk away from.

The agent runs on the server, not in your terminal. Close the terminal, lose the
network, replace the manager — the conversation continues and is waiting where you
left it. Every session gets its own agent configuration, HOME, cache and login, so
a personal and a company subscription can work in the same repository without
seeing each other.

Go + [xli](https://github.com/lesomnus/xli) CLI + payday gRPC + SQLite + Bubble
Tea. Linux hosts the containers; a Windows or Linux client can drive it remotely.

Successor to [cld](https://github.com/lesomnus/cld).

## Requirements

- **Host:** Linux with Docker. Nothing else — Node, the devcontainer CLI and the
  agent binaries are managed inside containers.
- **Client (optional):** Windows or Linux, connecting over SSH or authenticated
  TCP. Needs neither Docker nor Go.

## Install

```sh
CGO_ENABLED=0 go build -o cxz ./cmd/cxz
./cxz install --workspace-root /absolute/path/holding/your/projects
```

Prebuilt binaries and checksums are on the
[releases page](https://github.com/lesomnus/cxz/releases).

`install` builds a manager image, starts it in the background, waits for its API
and returns. Projects must live under the workspace root you give it.

## Quick start

```sh
cxz account add codex personal    # register an authentication profile
cxz account login personal        # Codex logs in once, centrally
cxz up .                          # prepare this project's container
cxz                               # open the TUI
```

Then, in the TUI: `n` creates a session, `Enter` opens it, type, `Ctrl+S` sends.

`Ctrl+D` detaches and the agent keeps working. Run `cxz` again whenever.

What each step does:

1. **An account is an authentication profile**, not a cxz user. Codex logs in once
   and its token is supplied to projects; Claude logs in per session.
2. **`cxz up` prepares, it does not start an agent.** It uses the directory's
   `devcontainer.json`, or a plain Debian devcontainer if there is none.
3. **`cxz` with no subcommand is the TUI**, and the only way in. Subcommands print
   and exit.

## Concepts

| | |
|---|---|
| **Installation** | One manager, its containers, volumes and ownership identity. |
| **Project** | A registered workspace directory plus the container cxz owns for it. |
| **Session** | One conversation: its own agent, account, configuration and history. |
| **Run** | One agent process serving a session. Resuming starts a new run; the conversation carries on. |
| **Account** | An agent authentication profile. Fixed for a session once created. |

Full list in the [glossary](docs/glossary.md).

## Commands you will actually use

```sh
cxz                                   # the TUI
cxz up .                              # prepare a project
cxz down .                            # remove its containers, keep everything else
cxz project exec web -- go test ./...  # run something in the container
cxz project shell web
cxz session new --account personal .   # start a conversation from a script
cxz project ls                        # add --format json for scripts
cxz --endpoint ssh://user@host        # the same TUI, someone else's installation
```

Flags come before positional arguments. Every command has `--help`, and help works
without a running manager.

## Keys worth knowing

| | |
|---|---|
| `n` / `a` | New session / accounts (project list) |
| `Ctrl+Q` | Back to the project list; the agent keeps running |
| `Ctrl+S` | Send. `Enter` inserts a newline |
| `Tab` | Between pending approvals and the composer |
| `F2` / `F3` | Allow / deny an approval |
| `F4` or `Esc` `Esc` | Interrupt the turn |
| `Ctrl+End` | Jump to the latest output |
| `Ctrl+D` | Detach |
| `/help` | Shortcut help, without prompting the agent |

Everything else: [the TUI](docs/tui.md).

## Documentation

**Setting up** — [installation](docs/installation.md) ·
[accounts](docs/accounts.md) · [settings](docs/settings.md) ·
[containers and Docker](docs/docker.md) · [remote access](docs/remote.md)

**Using it** — [projects and sessions](docs/projects.md) ·
[the TUI](docs/tui.md) · [approvals and questions](docs/approvals.md) ·
[skills](docs/skills.md) · [MCP](docs/mcp.md) · [memory](docs/memory.md) ·
[auxiliary AI](docs/auxiliary-ai.md) · [CLI reference](docs/cli.md)

**Running it** — [updates and versions](docs/updates.md) ·
[history and retention](docs/history.md) · [security](docs/security.md)

**Internals** — [architecture](docs/architecture.md) · [glossary](docs/glossary.md)
· [development](docs/development.md)

Index: [docs/README.md](docs/README.md).

## Limits worth knowing before you rely on it

- **One live session per project at a time.** Stop one to start another.
- **Journals hold your source and may hold secrets.** Do not publish them.
- **Back up the state volumes and your workspace.** SQLite alone is not a backup.
- **Crashes can leave a tool call in an unknown state.** There is no exactly-once
  guarantee; check the history before repeating a task.
- Claude runs with its default tool set. cxz does not sandbox the agent beyond the
  devcontainer it owns.
