# CLI reference

`cxz` with no subcommand opens [the TUI](tui.md). Every subcommand prints its result
and exits — nothing attaches.

`--help` is generated for every command and works without a running manager, as
does completion:

```sh
source <(cxz completion zsh)
```

## Two rules that catch people

**Flags come before positional arguments.**

```sh
cxz session new --account personal .      # correct
cxz session new . --account personal      # rejected
```

**Global options come before the subcommand.**

```sh
cxz --state /private/state project up .
```

`project exec` is the exception in the other direction: everything after `--` is
passed through verbatim.

## Global options

| | |
|---|---|
| `--endpoint NAME_OR_URL` | A connection name, or `ssh://`, `tcp://`, `local://`, `unix://`. Or `CXZ_ENDPOINT`. |
| `--token-file FILE` | Bearer token for a `tcp://` endpoint. Or `CXZ_TOKEN_FILE`. |
| `--session ID_OR_ALIAS` | Which session the TUI opens on. |
| `--state DIR` | Client and runtime state directory. Use the same one consistently. |
| `--format table\|json` | JSON preserves every field; table is for reading. |
| `-x`, `--exit-on-error` | Return errors instead of offering interactive recovery. |

## Installation

```sh
cxz install --workspace-root DIR    # start the background manager
cxz install --recreate              # replace it, keeping project processes and data
cxz uninstall                       # remove the manager only
cxz version                         # client build and pinned agent versions
```

## Projects

```sh
cxz up WORKSPACE                    # prepare and print project data; no session
cxz down WORKSPACE                  # remove containers; keep everything else
cxz project add --name NAME --alias ALIAS WORKSPACE
cxz project set --name NAME --alias ALIAS PROJECT
cxz project up PROJECT              # recreate and resume
cxz project recreate --yes PROJECT  # rebuild; writable layer lost
cxz project down PROJECT
cxz project purge PROJECT              # preview permanent removal
cxz project purge --yes PROJECT        # delete this project and its owned data
cxz project ls                      # owned and foreign projects
cxz project logs PROJECT            # provisioning output
cxz project shell PROJECT
cxz project exec PROJECT -- COMMAND...
```

`up` prepares and returns without creating a session or starting an agent.
`recreate` requires `--yes` because the writable layer is lost and attached editors
disconnect.

A `PROJECT` argument resolves as exact ID or path, then alias, then an unambiguous
display name.

## Sessions

```sh
cxz session new --account ACCOUNT [--model ID] [--name NAME] [--alias ALIAS] WORKSPACE
cxz session ls
cxz session get ID
cxz session send ID "text"
cxz session reply ID REQUEST_ID allow|deny [ANSWERS_JSON]
cxz session interrupt ID
cxz session resume ID
cxz session stop ID
cxz session events ID [AFTER_SEQ]
```

`--account` is required from a script; interactive runs offer a picker. Sessions
accept their alias anywhere an ID is taken.

Event streams reconnect by cursor. Mutations are never retried blindly; these APIs
accept an idempotency key so retrying is your decision.

## Accounts

```sh
cxz account add [--name "Display Name"] AGENT ALIAS
cxz account login ALIAS
cxz account login --session SESSION ALIAS     # Claude, per session
cxz account ls
cxz account get ALIAS
cxz account status ALIAS
cxz backend ls                                 # available auth strategies
cxz binding ls ALIAS                           # project bindings; metadata only
cxz github sync                                # host gh credentials into projects
cxz gitconfig sync                             # refresh host Git settings in all running projects
```

Registering and logging in are separate. See [accounts](accounts.md).

## Settings

```sh
cxz edit                            # settings.jsonc
cxz edit docker-compose             # the shared Compose override
cxz edit share PATH                 # a file in the shared source directory
cxz devcontainer render [PROJECT]   # write out the devcontainer in effect
cxz config show
cxz config set claude-model ID
cxz config unset codex-model
cxz config files                    # host file mappings
```

`devcontainer render` answers "what is actually applied". It writes the
configuration cxz handed the devcontainer CLI, every Compose file in merge order
and their merged result into a directory, then prints its path. Your
`.devcontainer` is read, never written; see
[the shared Compose override](docker.md#the-shared-compose-override).

## Skills and MCP

```sh
cxz skill list [--project PROJECT]
cxz skill add NAME                          # register from ${CXZ_SHARE_DIR}/skills
cxz skill remove NAME
cxz skill enable NAME [--project PROJECT]
cxz skill disable NAME [--project PROJECT]
cxz skill inherit NAME --project PROJECT

cxz mcp list [--project PROJECT]
cxz mcp add ID FILE
cxz mcp remove ID
cxz mcp enable ID [--project PROJECT]
cxz mcp disable ID [--project PROJECT]
cxz mcp inherit ID --project PROJECT
cxz mcp logs ID --project PROJECT --session SESSION
cxz mcp restart ID --project PROJECT --session SESSION
```

Both follow the same shape: a global default, with an optional per-project override,
and `inherit` to drop the override. See [skills](skills.md) and [MCP](mcp.md).

## Auxiliary AI

```sh
cxz ai list
cxz ai models ACCOUNT
cxz ai login ACCOUNT                 # dedicated login, on the manager host
cxz ai set summary    --account ACCOUNT --model MODEL [--effort EFFORT]
cxz ai set suggestion --account ACCOUNT --model MODEL [--effort EFFORT]
cxz ai disable summary|suggestion
cxz ai status SESSION
cxz ai cancel SESSION
```

Background summaries and next-message suggestions, on their own account. See
[auxiliary AI](auxiliary-ai.md).

## The shared Docker engine

```sh
cxz docker status
cxz docker up                       # apply configuration; restarts Docker workloads
cxz docker down                     # remove the engine, keep its images and volumes
cxz docker sync                     # save configuration without restarting
```

## Updates

```sh
cxz use                             # running version, channel, pin state (JSON)
cxz use @edge | @stable | vX.Y.Z
cxz use --unpin
cxz self-update [--ref REF] [--client-only]
cxz self-update status|check|enable|disable [--server]
cxz manager update IMAGE            # explicit tag or digest, never latest
cxz manager rollback                # the recorded previous image
```

`--server` manages the manager's policy and only works on the Linux host. See
[updates](updates.md).

## Diagnostics

```sh
cxz manager doctor                  # read-only health checks
cxz manager logs
cxz project logs PROJECT
cxz terminal-info                   # terminal environment and palette; no manager needed
```

## Remote access

```sh
cxz --endpoint ssh://user@host
cxz --endpoint tcp://host:7349 --token-file FILE
cxz expose --token-file FILE --listen tcp://127.0.0.1:7349
```

See [remote access](remote.md).

## Destructive

```sh
cxz session purge --dry-run SESSION
cxz session purge --yes SESSION
cxz purge --dry-run
cxz purge --yes [--groups containers,projects,state,tools,networks,local]
```

The only two commands that delete conversations. `session purge` destroys one —
journal, agent profile, uploads, memories and the record itself; deleting a session
keeps all of that. `purge` does the whole installation, credentials included. Both
refuse to run without `--yes`, and `--dry-run` reports the exact targets. See
[projects and sessions](projects.md#deleting-a-session).

## Development

```sh
cxz manager serve [--agent /path/to/claude]
```

A foreground server, for developing cxz itself. Not how you install it.

## Internal entry points

Commands beginning with `_`, plus `wisp`, are internal: cxz invokes them inside
containers. They are not a supported interface and their arguments change without
notice.
