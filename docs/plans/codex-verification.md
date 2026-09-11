# Codex CLI verification worksheet

What cxz needs to know about Codex CLI before the agent adapter seam (§3.4 of
[architecture.md](../architecture.md)) can be fixed.

**Tested against `codex-cli 0.154.0` (release tag `rust-v0.154.0`), x86_64,
Debian 13 devcontainer, 2026-09-11.** Codex moves fast; re-check before relying
on any detail here.

Items are `C0`…`C12`. Everything reachable without a Codex account has been
run; what remains needs an authenticated session — see
[What is still needed](#what-is-still-needed).

---

## Installed in this container

```
~/.local/bin/codex              0.154.0   static-pie musl, 263 MB
~/.local/bin/codex-app-server   0.154.0   204 MB
~/cxz-codex-probe/pkg/          full package tree (bin + resources + path)
~/cxz-codex-probe/schema/       39 app-server protocol JSON schemas
~/cxz-codex-probe/config-schema.json      config.toml schema (204 KB)
```

`~/.local/bin` was added to `~/.zshrc`. `~/.codex` was deliberately removed
after install so the first-run experience (C9) can still be observed cleanly.
Probes ran under throwaway `CODEX_HOME` directories.

---

## Answered

### C10 — Distribution and pinning ✅

- Release tag is `rust-v<version>`; the version itself has no prefix.
- **Linux ships musl only, `static-pie`** — no gnu build, and none needed. One
  artifact runs in any container regardless of libc. Simpler than Claude Code,
  whose platform key encodes musl-vs-glibc.
- **Two artifact shapes, and the difference matters:**
  - bare binary (`codex-x86_64-unknown-linux-musl.tar.gz`) — a single
    executable, signed with a `.sigstore` file, **not** covered by
    `codex-package_SHA256SUMS`
  - **package** (`codex-package-x86_64-unknown-linux-musl.tar.gz`) — a tree,
    and the only shape with a published SHA-256
- Also published: npm tarballs, Python wheels, and `install.sh`. Pulling the
  GitHub release directly stays the most controllable.
- `config-schema.json` ships as a release asset — a machine-readable schema for
  `config.toml`, which is how C9 should be answered rather than by trial.

**The package is a tree, not a binary:**

```
bin/codex                       317 MB with its sibling
bin/codex-code-mode-host
codex-package.json              layoutVersion, entrypoint, resourcesDir, pathDir
codex-path/rg                   bundled ripgrep (5.2 MB)
codex-resources/bwrap           the Linux sandbox (1.4 MB with zsh)
codex-resources/zsh/bin/zsh
```

**Consequence for §3.4.** cld's "copy one binary, verify its checksum, swap a
symlink atomically" does not transfer. `ReleaseSource` must be able to install a
**directory tree** with a manifest, and cxz should prefer the package shape
because it is the one with a published checksum. `codex-package.json` describes
the layout, so it can be read rather than hardcoded. A bonus: the package
bundles its own `rg`, so a Codex container needs no separate ripgrep injection.

### C1 — Codex's own sandbox does not work in a container ✅

The bare binary panics outright:

```
thread 'main' panicked at linux-sandbox/src/launcher.rs:51:13:
bubblewrap is unavailable: no system bwrap was found on PATH and no bundled
codex-resources/bwrap binary was found next to the Codex executable
```

With the package's bundled `bwrap` present, it gets further and still fails:

```
bwrap: loopback: Failed RTM_NEWADDR: Operation not permitted
```

— in a `privileged: true` devcontainer, running as uid 1000. bubblewrap wants to
build a network namespace and cannot.

**It can be disabled**, and then everything works:

```sh
codex sandbox -c sandbox_mode=danger-full-access -- sh -c 'echo ok'   # ok
```

`SandboxMode` has three values (see `config-schema.json`), plus a
`features.use_linux_sandbox_bwrap` flag.

**Consequence for §1 and §7.** This is the predicted case, confirmed: cxz
**disables Codex's own sandbox** and relies on the container, which is the
design's premise anyway. Note the failure mode — a *panic*, not a graceful
degradation — so this is not optional configuration; a Codex session without it
is simply broken. It also reinforces §1: relaxing an agent's own sandbox is only
defensible because cxz defined the container it runs in.

### C2 — app-server is a daemon, not just a stdio process ⚠️ reshaped

```
codex app-server daemon    manage the local app-server daemon
codex app-server proxy     proxy stdio bytes to the running control socket
codex agents               browse all agent sessions on the shared local daemon
codex remote-control       manage the app-server daemon with remote control enabled
```

Codex already has **a daemon, a control socket, multiple sessions under it, and
a remote-control concept** — substantially the same shape as cxz's supervisor
(§3.1) and scoped control API.

**This is the largest open design question left.** Either cxz's supervisor wraps
Codex's daemon (one process per container hosting many sessions, cxz proxying
its socket) or it bypasses it (one app-server per session, stdio). The first
matches Codex's grain and may give session multiplexing for free; the second is
uniform with however Claude Code ends up working. **Needs a decision, and it
should be informed by C2-remaining below.**

### C3 / C6 — the server→client surface ✅ (shape; behaviour still unverified)

`codex app-server generate-json-schema --out <DIR>` emits 39 schemas — no
account needed, and it is the authoritative answer rather than documentation.

Server-originated requests the client **must** answer:

| Method | Purpose |
|---|---|
| `item/commandExecution/requestApproval` | run a command |
| `item/fileChange/requestApproval` | modify a file |
| `item/permissions/requestApproval` | escalate permissions |
| `item/tool/call` | invoke a client-side tool |
| `item/tool/requestUserInput` | **free-form question to the user** |
| `mcpServer/elicitation/request` | structured input for an MCP server |
| `account/chatgptAuthTokens/refresh` | **ask the client for a fresh token** |
| `attestation/generate` | attestation |
| `openai/form` | form input |

`ClientRequest.json` (199 KB) carries the whole client→server surface.

**Consequence for §3.3 and §5.1.** The bidirectional, correlated request model
is confirmed, and it is richer than "approvals": `item/tool/requestUserInput`
means a mid-turn free-form question is first-class, exactly as §5.1 assumed.

### C5 — Codex solves the central-login problem that Claude Code did not ✅ 🔑

Two findings that together change §4.2 for this agent:

**1. `account/chatgptAuthTokens/refresh` is a server→client request.** On a 401,
the app-server asks *the client* for a token:

```
params:   { reason: "unauthorized", previousAccountId: string|null }
response: { accessToken: string, chatgptAccountId: string, chatgptPlanType: string|null }
```

**2. Login accepts credentials on stdin, non-interactively:**

```sh
printenv OPENAI_API_KEY     | codex login --with-api-key
printenv CODEX_ACCESS_TOKEN | codex login --with-access-token
```

**Consequence for §4.2.** For Codex, the daemon can hold the account centrally
and hand each container a short-lived **access token** — no credential file
copies (so no rotation collisions, the §4.2 (b) failure) and no base-URL change
(so no product degradation, the §4.2 (a) failure). Codex supports as a
first-class protocol method exactly what had to be abandoned for Claude Code.

This is the strongest evidence yet for §3.4's optional-capability design:
`AuthBackend` is not hypothetical scaffolding. One agent has it and the other
does not, and the difference is visible to the user as "log in once" versus "log
in per project".

### C9 — Config ✅ (partially)

- `CODEX_HOME` relocates the config home, **but refuses to operate under a
  temporary directory**: `Refusing to create helper binaries under temporary dir
  "/tmp"`. cxz must place it on real storage — which it would anyway (§4).
- Codex **generates helper binaries inside the config home**
  (`$CODEX_HOME/tmp/arg0/codex-arg0XXXXXX`). Regenerable, so `BackupSkip` in
  §4's allowlist — and a reminder that the allowlist is the right default.
- Config is TOML; `config-schema.json` is the reference for keys.
- `codex doctor` reports auth, network, disk, install method, PATH and search
  provider without an account. A good provisioning self-check for cxz to run.

---

### C2-remaining / C3 / C6 / C7 / C11 — driven end to end ✅

A probe client (`initialize` → `thread/start` → `turn/start`) was run against
`codex-app-server` over **newline-delimited JSON-RPC on stdio**. It works with no
daemon, no installer layout, and no socket. Full transcript kept as the adapter's
first fixture.

**The daemon path is usable — the layout is trivial to satisfy.** An initial
probe read the refusal

```
Error: managed standalone Codex install not found at ~/.codex/packages/standalone/current/codex
```

as a blocker. It is not. `CODEX_HOME` is cxz's to place, and the expected layout
is just the package tree under `packages/standalone/<version>/` with **`current`
as a symlink** — the same atomic-swap pattern cld already uses for agent
binaries. Satisfying it and starting the daemon works first try:

```json
{"status":"started","backend":"pid","pid":25999,
 "managedCodexPath":"~/.codex/packages/standalone/current/bin/codex",
 "managedCodexVersion":"0.154.0",
 "socketPath":"~/.codex/app-server-control/app-server-control.sock",
 "cliVersion":"0.154.0","appServerVersion":"0.154.0"}
```

Machine-readable, and it reports the socket and both versions. Because `current`
is a symlink cxz owns, version pinning stays under cxz's control — which
disposes of the concern that a self-updating daemon would move the version out
from under `release.Manager`.

**But the daemon wrapper turns out to be unnecessary**, because the transport is
exposed directly:

```
codex-app-server --listen <URL>
  stdio://        (default)
  unix://         default control-socket path
  unix://PATH     an explicit socket
  ws://IP:PORT    a WebSocket listener
  off
```

The managed daemon is literally `codex app-server --listen unix://` under
supervision. Both were verified: `unix://<abs path>` creates a 0600 socket in
about two seconds, and `ws://127.0.0.1:8899` opens a TCP listener.

**Transport options, and where each lands for cxz:**

| Transport | Status | Fit |
|---|---|---|
| `stdio://` | **proven end to end** — init, thread, turn, approval, completion | The MVP. Supervisor owns the pipe, one process per session, exactly §3.1 |
| `unix://PATH` | listener confirmed; **framing not determined** — a direct newline-JSON client was disconnected at `initialize`, and `codex app-server proxy` exists precisely to bridge stdio to this socket, so the socket likely speaks a different framing | Worth revisiting if one container must host several Codex sessions in one process |
| `ws://IP:PORT` | listener confirmed, not driven | Interesting: the daemon shares a network with every container it creates (§1), so it could connect with **no relay at all**. See the security note below |
| `codex app-server daemon` | works once the layout exists | Adds update management cxz does not want and a second lifecycle to supervise; no benefit over running `--listen` directly |

**Security note on `ws://`.** The listener has no authentication. On loopback
that is fine; bound to an interface it would be reachable by anything sharing
the container's network — which in cxz's topology includes the daemon *and*
potentially other projects' containers. That collides with §7's no-lateral-
movement property, so a WebSocket transport requires either a per-project
network or a front door of cxz's own. Do not bind it broadly for convenience.

**Decision.** stdio for the MVP, because it is proven and matches the supervisor
design. `--listen` is a known, cheap upgrade path rather than a rewrite, and the
managed layout is documented here so that choosing it later costs nothing. The
question that would force the change is multi-session-per-container (§2, §10),
which is deferred.

**Unresolved:** the socket framing, and therefore whether one app-server process
multiplexes threads across clients. Drive it through `codex app-server proxy`
rather than connecting raw.

**Approval requests block — confirmed by measurement.** The request arrived at
t=9.55s together with

```json
{"method":"thread/status/changed","params":{"status":{"type":"active","activeFlags":["waitingOnApproval"]}}}
```

and nothing advanced during a deliberate 10-second delay. The turn resumed the
instant the reply was sent. §3.3's blocking assumption holds.

- Request: `item/commandExecution/requestApproval`, carrying `approvalId`,
  `command`, `cwd`, `reason`, `threadId`, `turnId`, `itemId`, plus policy
  amendment proposals.
- Decision enum: **`accept` | `acceptForSession` | `allow` | `cancel` |
  `decline` | `deny`** — *not* `approved`, which cost one round trip to learn.
- **A malformed reply is a rejection, not a hang**: an unknown variant produced
  `Rejected("approval request failed")` and the turn continued. Good — the
  failure mode is safe.

**Event model maps onto §3.3 almost directly:**

| Codex | cxz kind |
|---|---|
| `turn/started` / `turn/completed` | `turn_start` / `turn_end` |
| `item/started` + `item/completed`, `item.type` = `userMessage`, `agentMessage`, `reasoning`, `commandExecution` | `text`, `thinking`, `tool_call`, `tool_result` |
| `item/agentMessage/delta` | streaming `text` |
| `thread/tokenUsage/updated` | `usage` |
| `item/commandExecution/requestApproval` | `approval` (a **request**) |
| `thread/status/changed` | session activity (§5.2) |

Notes: every notification carries `emittedAtMs`, which is the envelope's `ts`;
`agentMessage` has a `phase` (`commentary` vs `final_answer`); `turn/interrupt`
and `turn/steer` exist (**C7**), as does `codex queue`.

**C11 answered:** `thread/tokenUsage/updated` gives per-turn
`total/input/cachedInput/cacheWrite/output/reasoning` tokens, and
`account/rateLimits/updated` pushes `usedPercent`, `windowDurationMins`,
`resetsAt` and credit balance — the equivalent of cld's `cld usage`, delivered
over the protocol instead of scraped.

**The protocol is far larger than an agent loop.** `ClientRequest` also carries
`fs/readFile|writeFile|readDirectory|watch|getMetadata|copy|remove`,
`command/exec` with `resize`/`write`/`terminate`, `account/login/start|cancel`,
`config/read|value/write`, `model/list`, `thread/list|read|items/list|turns/list`,
plus skills, plugins and marketplace. Two consequences: **agent login is
drivable from the web UI** (§5.1) without a terminal, and Codex supplies natively
much of what §5.3's supervisor file API does — which cxz still needs for
uniformity across agents.

### C4 — Storage: an append-only log with a SQLite projection ✅ 🔑

```
sessions/YYYY/MM/DD/rollout-<ISO8601>-<threadId>.jsonl    the record
thread_history_1.sqlite    thread_items, thread_turns, thread_realtime_items,
                           thread_history_projection_state
state_5.sqlite  queue_1.sqlite  logs_2.sqlite  goals_1.sqlite  memories_1.sqlite
config.toml  installation_id  models_cache.json  cache/  plugins/cache/
.sandbox_migration  tmp/arg0/…  thread-writer-locks/
```

**Codex independently arrived at cxz's §3.2 design**: an append-only rollout log
as the record, with SQLite as a *projection* over it (the table is literally
named `thread_history_projection_state`). That is a useful signal that
raw-as-truth is the right shape, not a cxz idiosyncrasy.

Two consequences for §4:

- **Paths are date- and thread-keyed, never workspace-keyed.** Claude Code's
  encoded-cwd problem — and the path rewriting cld needs on restore — simply
  does not exist for Codex.
- **The SQLite files are WAL-mode and live.** A file-level snapshot of a running
  container would capture a torn database. cxz's volume approach is unaffected,
  but the off-host backup (§4.1) must either back up only `sessions/` (the
  record, from which projections rebuild) or use a SQLite-aware backup. Copying
  `*.sqlite*` while Codex runs is the wrong answer.

For §4's allowlist: preserve `sessions/`, `config.toml`, `auth.json`; treat
`cache/`, `plugins/cache/`, `models_cache.json`, `tmp/`, `*-shm`, `*-wal`,
`*.lock`, `thread-writer-locks/` as regenerable.

### C5 — Credentials, confirmed ✅

`~/.codex/auth.json` (mode 0600) holds `auth_mode: "chatgpt"`, `last_refresh`,
and `tokens: { id_token, access_token, refresh_token, account_id }`.

**A rotating refresh token is present**, so copying this file between live
containers carries exactly the hazard that broke cld (§4.2 (b)). The protocol's
`account/chatgptAuthTokens/refresh` is the way around it: the daemon keeps
`auth.json`, and each container receives only an `access_token`.

---

## What is still open

- **C9-remaining.** What a first-ever run prompts for, and which `config.toml`
  keys suppress it. `config-schema.json` (shipped as a release asset) should
  answer this without trial.
- **C4-resume.** `thread/resume` and `codex resume --last` after recreating the
  container and after moving the workspace.
- **C8.** Image and file attachment: `UserInput` has a `text` and an `image`
  variant — confirm how the image is carried.
- **C12.** Whether a thread gets a generated name (`thread/name/set` exists, so
  probably client-supplied).
- Whether `codex login --with-access-token` accepts a daemon-minted token and
  keeps the session first-party — the practical test of the §4.2 strategy.
- The `unix://` socket framing, via `codex app-server proxy`, and whether one
  app-server process serves threads to several clients. Only matters if a
  container ever hosts multiple Codex sessions.
