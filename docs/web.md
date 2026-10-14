# Web client

A browser client served from the same origin as its RPC endpoint. It operates
existing sessions in the installed local Manager, and is not a replacement for
every TUI or CLI feature.

The session panel keeps the current session highlighted. Its scrollbar shows
only a handle while the panel is hovered, with no visible track or arrow buttons;
revealing it does not change the list width.

The conversation starts directly at the transcript without a title bar. The
terminal toggle sits immediately to the left of Send in the composer toolbar.
The selected session's identity remains in the session panel; narrow layouts
use browser history to return to the session list.

The composer toolbar has a Stop response button and a monospace elapsed clock.
Click twice within three seconds, or press Escape twice within the same window,
to interrupt the current turn through SessionService/Interrupt. This keeps the
agent session available for another message. A single action arms confirmation;
holding Escape does not confirm, and switching sessions or completing a turn
clears it. Menus and preview cards retain their own Escape handling.

The clock follows the latest snapshot and native live events, independently of
historical scrolling. It starts at the recorded input, continues through approval
waits and steering, and resets when the turn finishes. Reconnection preserves a
known start. If retained history lacks the turn's start, it measures from first
observation instead. `ElapsedTime` is a reusable display; unused leading digits
are dimmed without changing the text width.

When the message starts with a slash and the caret is at the end of its first
line, the text area offers fuzzy command suggestions. The selected suggestion
occupies the same line as the native caret, with neighboring commands above and
below it. The overlay follows the editor's text width, padding and line height;
it excludes the line-number gutter and scrollbar and never changes composer
height. Up/Down browse suggestions while keeping the typed query and caret in
place. Right accepts a suggestion as one undoable edit. Escape dismisses the
suggestions before the turn-interrupt shortcut; blur, composition and moving
away from the query hide them. Suggestions are enabled only for the session
composer, leaving question answers as ordinary editable text.

The overlay has vertical inset around its rows and a dark translucent backdrop
with Gaussian blur. The tint lightly darkens the blurred content rather than
concealing it. Padding extends the surface without shifting the selected
line. The backdrop occupies its own layer below the native textarea so the caret
remains visible; applying a filter to the whole overlay would change that stacking.

The catalog contains `/model`, `/effort`, `/compact`, `/download` and Claude's
`/context`, plus model and effort values reported by the provider. Local commands
`/memory`, `/usage`, `/background`, `/aux` and `/@redact` open resource controls
without being sent to the agent. Submitting a
bare model/effort command opens the existing picker; submitting a value uses the
same setting path as that picker, including resetting an explicit effort before
a model change. Suggestions remain visual until accepted, so an unaccepted hint
cannot silently replace submitted text. [Web/TUI parity](web-parity.md) describes
the resource views, input assistance, queued messages and remaining interfaces.

Inside a single-backtick span, a path beginning with `/` or `~` offers file and
directory hints from the session's project container. Up/Down browses; Tab or
Right accepts. Directories continue completion, and a file leaves the existing
closing backtick and surrounding message intact. Fenced code, bare paths, selections
and IME composition do not trigger lookups. Parent listings are briefly cached,
so refining a filename filters locally rather than issuing a request per character.
The same hints work in `/download <path>`, including workspace-relative arguments.
This command downloads a regular file using ProjectService.Download and the
browser's save behavior without sending an agent prompt. Quoted paths with spaces,
absolute paths and container-user home paths are supported. A failed transfer keeps
the command for retry; switching sessions cancels it. Web path access remains scoped
to the owned project container and its remote user. File previews retain their
frontend size limit; downloads use the full file stream.

The browser shares project and session inventories across navigation. Each entity
type has a bounded initial List and a single Watch spanning all listed projects;
project groups use this shared state instead of opening their own requests. The
Watch snapshot streams the full inventory, followed by row additions, updates and
removals. Disconnected streams reconnect with backoff, and signing out cancels
both subscriptions. On the Manager API, empty List or Watch filters default to
all listed resources; explicit filters still allow narrower or archived scopes.
Older Managers that require individual IDs for Watch retain a paginated static
list and show an update message instead of repeatedly retrying the unsupported
subscription. The Manager must be updated to use shared live subscriptions.

On plaintext HTTP origins, Connect subscriptions travel over authenticated
`/rpc-stream/` WebSockets. Their Connect headers, message envelopes, errors and
trailers are forwarded to the same Manager handler. This keeps long-lived
subscriptions out of the browser's HTTP/1.1 connection pool shared across tabs,
so opening more windows cannot block history reads or Send. Closing a subscription
or signing out cancels its native RPC. HTTPS retains the ordinary Connect fetch
transport. The loopback gateway must be updated for the WebSocket endpoint.

The conversation renders messages, tool activity and actionable notices.
The composer toolbar shows the current run's active provider-reported background
jobs beside the elapsed time. Its border lights progressively as jobs are added
and softly drifts and breathes while work remains active, including after the
foreground response finishes. Click the count for background task details.
Unknown telemetry is a dash rather than a guessed zero. The initial state comes
from Session.Background; background events update it without browser polling,
with snapshot refreshes on reconnection and return to the tab. Ordinary
tool launch results do not count as background jobs or mark them completed.
Claude supplies native task telemetry. For Codex, the supervisor queries the
experimental `thread/backgroundTerminals/list` API after thread loading, command
activity, and periodically while the session is connected. It combines paginated
results and publishes changed snapshots through the same Background API and
Watch stream, shared by all browser windows. A successful empty list means zero;
unsupported APIs or failed reads remain unavailable without failing the turn.
Codex counts its reported background terminals, not arbitrary operating-system
processes or subagents. Existing Codex sessions need an updated supervisor and
a session restart to use this integration. See the
[official App Server documentation](https://learn.chatgpt.com/docs/app-server#clean-background-terminals).

Permission requests that resolve automatically before the display grace period
remain in the journal without flashing an approval card above the composer.
Unresolved requests still become actionable cards; human questions appear
immediately. Fixed-width UI text shares the `--ui-font-mono` token, including the
elapsed timer and background count. Terminal and editor defaults resolve that
same token, with explicit editor font preferences taking precedence.

Protocol `raw` records, quota/model polling and internal control acknowledgements
remain in the journal but do not appear as conversation rows. Usage snapshots
still drive composer indicators, and turn-completion events still supply response
metrics. A successful completion represented by a loaded final response is shown
in that response's footer rather than a separate `turn_end completed` row.
Failures, interruptions and completion notices without a matching loaded response
remain visible.

Tool executions stay at their original transcript position. The call, associated
approval, streamed output and result are grouped by run and native execution ID
into one status row, like the TUI. Shell wrappers are shown as the shell name and
the script it runs. Click the row for input, output, result and approval records
in an overlay attached below the selected task. The selected summary's exterior
grows and gains a border while its text and measured row height stay unchanged.
Details emerge from beneath that summary, follow it while scrolling, and close
if the summary leaves view. Click the same summary again, press Escape, or click
an empty transcript side margin to dismiss. The card has no separate Close button;
its height stays within the conversation and long content scrolls internally.
Unrelated approvals and questions retain their existing answer controls;
orphan output/results at a loaded-history edge
remain inspectable until their call is loaded.

History paging follows the rendered content, including after viewport changes.
Short pages containing mostly protocol traffic automatically load older pages
until the pane has reading room, without requiring a scrollbar first. The initial
tail is fetched through the session snapshot; near-edge paging follows the reading
direction rather than alternating both ends. Cache trimming prefers conversation
and tool records over internal traffic, retains fetched pagination boundaries and
keeps metadata separately. Row measurements and viewport changes preserve the
reader's position or keep the latest response in view when following.

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

File bodies pass through the authenticated attachment proxy to the Manager's
private raw HTTP handler when supported, with an Upload RPC adapter for older
Managers. [Attachment data path](attachment-data-path.md) documents the rollout
and measured CPU/allocation improvements.

Vite terminates `/rpc-stream/` locally and forwards the Connect HTTP stream to
the installed gateway, so this development path also works with older gateways.
Restart Vite after changing its gateway plugin; a browser reload starts the new
transport. `playwright.dev.config.ts` exercises simultaneous windows against
this actual local development bridge.

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

Vite refreshes frontend code, not the running gateway. If container path hints
report `permission_denied: path outside project workspace`, the gateway still
has the older workspace-only restriction. Refresh its image after updating cxz,
or run the gateway from the updated checkout on the Manager host:

```sh
npm --prefix ts run build
cxz web down
go run ./cmd/cxz web serve --assets-dir ./internal/webui/assets
```

Keep that foreground process running while using Vite. This replaces only the
web gateway; existing projects do not need to be recreated. Running `web up`
without updating its image uses the same older gateway code.

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
and serves the UI files packaged in the image, browser authentication and an HTTP proxy to the Manager's Connect API. It mounts only the
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

## Current support

| Area | Web support |
| --- | --- |
| Projects and sessions | Shared inventories and watches, creating/opening sessions, archive/restore, editable titles and aliases |
| Accounts | Profile registration, central Codex login, isolated Claude session login and helper login |
| Search | Indexed server search, project/date/match filters, result paging and event navigation |
| Shared memory | Project library, revision-checked documents, snapshots, forks, merge, import, rename and trash controls |
| Conversation | Virtualized paged history, live events, Markdown, highlighted code, model/effort snapshots, response metrics and copy |
| Drafts | Per-session drafts and paste chips survive navigation and reload in tab-local sessionStorage |
| Sending | Explicit Send or Ctrl+Enter; mutation IDs; no automatic replay after ambiguous failure |
| Session control | Interrupt, stop/resume, restart, purge with dry-run review and confirmation; queued-message cancel/recall |
| Approval | Allow/deny and full tool input/output/approval inspection |
| Questions | Claude choices/free text/previews; Codex requestUserInput and asynchronous questions; recorded replies |
| MCP | Basic request display; schema-based elicitation forms still require the TUI |
| Model and usage | Model/effort selectors, account quota/reset, context gauge, per-response metrics and retained conversation statistics |
| Work status | Active sessions, pending requests, provider background-task reports |
| Files and input assistance | File/folder upload, large paste attachments, path completion, /download, project session mentions and secret file chips |
| Workspace editor | Wide-view Monaco file preview; explicit Connect embeds OpenVSCode in the project devcontainer |
| Workspace shell | Ctrl+Backquote or toolbar toggle; retained terminal tabs, drag docking, resize, WebGL with fallback |
| Settings | Browser-local JSON document, language/theme, editor fonts/indentation/palette, terminal behavior, voice input and sounds |
| Attention | Unread session markers, favicon badges and completion/question sounds after browser interaction |
| Voice | Browser speech recognition, configured language or browser preference, optional local-only recognition |
| Auxiliary AI | Live response summaries, helper profile configuration, automatic preferences, run/cancel, suggestions/title results and task usage |
| Authentication | Token login, Secure/HttpOnly/SameSite cookie, sign-out and expiry |

There is no UI yet for permission policy selection, model catalog refresh/provenance,
MCP/skills management and schema elicitation forms, installation/history/version
maintenance, logs/debug recording, multiple composer cursors, line move/duplicate
shortcuts or multiple Manager connections. [Web/TUI parity](web-parity.md) tracks
these remaining interfaces. The Manager's RPC availability is independent of which
UI controls exist.

A disconnected mutation may have reached the Manager even if the browser did
not receive its result. The draft remains; check the conversation before
explicitly retrying. Switching tabs or suspending the phone does not cancel an
agent's turn. A retention gap means the missing journal entries cannot be
recovered from the browser.

## Architecture and authentication

The Manager exposes its **actual registered gRPC server** through payday on a
private HTTP Unix socket, `STATE/run/web.sock`, with owner-only permissions. All
registered services, their generated method definitions and the existing Manager
interceptors are used by Connect/gRPC-Web. Adding a Manager RPC does not require
a corresponding web implementation or forwarding method. Native gRPC continues
on `STATE/run/daemon.sock`; project-container runtimes do not open the web socket.

The `cxz web` frontend server forwards authenticated RPC requests as HTTP to that
socket, without decoding messages or maintaining a second service registry. A
host-side `web serve` reaches an installed Manager through `docker exec` and
`_web-bridge`; the installed web container uses its existing read-only run mount.
No additional network port is opened on the Manager. Browser cookies remain in
the frontend server; request cancellation, streaming responses, RPC errors and
metadata pass through to the Manager.

The frontend still owns UI files, browser authentication and the HTTP/WebSocket
bridges for terminals, uploads and OpenVSCode. `POST /editor/connect` prepares the
workbench bridge and keeps its connection token server-side. Ordinary resource
RPCs, including `Session.Patch`, `Session.Purge` and `Session.AuxEvents`, go
directly to the registered Manager handlers. The browser uses generated
protobuf-es descriptors, Connect transport and payday Store/Queries; journal
Events is a separate ordered stream from resource Watch.

Both the Manager and web frontend need this architecture change. Updating only
the UI or frontend against an older Manager produces a `502` explaining that
the Manager Connect endpoint is unavailable. Update/restart the Manager first,
then refresh the web frontend. Projects do not need recreation for this listener
change. A foreground `web serve` is refreshed by restarting that process.

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
external images are disabled. Assets are served from the files packaged in the web container; they are not
embedded in Go binaries. Source deployments can select a built assets directory.

The broader feature contracts and support matrix are tracked separately in
PR #78; this page describes the current implementation in this checkout.

## UI development without a server

Use the [WASM design sandbox](web-sandbox.md) to preview the same UI with
simulated sessions, approvals and tool progress. It is a separate development
entry point and is not included in the installed web gateway.
