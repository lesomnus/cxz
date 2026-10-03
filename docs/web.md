# Mobile web client

The first web client is for **HTTPS inside a private VPN**, served from the
same origin as its RPC endpoint. It operates existing sessions in the installed
local Manager. It is not a replacement for every TUI or CLI feature.

## Start the gateway

Build or install a cxz version containing this feature on the Manager host.
Create a random browser token (separate from provider credentials) once:

```sh
mkdir -p ~/.config/cxz
(umask 077; openssl rand -hex 32 > ~/.config/cxz/web-token)
```

Obtain a TLS certificate trusted by the phone for the VPN hostname. Run:

```sh
cxz web \
  --listen 0.0.0.0:7350 \
  --origin https://cxz.example.vpn:7350 \
  --tls-cert /path/to/certificate.pem \
  --tls-key /path/to/private-key.pem \
  --access-token-file ~/.config/cxz/web-token
```

Replace the hostname and certificate paths with your own. Open the exact
`--origin` URL on the phone and paste the token into **Web access token**.
The certificate must match that hostname and be trusted by the browser. Limit
network access to your VPN. The default listener is loopback; the example binds
all interfaces and therefore relies on the host's VPN/firewall configuration.
The foreground gateway is a Linux-host command; Windows can use the web client
in a browser. It connects to the local installed Manager via the same transport
as the CLI. Named SSH connections and cross-origin gateways are not supported
in this first UI.

Ctrl+C stops only the gateway. Manager, project runtimes and agents continue.
Restarting the gateway invalidates browser logins; reconnect and sign in again.
A persistent service/process manager can run this foreground command. It is not
yet installed as a service or automatically restarted after a binary update.

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
