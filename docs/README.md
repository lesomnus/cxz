# cxz documentation

Start at the [README](../README.md) for what cxz is, how to install it and a quick
start. This is the map of everything else.

## Setting up

| | |
|---|---|
| [installation.md](installation.md) | What gets installed, the workspace root, client state, devcontainer configuration, trust |
| [accounts.md](accounts.md) | Authentication profiles, the two login shapes, session isolation |
| [settings.md](settings.md) | `cxz edit`, `settings.jsonc`, shared sources, file mappings |
| [docker.md](docker.md) | What cxz creates, the shared engine, Compose overrides, remote engines |
| [remote.md](remote.md) | SSH and TCP frontends, named connections, Windows |

## Using it

| | |
|---|---|
| [projects.md](projects.md) | Lifecycle, what survives what, names and aliases, purge |
| [tui.md](tui.md) | Keys, slash commands, reading a conversation, scrolling, attachments |
| [approvals.md](approvals.md) | The approval box, question dialogs, permission modes |
| [skills.md](skills.md) | The Agent Skills library and per-project scope |
| [mcp.md](mcp.md) | MCP registrations, global defaults, project activation |
| [memory.md](memory.md) | Retained agent data and shared project memory |
| [auxiliary-ai.md](auxiliary-ai.md) | Background summaries and next-message suggestions |
| [cli.md](cli.md) | Every command |

## Running it

| | |
|---|---|
| [updates.md](updates.md) | Channels, what waits for what, pinning, recovering a failed switch |
| [history.md](history.md) | Journals, the two retention budgets, compaction, backups |
| [security.md](security.md) | What cxz protects you from and what it does not |

## Internals

| | |
|---|---|
| [architecture.md](architecture.md) | Ownership, the chain, durability, the resource API |
| [glossary.md](glossary.md) | Names for the parts, and names to avoid |
| [development.md](development.md) | Building, tests, images, diagnostic recordings |

## Records

`plans/` and `releases/` hold planning notes and release notes, some in Korean.
They record what was intended at a point in time; the pages above are the
reference, and the code wins over both.

There is no hand-written changelog. Git history is the record of what changed.
