# Remote TUI and Windows frontend

The manager, wasp, agent processes and devcontainers run on Linux. The same
`connect` command runs on Linux and Windows; the Windows binary contains the
frontend rather than server/install commands. No local Docker installation is
needed for a remote conversation.

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
socket and lifecycle. Ctrl+C stops only the forwarder. By default it listens on
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
when invoked with no command. Linux `cxz` without a command still opens its local
installation. `--state` is always the **local client** settings/recordings
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
