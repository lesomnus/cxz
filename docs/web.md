# Web client

A browser client served from the same origin as its RPC endpoint. It operates
existing sessions in the installed local Manager, and is not a replacement for
every TUI or CLI feature.

The session panel keeps the current session highlighted. Its scrollbar shows
only a handle while the panel is hovered, with no visible track or arrow buttons;
revealing it does not change the list width.

The conversation renders messages, tool activity and actionable notices.
Protocol `raw` records, quota/model polling and internal control acknowledgements
remain in the journal but do not appear as conversation rows. Usage snapshots
still drive composer indicators, and turn-completion events still supply response
metrics; diagnostics and errors remain visible.

## A browser on this desktop

On the Linux Manager host, with the Manager already running:

```sh
cxz web up
```

With nothing configured that writes two files and prints where to go:

```
Generated ~/.local/state/cxz/web-token
Generated ~/.local/state/cxz/web.json
cxz web up: cxz-manager-web · http://127.0.0.1:7350
Open http://127.0.0.1:7350 and sign in with the token in ~/.local/state/cxz/web-token
```

There is no certificate to make, no hostname to pick and no trust store to
touch, because **a loopback origin is already a secure context**: Chrome and
Firefox give `http://127.0.0.1` the same treatment as HTTPS, including the
`__Host-` session cookie this gateway signs in with. The bytes TLS would have
protected never reach a network.

`127.0.0.1` rather than `localhost` is deliberate. `localhost` resolves to `::1`
first on many systems, and an SSH tunnel binds IPv4 loopback only, so the one
spelling works in both places.

Plaintext is confined to that case. An `http` origin must name a loopback
address, and a gateway with no certificate is refused on a published address a
network can reach — `cxz web up --listen 0.0.0.0:7350` fails rather than putting
an unencrypted gateway on your LAN.

## Developing the UI against the installed Manager

On the same host as cxz, start the gateway, then run Vite from the checkout:

```sh
cxz web up
cd ts
npm ci
npm run dev
```

Open `http://127.0.0.1:5173/`. UI edits use Vite HMR; API calls, editor connections
and terminal WebSockets are proxied to the installed gateway. This uses real
projects and sessions, including real sends and decisions.

The dev server reads `STATE/web-installation.json` first, so gateway address and
token-path overrides from the last successful `cxz web up` are respected. It
falls back to `STATE/web.json`. `STATE` follows `CXZ_STATE`, `$XDG_STATE_HOME/cxz`,
then `~/.local/state/cxz`, like the CLI. To explicitly select another configuration:

```sh
CXZ_WEB_CONFIG=/absolute/path/to/web.json npm run dev
```

The default token file is `STATE/web-token`; `access_token_file` in the selected
configuration can point elsewhere, including a path relative to that JSON file.
On the initial browser authentication check, Vite reuses an existing gateway
session or reads this file and signs in server-side. Only the gateway's HttpOnly
session cookie reaches the browser. The access token is never included in UI
assets, browser storage or dev-server logs, and is read again for each new login
so token rotation does not require restarting Vite. Sign out still works; a page
reload signs in automatically again in this local development mode.

Automatic sign-in binds Vite to loopback HTTP and checks the browser origin
before rewriting headers for the gateway. Production builds and `npm run sandbox`
do not load this dev-only configuration. Stop the sandbox before starting dev if
it is using the same port; `npm run dev -- --port 5174` selects another.

## Reaching it from another machine

Two ways, and the first needs nothing new:

- **An SSH tunnel.** `cxz connection tunnel work` forwards `127.0.0.1:7350` on
  your client to the same address on the host, so the loopback origin matches on
  both ends and SSH encrypts the hop. See
  [remote access](remote.md#browsing-through-the-tunnel).
- **HTTPS on a reachable address**, for a device that cannot tunnel, such as a
  phone on a private VPN. That needs a certificate the device trusts:

```json
{
  "listen": "192.0.2.10:7350",
  "origin": "https://cxz.example.vpn:7350",
  "tls_cert": "/absolute/path/to/certificate.pem",
  "tls_key": "/absolute/path/to/private-key.pem",
  "access_token_file": "web-token"
}
```

Use a certificate for that exact hostname. `0.0.0.0:7350` listens on every
interface and requires firewall rules restricting access to the VPN. cxz does
not issue certificates or install them into any trust store.

## What `web up` creates

`<manager-container>-web`, using the installed Manager's image, with Docker's
`unless-stopped` restart policy. It survives terminal closure and host reboot,
and serves both the embedded UI and the payday Connect API. It mounts only the
Manager state volume's `run` subdirectory, the token file, and the certificate
and key when those are configured, all read-only; it receives no Docker socket
and no database mount. Configured files must exist on both the command host and
the Docker engine host at the same absolute paths — a normal host-local Docker
install satisfies this; remote engines need the files explicitly shared.

```sh
cxz web status           # container, origin, token, image skew
cxz web down             # remove the container; configuration and token stay
```

`status` answers "is it up, what does it serve, and is it current" without
`docker inspect`. It reports the gateway's image against the Manager's: after a
version switch that could not refresh it, the gateway serves the previous
version's UI against this version's Manager, which is the one skew a browser
cannot show you.

## Configuration and lifecycle

`web up`, `web serve` and `web status` read the same configuration:

```sh
cxz web up --config /path/to/web.json --listen 192.0.2.10:7350
cxz web serve --config /path/to/web.json
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
- `web up` generates `STATE/web.json` and `STATE/web-token` only when neither
  default file exists, and never overwrites either one: a token already pasted
  into a browser keeps working. A `--config` path that does not exist is an
  error, not a cue to generate one.
- Re-run `cxz web up` after changing configuration, renewing certificate files
  or rotating the token. It replaces/starts the web container. An invalid local
  certificate or token is rejected before stopping the old gateway.

`cxz self-update` on the Manager host refreshes a running installed web container
after the Manager, using the same image. `cxz use VERSION`, `cxz use @edge` and
`cxz use @stable` also refresh it as part of the host version switch. Missing or
stopped web containers are not started by updates. `--client-only` and Windows
frontend updates do not update a remote gateway. A foreground `cxz web serve`
process is not managed by these commands; background automatic Manager rollout is also
separate from this host-command integration.

A target image must support this gateway; take it down with `cxz web down`
before selecting a release that predates the command it starts. Installation checks this before replacing
the gateway. If startup fails, the previous container is restored when possible,
and the update reports failure rather than claiming the web update succeeded.
Manager/agent versions are not rolled back by a web failure. A version switch
that fails here remains retryable using the same `cxz use` command.

```sh
cxz web down             # removes only web; configuration/token/certificates remain
cxz web up               # creates it again from configuration
cxz uninstall            # removes web and Manager; retains projects and data
```

Use `docker logs <manager-container>-web` for startup diagnostics and
`docker stop <manager-container>-web` to temporarily stop it; `cxz web status`
reports either state. A gateway restart invalidates browser logins; sign in
again. It does not stop agent turns. `cxz web serve` runs the same gateway in
this terminal for developing cxz itself, and Ctrl+C stops only that gateway.

## First-version support

| Area | Web support |
| --- | --- |
| Projects and sessions | Listed projects/sessions, project filter, pagination, opening existing sessions |
| Conversation | Retained history, live events, Markdown, code blocks, response model/effort snapshots beside the agent logo, response copy, raw event details |
| History | Last 2,000 events in browser memory; Manager retention still applies; latest button |
| Drafts | Per-session in-memory drafts survive navigation within the tab, not reload/sign-out |
| Sending | Explicit Send button; mutation IDs; no automatic replay after ambiguous failure |
| Session control | Interrupt turn, stop, resume |
| Approval | Allow/deny and raw payload inspection |
| Questions | Claude AskUserQuestion and Codex requestUserInput choices/free text |
| MCP | Server name/message/raw request; complex elicitation forms require the TUI |
| Connectivity | Same-origin Connection, cursor reconnect, deduplication and retention gap notice |
| Workspace editor | Wide-view read-only Monaco file preview; explicit Connect embeds OpenVSCode in the session project devcontainer; see [editor contract](web-editor.md) |
| Workspace shell | Ctrl+Backquote or title-bar button opens a project terminal below the composer; folding retains shell state; see [terminal contract](web-terminal.md) |
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
