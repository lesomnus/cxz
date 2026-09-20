# Remote TUI and Windows frontend

The manager, wasp, agent processes and devcontainers run on Linux. The same
`connect` command runs on Linux and Windows; the Windows binary contains the
frontend rather than server/install commands. No local Docker installation is
needed for a remote conversation.

## All connections in one project view

Edit `settings.jsonc` with `cxz edit` on Linux or Windows. JSON comments (`//`,
`/* ... */`) and trailing commas are supported. Connections are keyed by name
alongside a `default` selection:

```json
{
  "$schema": "./settings.schema.json",
  "connections": {
    "default": "work",
    "work": { "target": "ssh://work" },
    "home": { "target": "local://" },
    "vpn": {
      "target": "tcp://home-vpn:7349",
      "token_file": "${STATE}/tokens/home"
    }
  }
}
```

Run `cxz` or `cxz tui` on Linux, or `cxz` on Windows, to see every configured
connection. Existing settings fields such as `files`, `docker` and model
preferences can remain in the same file. Connection changes apply the next time
the TUI starts.

On Windows, `cxz edit` edits the local client's settings without requiring Docker
or a live daemon. It uses `VISUAL`, then `EDITOR`, and otherwise opens Notepad.
With Notepad, save and close the document, then press Enter in the terminal.
For VS Code, set its wait option so editing finishes before validation:

```powershell
$env:EDITOR = 'code --wait'
.\cxz.exe edit
```

Editor commands use Windows command syntax; quote an executable path containing
spaces. Both executable and `.cmd`/batch editors are supported. Unless overridden
with `--state`, `CXZ_STATE` or `XDG_STATE_HOME`, the file is under
`$env:USERPROFILE\.local\state\cxz\settings.jsonc`. Directory selection follows
`--state` > `CXZ_STATE` > `XDG_STATE_HOME/cxz` > the home-directory default.

The first edit starts with commented examples for connections, model defaults,
file mappings and shared Docker. The examples are disabled until uncommented.
Closing the editor successfully saves that initial file even if it was not
changed. Existing settings keep their preferences and comments; `config
set`/`unset` also preserves unrelated settings and comments.

`cxz edit` extracts the bundled JSON Schema to `<state>/settings.schema.json`
before opening the editor and refreshes it when the binary's schema changes.
The initial file contains `"$schema": "./settings.schema.json"`; editing an
existing file also adds this reference if missing. A custom `$schema` is kept.
The relative reference works for both the temporary draft and the saved file,
including Windows paths with spaces. No daemon or schema download is required.
The schema provides descriptions, completions and validation in compatible
editors; cxz still validates the configuration on save.

VS Code recognizes the `.jsonc` extension as JSON with Comments without a
custom file association.

When `settings.jsonc` is absent, cxz reads `settings.jsonm`, then the original
`settings.json`. The next successful `cxz edit` or CLI preference save writes
`settings.jsonc`, preserving the old file as a backup. When multiple files
exist, the first in that order is used.

The editor opens a temporary draft. Invalid JSON, unknown settings, editor
failure and concurrent updates leave the saved file intact and report the
recoverable draft path. Windows saves connection/model preferences locally;
it does not publish Docker overrides or file mappings to any remote connection.
Edit those host settings with `cxz edit` on the Linux daemon host.

Projects display as `project1 via work`, with their sessions underneath.
`default` chooses the initial focus and the fallback for an unqualified session
reference; it does not hide other connections. If omitted, the alphabetically
first connection is selected. `cxz connect home` opens the same combined view
with home initially focused. `cxz connect --session SESSION work` selects a
session ID or alias on work; a qualified `work::SESSION` reference can also be
used.

Each connection loads and subscribes independently. An unreachable host does not
delay projects from another host. Failed connections retry; previously loaded
projects remain visible with an offline marker. Empty/connecting connections
have a row, so they can still be selected for accounts, settings or a new
workspace. IDs are scoped within the frontend, so equal IDs on different daemons
do not share drafts, event history or pending approvals.

Accounts, new sessions and Ctrl+P Docker settings target the selected connection
(or the current conversation when the project panel is not focused).
An open settings/accounts view retains that target. Memory copy targets are
limited to stopped sessions for the same agent on the same connection; copying
between daemon hosts is not implemented.

Targets:

- `ssh://work` uses the `Host work` entry in the client's SSH config.
- `local://` discovers the installation for the client's `--state` directory.
  With `cxz install`, this normally bridges to the manager inside Docker: the
  manager's Unix socket need not be mounted on the host. An alternative local
  installation can use `local:///absolute/state/directory`.
- `unix://${STATE}/run/daemon.sock` connects directly to a Unix socket, useful
  with a foreground `cxz serve`. `${STATE}` expands to the local client state
  directory; it is not the remote host's state. Local Docker and Unix socket
  targets are intended for Linux clients.
- `tcp://host:port` requires `token_file`. Relative token paths resolve against
  the client state directory; `${STATE}` also works in token paths. Token
  contents are never stored in settings.

Without `connections`, Linux `cxz` retains its existing local-installation
behavior. CLI management commands such as `cxz up`, `install` and
`project recreate` continue to target the local installation. Passing a URL
to `cxz connect` opens just that endpoint.

## SSH

Update the **host CLI** on the Linux daemon host so it provides `_connect`, then:

```sh
cxz connect ssh://user@linux-host
cxz connect --session SESSION_ALIAS ssh://user@linux-host:2222
cxz connect 'ssh://work-host?state=/home/user/.local/state/cxz&binary=/usr/local/bin/cxz'
```

The client runs OpenSSH (`ssh` / `ssh.exe` must be on PATH), honors SSH config
aliases, keys, agent and known-host policy, and uses batch authentication without
a PTY. Set up access with normal `ssh` first. Passwords embedded in URLs are
rejected; there is no password prompt inside the TUI. `state` and `binary` are
optional URL query parameters and must be URL-encoded where necessary.

The remote `_connect` process locates the host's existing cxz installation and
bridges gRPC through `docker exec` to its manager, or to the local Unix socket
for a foreground daemon. It does not start another daemon. SSH keeps the transport
encrypted; no TCP listener or token is needed. Closing the frontend closes its
SSH process, while sessions continue under wasp.

## TCP

On the Linux host, generate a private access token once, then run a foreground
forwarder for the existing installation:

```sh
umask 077
openssl rand -hex 32 > ~/.config/cxz-remote.token
cxz expose --token-file ~/.config/cxz-remote.token --listen tcp://127.0.0.1:7349
```

The parent directory of the token file must already exist. Keep the forwarder
running (or supervise it as a service). It preserves the local manager's database,
socket and lifecycle. It can be stopped and restarted without restarting the
manager or agents; `cxz install --expose` is not needed. Ctrl+C stops only the forwarder. By default it listens on
loopback; choose an explicit host/VPN interface to accept connections there.

Copy the token securely to the client, then:

```sh
cxz connect --token-file ./cxz-remote.token tcp://HOST:7349
```

TCP carries **plaintext gRPC**. Use it on loopback, through a tunnel or on a
trusted VPN; use SSH for encrypted access over an untrusted network. The bearer
token grants full access to this cxz installation, including mutations; it is
not an agent credential or a project-scoped capability. Authentication applies
to unary requests and streaming subscriptions. Both sides read the token from a
file; tokens are not accepted in endpoint URLs. Restart the forwarder after
rotating the token. A token must have at least 32 non-whitespace bytes.

Both frontends accept `CXZ_ENDPOINT` as the default for `cxz connect` and
`CXZ_TOKEN_FILE` as the default token path. The Windows frontend also uses these
when invoked with no command (an endpoint environment variable overrides the
configured combined view in that case). Without the environment variable,
Windows `cxz` uses the configured connections. Linux `cxz` uses configured
connections when present, otherwise its local installation. `--state` is always the **local client** settings/recordings
directory; SSH's `?state=...` selects the daemon host's installation.

## Remote functionality and boundaries

Project/session browsing, conversation streaming, send/interrupt/stop, permission
policies, quota, tool previews, retained memory and Docker settings use the remote
gRPC API. Recordings and terminal diagnostics describe the **client** terminal
and are saved locally. Workspace paths entered in remote mode must be absolute
Linux paths on the daemon host; the client does not resolve them against a
Windows drive or its current directory. Provider accounts/logins should be set
up on the daemon host before attaching remotely.

The existing embedded container terminal, workspace path completion and secret
helper still depend on direct local Docker access. In remote mode they report
that host-local access is required, rather than touching the client's Docker
engine. Interactive provider login is also performed on the host. File-based
memory browsing/copying and log retrieval already go through the daemon and work
remotely. This change does not add a remote PTY/helper RPC.

## Builds

```sh
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o cxz.exe ./cmd/cxz
GOOS=windows GOARCH=arm64 CGO_ENABLED=0 go build -o cxz-arm64.exe ./cmd/cxz
```

Release scripts build Linux amd64/arm64 runtime archives and Windows amd64/arm64
frontend ZIPs, all covered by `SHA256SUMS`. CI includes a native Windows frontend
build and command/transport tests. Linux container images remain Linux-only.
