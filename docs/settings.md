# Settings and shared sources

Settings belong to an **installation**, not to the machine you happen to be typing
on. Connecting a remote frontend does not change the server's settings.

```sh
cxz edit                    # settings.jsonc in $VISUAL / $EDITOR / Notepad
cxz edit docker-compose     # the project Compose override
cxz edit share CLAUDE.md    # a file in the shared source directory
```

`settings.jsonc` accepts line and block comments and trailing commas. CLI
preference updates preserve your comments. `cxz edit` writes the schema beside the
file and links it with `$schema`, so editors complete and validate as you type.

A new settings file arrives full of commented-out examples.

Legacy `settings.jsonm` and `settings.json` are still read; the next save migrates
them.

## What lives there

- **Model defaults** per agent, also settable with `cxz config set codex-model ID`
  or per session with `session new --model ID`.
- **Connections** — named remote installations, and which is `default`. See
  [remote access](remote.md).
- **History limits** — the client scroll window. See
  [history and retention](history.md).
- **Docker** — the shared engine's mode. See [containers and Docker](docker.md).
- **File mappings** — below.

Two directories sit beside `settings.jsonc` rather than inside it: `share/`
(below) and `devcontainer/`, your
[default devcontainer template](docker.md#your-own-default-template).

## The shared source directory

`share/`, beside `settings.jsonc`, is where you keep files you want delivered into
sessions. `cxz edit share PATH` opens one, creating parent directories as needed.
Reference it as `${CXZ_SHARE_DIR}` in a mapping source.

This is also where the [skills](skills.md) library lives, under
`${CXZ_SHARE_DIR}/skills/`.

## File mappings

A mapping copies a host file or directory into each session:

| Destination variable | |
|---|---|
| `${AGENT_CONFIG_DIR}` | The session's agent configuration directory |
| `${SESSION_HOME}` | The session's HOME |
| `${WORKSPACE}` | The workspace inside the container |

```jsonc
// in settings.jsonc
"files": [
  { "src": "${CXZ_SHARE_DIR}/CLAUDE.md", "dst": "${AGENT_CONFIG_DIR}/CLAUDE.md" },
]
```

A directory source is copied recursively, preserving the executable bit. A mapping
may be limited to one agent.

Mappings are pushed to projects when a session is resumed or created, and written
at the **next agent start** — saving a mapping never restarts a running agent.
Files the mappings do not name are left alone rather than deleted.

Budget: 512 files and 2 MiB per project, counted across all mappings.

## Preferences the manager pushes

Three things are resolved by the manager and delivered to projects together: file
mappings, the [history policy](history.md), and [MCP](mcp.md) and
[skill](skills.md) activation. All of them apply at the next agent start.

A project runtime older than the manager will not understand a newer push. It is
skipped rather than failing the operation, and the value arrives when that runtime
updates — otherwise a version gap would make the project unusable.
