# Mobile web client

The first web client is for **HTTPS inside a private VPN**, served from the
same origin as its RPC endpoint. It operates existing sessions in the installed
local Manager. It is not a replacement for every TUI or CLI feature.

## Install a background gateway

On the Linux Manager host, install/update cxz and the Manager first. Create a
browser token once, separate from provider credentials:

```sh
mkdir -p ~/.local/state/cxz
(umask 077; openssl rand -hex 32 > ~/.local/state/cxz/web-token)
```

Create `~/.local/state/cxz/web.json` (the default state directory):

```json
{
  "listen": "127.0.0.1:7350",
  "origin": "https://cxz.example.vpn:7350",
  "tls_cert": "/absolute/path/to/certificate.pem",
  "tls_key": "/absolute/path/to/private-key.pem",
  "access_token_file": "web-token"
}
```

Use a certificate trusted by the phone for that exact VPN hostname. Change
`listen` to your host's VPN IP and port to allow phone access; `127.0.0.1` only
accepts local connections. `0.0.0.0:7350` listens on all interfaces and requires
firewall rules restricting access to the VPN.

```sh
cxz install web
```

This creates `<manager-container>-web`, using the installed Manager's image,
with Docker's `unless-stopped` restart policy. It survives terminal closure and
host reboot. The container serves both the embedded UI and payday Connect API.
It mounts only the Manager state volume's `run` subdirectory and the certificate,
key and token files, read-only; it receives no Docker socket or database mount.
Those files must exist on both the command host and the Docker engine host at
the configured absolute paths. This matches a normal host-local Docker install;
remote engines need explicitly shared files. The Manager must already be running.

Open the exact `origin` URL on the phone and paste the token file's contents into
**Web access token**. Windows can use the browser, but installation runs on Linux.

## Configuration and lifecycle

Both `cxz web` and `cxz install web` accept the same configuration:

```sh
cxz install web --config /path/to/web.json --listen 192.0.2.10:7350
cxz web --config /path/to/web.json
```

- Default file: `STATE/web.json`, where `STATE` follows `--state`, `CXZ_STATE`,
  `$XDG_STATE_HOME/cxz`, then `~/.local/state/cxz`.
- `--config` selects a different JSON file. Unknown keys are rejected.
- Explicit `--listen`, `--origin`, `--tls-cert`, `--tls-key` and
  `--access-token-file` override file values.
- Relative file paths in JSON resolve against the JSON file's directory;
  relative command-line paths resolve against the current directory. Use absolute
  paths rather than `~` inside JSON.
- Successful installation saves the resolved settings in
  `STATE/web-installation.json`. These contain file paths, never token/key values.
  If no default `web.json` exists, subsequent commands reuse this saved configuration.
- Re-run `cxz install web` after changing configuration, renewing certificate files
  or rotating the token. It replaces/starts the web container. An invalid local
  certificate or token is rejected before stopping the old gateway.

`cxz self-update` on the Manager host refreshes a running installed web container
after the Manager, using the same image. `cxz use VERSION`, `cxz use @edge` and
`cxz use @stable` also refresh it as part of the host version switch. Missing or
stopped web containers are not started by updates. `--client-only` and Windows
frontend updates do not update a remote gateway. A foreground `cxz web` process
is not managed by these commands; background automatic Manager rollout is also
separate from this host-command integration.

A target image must support `cxz web`; uninstall the web gateway before selecting
an older release without that command. Installation checks this before replacing
the gateway. If startup fails, the previous container is restored when possible,
and the update reports failure rather than claiming the web update succeeded.
Manager/agent versions are not rolled back by a web failure. A version switch
that fails here remains retryable using the same `cxz use` command.

```sh
cxz uninstall web        # removes only web; configuration/token/certificates remain
cxz install web          # installs it again from configuration
cxz uninstall            # removes web and Manager; retains projects and data
```

Use `docker logs <manager-container>-web` for startup diagnostics and
`docker stop <manager-container>-web` to temporarily stop it. A gateway restart
invalidates browser logins; sign in again. It does not stop agent turns.
`cxz web` still runs in the foreground; Ctrl+C stops only that gateway.

## First-version support

| Area | Web support |
| --- | --- |
| Projects and sessions | Listed projects/sessions, project filter, pagination, opening existing sessions |
| Conversation | Retained history, live events, Markdown, code blocks, response copy, raw event details |
| History | Last 2,000 events in browser memory; Manager retention still applies; latest button |
| Drafts | Per-session in-memory drafts survive navigation within the tab, not reload/sign-out |
| Sending | Explicit Send button; mutation IDs; no automatic replay after ambiguous failure |
| Session control | Interrupt turn, stop, resume |
| Approval | Allow/deny and raw payload inspection |
| Questions | Claude AskUserQuestion and Codex requestUserInput choices/free text |
| MCP | Server name/message/raw request; complex elicitation forms require the TUI |
| Connectivity | Same-origin Connection, cursor reconnect, deduplication and retention gap notice |
| Authentication | Token login, Secure/HttpOnly/SameSite cookie, sign-out, 12-hour expiry |

Not yet implemented: session/project creation and deletion, Settings, version
panels, model/effort/permission controls, structured usage metrics, auxiliary AI
controls, file transfer, slash-command UI, secret attachments, async Codex
question forms, PWA installation/push or multiple connections. Raw tool details
are available, but the TUI's specialized input/output presentation is not yet
ported. The gateway exposes only the RPC subset used here; unimplemented RPCs
fail rather than silently gaining a second implementation.

A disconnected mutation may have reached the Manager even if the browser did
not receive its result. The draft remains; check the conversation before
explicitly retrying. Switching tabs or suspending the phone does not cancel an
agent's turn. A retention gap means the missing journal entries cannot be
recovered from the browser.

## Architecture and authentication

`payday/web` translates Connect and gRPC-Web into registered gRPC services.
The gateway forwards the existing generated ProjectService and SessionService
requests to the Manager. Lifecycle validation, run IDs, mutation handling and
storage remain in the Manager. The browser uses generated protobuf-es service
descriptors, Connect transport and payday Store/Queries; journal Events is a
separate ordered stream from payday's resource Watch.

There is no REST conversation API. `/auth/login`, `/auth/status` and
`/auth/logout` are browser-session endpoints. The first version has one
installation-owner capability, not multi-user accounts or per-project ACLs.
The browser token therefore authorizes conversation reads and available actions
across the connected Manager's projects. Do not distribute it to untrusted users.

The token stays out of URLs and localStorage. Login creates a random, expiring
server-side session. Logout/expiry cancels that session's active gateway
requests; agent turns remain independent. Origin and Host must match the
configured HTTPS origin, including for mutation requests. Cross-origin access
is disabled. Markdown is sanitized, embedded HTML cannot execute scripts, and
external images are disabled. Assets are embedded in the binary so source-based
updates do not require Node on the deployment host.

The broader feature contracts and support matrix are tracked separately in
PR #78; this page describes only the implementation shipped by the web PR.

## UI development without a server

Use the [WASM design sandbox](web-sandbox.md) to preview the same UI with
simulated sessions, approvals and tool progress. It is a separate development
entry point and is not included in the installed web gateway.
