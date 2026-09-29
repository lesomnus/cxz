# MCP management

MCP registrations belong to the connected host manager, not the frontend machine.
Settings → MCP servers lists global defaults. Tab switches to the selected
project. Enter/Space toggles activation; `i` removes a project override and
inherits the global default again. `a` registers a local stdio or remote HTTP
server (new registrations start disabled). Mouse clicks toggle rows too.

Project creation does not copy defaults. Projects inherit current global defaults
unless an explicit on/off override exists. Changing P never changes Q. Changing
a global default affects only projects still inheriting that server. Activation
is project-wide, including all providers/accounts in that project.

The project page reports running sessions whose launch settings differ from the
saved settings. Changes apply when a new agent starts or an existing agent is
explicitly restarted. Saving settings never restarts an agent, injects a user
message, or resends a prompt. Disabled servers are omitted from cxz's agent MCP
configuration; their tools and built-in usage instructions are not advertised.
Existing turns/history may still contain earlier tool descriptions or results.

## CLI

```sh
cxz mcp add playwright ./playwright.json
cxz mcp enable playwright
cxz mcp disable --project P playwright
cxz mcp inherit --project P playwright
cxz mcp list --project P
cxz mcp remove playwright
cxz mcp logs --project P --session SESSION playwright
cxz mcp restart --project P --session SESSION playwright
```

`add` takes a JSON definition and replaces an existing external registration with
the same ID. `remove` also removes its project overrides. Built-ins cannot be
replaced or removed, but use the same activation settings. IDs contain ASCII
letters, numbers, underscores and hyphens (up to 64 characters).

```json
{
  "name": "Playwright",
  "kind": "stdio",
  "enabled": false,
  "command": "npx",
  "args": ["-y", "@playwright/mcp"],
  "env": {}
}
```

The command executes inside the project's container, with the session workspace
as its current directory. The command and its dependencies must be available
there. cxz does not install Node/Python or provide a package gallery; package
runners such as npx can install packages themselves. Pin package versions when
reproducibility is required. Environment values are literal (no shell expansion).
Only PATH, HOME, LANG, LC_ALL and TMPDIR are inherited; registered `env` values
are added explicitly. Agent account credentials are not inherited.

```json
{
  "name": "Remote tools",
  "kind": "http",
  "enabled": false,
  "url": "https://example.com/mcp",
  "headers": {"Authorization": "Bearer ..."}
}
```

HTTP uses each agent's native Streamable HTTP client. The manager owns the
registration and project activation; the agent owns the HTTP connection. No
custom OAuth login UI, shared remote authentication broker, or legacy SSE-only
adapter is included. Headers and environment values are stored in private files
and omitted from list responses. This is private file storage, not an encrypted
credential vault. Do not put credentials in names, command arguments or URLs.

## Runtime ownership

```text
Manager: registrations + project overrides
    ↓ resolved launch settings
Project runtime
    ├── built-in providers (in process, shared project storage)
    └── external stdio children (isolated per agent connection)
           ↑ private Unix socket
Agent → cxz _mcp-bridge (stdio)
```

A built-in provider registers its implementation with the common runtime. The
bridge contains no tool implementation. Each launch has a random capability and
immutable configuration snapshot. The runtime checks that capability and binds
the connection to the retained session identity; tool arguments do not choose
another session. This routing check is not an OS security boundary against
processes with the same user and full container filesystem access.

A runtime restart does not terminate agents. Bridges reconnect on the next
request and repeat only MCP initialization. In-flight calls receive an
unknown-outcome error and are never automatically replayed. Messages through the local bridge are limited to 4 MiB. Stateful external
servers lose their in-memory state; resource subscriptions are not restored.
`mcp restart` disconnects a selected local MCP so the next request reconnects;
it preserves the agent's launch configuration and does not apply new activation
settings. HTTP lifecycle remains agent-managed.

Local stderr is bounded to 1 MiB per connection in the session directory and is
reset on reconnect. `mcp logs` reads it explicitly and masks registered environment
values. MCP tool payloads are not separately logged by the runtime.

The manager stores `mcp.json`; project runtimes store `mcp-runtime.json` and each
session's `mcp-launch.json`. Existing databases need no migration or deletion.
