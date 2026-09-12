# cxz architecture

cxz runs coding agents — Claude Code, Codex CLI, and others — inside per-project
containers, keeps every session alive independently of any client, and renders
each conversation in a web UI that understands what the agent is actually doing.

It succeeds [cld](https://github.com/lesomnus/cld), which does the same for
Claude Code alone, through tmux and a terminal. This document records the
decisions that differ from cld and the reasoning behind them, so that a later
change knows what it is overturning.

Status: architecture with a TUI-first implementation in progress (2026-09-11).
Owned devcontainer management and Claude/Codex adapters are implemented; see
[cld-parity acceptance](plans/cld-parity.md) and [verification progress](progress.md).
The web/IDE/authentication portions remain design work. Sections marked **Open**
are genuinely undecided. The user's current scope brings Codex into the TUI phase.

## Goals

- **Multiple agent vendors.** Claude Code first, Codex CLI second. A third
  vendor should cost an adapter, not a refactor.
- **A conversation UI, not a terminal.** Tool calls, diffs, and turn boundaries
  are structured data the UI renders, not ANSI bytes it replays.
- **Many sessions at once, legible at a glance.** Which sessions are working,
  which are waiting for input, which finished — without opening any of them.
- **Sessions outlive their clients.** Close the browser, restart the daemon,
  reboot the laptop: the conversation is still there and still running.
- **devcontainer compatible.** A project's `devcontainer.json` is the source of
  truth for its container. cxz does not invent a competing format.
- **A real IDE in the browser.** Reading code the agent wrote requires
  go-to-definition and semantic navigation, and testing it requires running it.
  A viewer is not a substitute for either. See §5.5.
- **The host is disposable.** The operator rebuilds their machine often. Every
  toolchain lives in a container, and everything cxz needs must be
  reconstructible from a git repository plus one config — or explicitly
  preserved off the host. See §4.

## Non-goals

- Being a general remote-development platform. cxz runs agents; the IDE is
  borrowed, not built.
- Multi-tenancy. One operator, their own machine, their own projects. Access
  control exists to protect that operator from the network and from the code
  the agents run — not to separate users from each other.
- Reimplementing the devcontainer specification. Container creation delegates
  to the official CLI, as cld does.

## 1. cxz owns its containers

The single most important structural decision: **cxz creates the containers it
manages, and manages only the containers it created.** cld did not, and most of
cld's complexity is the price of that.

Every one of these mechanisms in cld exists solely because it attaches to a
container someone else created — which means cxz never has to build any of
them:

| cld mechanism | Exists because | In cxz |
|---|---|---|
| `internal/syncer` — `docker cp` snapshot/restore of agent state, with workspace-path rewriting | A running container cannot be given a new mount | A named volume. The package disappears |
| `claude.EncodeProjectPath` — mirroring Claude Code's private transcript directory naming | State must be copied out and put back where the agent will look for it | Never needed; the state never leaves |
| `internal/agentx` — multiplexing relay over `docker exec` stdio | A running container cannot be given a network | Shared network. Plain HTTP |
| `scoped_api` + hijacked pty attach | The container has no route back to the daemon | Still scoped, but over a normal transport |
| Docker event reactor + `devcontainer.local_folder` label detection | Creation happens elsewhere | Creation is a local function call |

Those are also where cld's bugs and fragility concentrate — its own source calls
the transcript-path encoding "inherently brittle across its versions."

cxz creates a container by invoking the official `devcontainer up` against the
project's `devcontainer.json`, adding what it needs **at creation time**: a
volume for agent state, membership of the daemon's network, and its own
supervisor binary. None of those can be added afterwards, which is the whole
reason ownership matters.

### Foreign containers

cxz still watches Docker events, because it must know when a project's container
is already running outside its control — otherwise `cxz up` would race it or
create a second one for the same workspace. But detection is all that happens.

Such a project is listed with its state shown as running outside cxz, and one
action offered: **recreate with cxz**, which re-runs `devcontainer up` against
the same `devcontainer.json` and replaces the container with one cxz owns.

The UI must state what that costs before doing it:

- Anything installed into the running container at runtime and not present in
  the image is lost. The workspace is a bind mount and survives; named volumes
  survive.
- An editor attached to that container disconnects.
- It is a recreate, not a rebuild — with the image cached it is quick.

A project can also be excluded from cxz entirely, by label or by a workspace
glob, in which case it is not listed at all.

### Why not adopt a foreign container

It is possible; cld does it. But it costs a second implementation of every
stateful mechanism — state capture, transport, binary injection — and it can
never be complete, because a running container cannot be given a mount or a
port. The result is a product with two behaviours that users cannot predict
from the outside, and a permanent tax on every feature that touches container
state.

Recreating is one code path, and a devcontainer is meant to be disposable.
"cxz needs to own the container" is a rule that fits in a person's head.

### Capabilities still vary — by agent, not by container

Every managed container is equivalent, so there is no capability matrix over
container modes. Capability differences remain between *agents*: whether one
offers a usable protocol mode, reports subscription usage, or supports a shared
login. Those travel on the session as a runtime value, and the UI renders an
unavailable capability as absent — never as empty or zero. (See
"measured-vs-zero" in the appendix.)

## 2. Sessions are first class

In cld a container and a session are the same thing: one tmux session named
`cld-<container>`, addressed by container identity, with the scoped API granting
access by container id.

That does not survive multiple vendors — the point of supporting Codex and
Claude Code is largely to run both against the same repository.

```
Project      the devcontainer.json and the workspace it names
  Container  the container cxz created for that project
    Session  one agent process, one conversation, one event log
```

**The project is the unit users see.** A project's container may be absent,
running under cxz, or running outside it (§1) — all three are states of one row,
not separate listings.

- A session's identity is its own, not its container's.
- A container may host several sessions, of different agents.
- Access control still binds to the *container* — a session may only be reached
  by a client that is entitled to that container — but the *target* of every
  operation is a session id.

**Open:** whether two sessions of the same project may run concurrently against
the same working tree, or whether cxz gives each its own worktree. Concurrent
agents editing one tree is a real hazard; git worktrees are the obvious answer
and a large amount of machinery.

## 3. Agent integration

### Protocol first, pty second

Agent CLIs offer two interfaces: an interactive terminal UI, and a headless
streaming one (`claude -p --output-format stream-json`, `codex exec`, and
equivalents).

cld drives the terminal, and pays for it everywhere: it parses transcript JSONL
out of band to learn the conversation's title, and injects lifecycle hooks that
shell out to report whether the agent is working or waiting — because a pty tells
you nothing but bytes.

cxz drives the **protocol** interface. Tool calls, turn boundaries, token counts,
and errors arrive as data. Everything cld reconstructs by scraping is simply
delivered.

The pty is kept as a **second-class transport**, not a second architecture:

- some vendors will not have a usable protocol mode
- interactive approval flows may be far easier to hand to the real TUI
- `cxz attach` from a terminal should still feel native

Both are the same pipeline with a different payload type (§3.3).

### 3.1 The supervisor

The naive protocol implementation is for the daemon to `docker exec` the agent
and read its stdout. That binds the session's life to the daemon's: a daemon
restart kills every conversation.

Instead, a small cxz binary runs **inside** the container and owns the agent
process. It is **bidirectional**: it carries output out and input back in.

```
container
  supervisor ──spawn──▶ agent (headless, structured stream)
       │  ▲
       │  └──input──── prompts, approvals, choices, interrupts
       └──append──▶ event log  (on the state volume)

daemon
  tail(event log) ──▶ normalize ──▶ event bus ──▶ SSE ──▶ browser
  input           ◀── control API ◀────────────────────── browser
```

This buys four things at once:

1. **Daemon restarts are invisible.** The agent keeps running; the daemon
   re-tails from the last offset.
2. **Replay.** A browser tab opened an hour late reads the log from the start.
   No separate "history" path.
3. **Multiple clients.** Laptop and phone are both just readers of one log.
4. **Backup is the log.** There is no state to reconstruct — see §4.

Injecting a binary into the container is a mechanism cld already proves out
(checksum-verified copy, atomic symlink swap), and it is the same mechanism used
for the agent binary itself.

The input direction is not an afterthought: **prompts, tool approvals, mid-turn
choices, interrupts, and interactive login all ride it.** A supervisor that only
streamed output would push every one of those back to a terminal, which is the
thing this design exists to avoid. An input carries the session and, where the
agent demands it, the id of the request being answered, so a late or duplicate
reply is rejected rather than misapplied.

### 3.2 Raw is the truth

Vendor stream schemas change without notice. A store that keeps only cxz's
normalized form loses information it cannot re-derive.

The event log is **append-only and stores the vendor's own bytes**. The
normalized view is a projection over it, rebuildable at any time. When an
adapter improves, old conversations gain the improvement retroactively.

Codex reached the same design independently: it writes
`sessions/YYYY/MM/DD/rollout-<ts>-<threadId>.jsonl` as the record and keeps a
SQLite database over it whose state table is named
`thread_history_projection_state`. Useful corroboration that this is the shape
the problem wants, rather than a cxz idiosyncrasy.

### 3.3 The event envelope

Normalize the envelope, which can be pinned down; keep the payload
half-open, which cannot.

```
Event {
  session   SessionID
  seq       uint64      // dense, per session — the log offset
  ts        time.Time
  turn      TurnID      // conversation turn this belongs to

  kind      Kind        // text | thinking | tool_call | tool_result
                        // | diff | usage | approval | error
                        // | turn_start | turn_end | pty
  payload   any         // structured, for a kind cxz understands
  raw       []byte      // always the vendor's own event
}
```

- **Some events are requests, not notifications.** An `approval` — and any
  vendor elicitation — carries a `request_id` and *blocks the agent until it is
  answered*. This is not a stylistic difference from a notification: an
  unanswered request stalls the session, and a reply to a stale one must be
  refused rather than misapplied. Codex's app-server makes this explicit
  (`serverRequest/approval`, `mcpServer/elicitation/request`), and any adapter
  must model it. **Measured**: an approval request halts the turn — the thread
  reports `activeFlags: ["waitingOnApproval"]` and nothing advances until the
  reply is sent. A malformed reply is treated as a rejection rather than
  hanging, which is the safe failure. Decisions are
  `accept | acceptForSession | allow | cancel | decline | deny`.
- A `kind` cxz does not recognize still reaches the UI, rendered as an opaque
  vendor event rather than dropped.
- `pty` is just another kind, carrying raw terminal bytes. That is the whole of
  the second-class pty path: same log, same replay, same multi-client behavior,
  and the browser hands it to xterm.js instead of the conversation renderer.

Deliberately **not** attempted: a lowest-common-denominator message schema
covering every vendor. Forcing two vendors into one shape misrepresents both.

### 3.4 The adapter seam

Following cld's style — declarative data plus small functions, with optional
capabilities discovered rather than stubbed.

```go
type Agent struct {
    Name    string   // "claude" | "codex"
    Display string
    Bin     string
    Glyph   string

    ConfigDir func(home string) string
    ConfigEnv string   // "CLAUDE_CONFIG_DIR" | "CODEX_HOME"

    Release  ReleaseSource  // where the binary comes from
    Seeder   Seeder         // onboarding / trust / settings
    Launcher Launcher       // argv, resume semantics

    Protocol ProtocolAdapter // nil → pty only
    Usage    UsageReporter   // nil → no subscription figures
    Auth     AuthBackend     // nil → per-container login only
}
```

#### What the two vendors actually look like

Verified 2026-09-11; both move quickly, so re-check before relying on detail.

| | Claude Code | Codex CLI |
|---|---|---|
| Config home | `CLAUDE_CONFIG_DIR` | `CODEX_HOME`, default `~/.codex` |
| Config format | JSON (`settings.json`, `.claude.json`) | **TOML** (`config.toml`), plus per-profile `<name>.config.toml` and trusted project-level `.codex/config.toml` layers |
| Protocol surface | headless stream (`-p --output-format stream-json`) | **`codex app-server`** — bidirectional JSON-RPC 2.0, and itself a daemon with a control socket and multiple sessions |
| Approvals | `--permission-prompt-tool stdio` → correlated `can_use_tool` control request; allow/deny verified | server→client requests: `item/commandExecution/requestApproval`, `item/fileChange/requestApproval`, `item/permissions/requestApproval`, `mcpServer/elicitation/request` |
| User questions | `AskUserQuestion` through `can_use_tool`, answers in `updatedInput`; verified | `item/tool/requestUserInput` — first-class |
| Central login | none; see §4.2 | **`account/chatgptAuthTokens/refresh`** — the server asks the *client* for an access token |
| Distribution | single binary per platform+libc | a **package tree** (`bin/`, `codex-resources/bwrap`, `codex-path/rg`, manifest); musl-only, `static-pie`, so libc-agnostic |
| Own sandbox | none | bubblewrap, and it **fails inside a container** — cxz disables it (§7) |
| Non-interactive | — | `codex exec --json` emits JSONL, but **cannot** answer approvals: an approval request fails the run unless policy pre-approves |

Two consequences:

- **Codex integrates through `app-server`, not `codex exec`.** `exec` is a
  batch runner: it has no way to answer an approval mid-session, so driving
  cxz's interaction model (§5.1) through it is impossible, not merely awkward.
  app-server is a bidirectional protocol built for exactly this kind of client.
- **The seam is request/response, not a stream plus a writer.** Claude Code's
  shape invites modelling the adapter as "read events, write prompts"; Codex
  proves that wrong, because the server originates requests the client must
  answer by id. `ProtocolAdapter` is therefore bidirectional and correlated from
  the start — a detail that is cheap now and structural later.

- **`ReleaseSource` installs a tree, not a file.** Codex ships as a package with
  a manifest, a bundled sandbox helper, and its own `rg`. cld's "copy one
  binary, verify, swap a symlink" does not transfer, and the package shape is
  also the only one with a published checksum — the bare binary carries a
  sigstore signature instead.

The TOML config also kills any shared seeding implementation: the JSON merge
used to seed Claude Code's onboarding and trust keys has no Codex counterpart,
which is why `Seeder` is per-agent rather than a shared helper.

**Transport.** Codex exposes one directly: `codex-app-server --listen` accepts
`stdio://` (default), `unix://PATH`, or `ws://IP:PORT`. Its own daemon
(`codex app-server daemon`) is that same binary under supervision, and wants an
installer-managed layout — which is cheap to satisfy (a package tree plus a
`current` symlink cxz owns, the atomic-swap pattern cld already uses) but adds
update management cxz does not want and a second lifecycle to supervise.

cxz drives `codex-app-server` **over newline-delimited JSON-RPC on stdio**, the
supervisor owning the process as designed (§3.1). Verified end to end
2026-09-11. The socket and WebSocket transports are a known upgrade path rather
than a rewrite: `ws://` is notable because the daemon already shares a network
with every container it created (§1), so it could connect with no relay at all —
but that listener has **no authentication**, so it would need a per-project
network or a front door of cxz's own before it could be used without breaking
§7. What would force the change is several sessions of one agent in one
container, which §10 defers.

Note also that Codex's protocol reaches well past an agent loop: `fs/*`,
`command/exec` with resize, `account/login/start`, `config/*`, `thread/list`.
Two of those matter here — **agent login is drivable from the UI** (§5.1), and
Codex natively provides much of §5.3's file API, which cxz still implements in
the supervisor for uniformity across agents.

Verification detail and what remains open are tracked in
[docs/plans/codex-verification.md](plans/codex-verification.md).

`ReleaseSource` is the one piece that varies most (Claude Code publishes a
channel API; Codex ships GitHub release tarballs; others are npm packages), but
the caching, offline fallback, and GC around it are vendor-neutral and carry over
from cld's `release.Manager` with the cache key extended to
`(agent, version, platform)`.

**Both agents now have a verified host approval path.** The initial Claude
probe omitted the SDK's `--permission-prompt-tool stdio` option. With that
option and explicit ask rules, Claude blocks until the host allows or denies
the tool. Questions, interrupts and same-session resume after process exit were
also verified. Claude remains first in §9; relaxed approvals are not required.
Detail and reproducible fixtures are in
[claude-code-verification.md](plans/claude-code-verification.md).

The envelope (§3.3) must still absorb different granularities. Claude emits
messages, optional partial chunks, `system/status` and terminal `result` events.
The tested CLI did not emit the `session_state_changed` event declared in the
newer SDK types, so pending requests and turn results supply fallback state.
An interrupted tool produces `is_error: true` and `terminal_reason: aborted_tools`;
the adapter must distinguish a requested interruption from an unexpected error.

**The seam is still not proven until an adapter actually ships.** These probes
establish what each vendor offers, not that one abstraction covers both.
Designing against a vendor you have only read about produces an abstraction
fitted to nothing; designing against two you have only probed is better, but not
the same as having built it.

## 4. State, persistence, and a disposable host

An owned container gets a named volume for the agent's config and state
directory. That single decision removes:

- copying state out on every change and back on every recreate
- predicting the vendor's private transcript directory naming
- rewriting workspace paths inside transcripts when a project moves
- the entire class of races between a live container and an older backup

Agents differ in what that volume holds, and the difference is instructive.
Claude Code keys transcripts by an encoded workspace path — the brittle mirroring
cld had to reimplement. Codex keys its rollout logs by **date and thread id**,
so the path-rewrite problem does not exist there at all. Neither shape can be
assumed; `BackupPolicy` is per-agent (§3.4) for this reason.

One hazard the volume does not remove: Codex keeps live **WAL-mode SQLite**
projections beside its logs. A file-level snapshot of a running container would
capture a torn database, so the off-host backup (§4.1) must copy the
append-only record and let projections rebuild — or use a SQLite-aware backup.
Copying `*.sqlite*` from under a running agent is wrong regardless of vendor.

Retained from cld, because they were right for reasons unrelated to the copying:

- **A cxz-owned config directory**, not the vendor default. Users bind-mount
  `~/.claude` into every container, and since every workspace sits at the same
  in-container path, that merges every project's history into one.
- **An allowlist for what is preserved**, never a denylist. A denylist silently
  adopts every new directory the vendor invents.
- **Per-project credential isolation.** See §4.2 — this one is load-bearing and
  was learned the hard way.

### 4.1 What must survive a host rebuild

The operating constraint (§Goals) is that the host is rebuilt often. Sorting
every piece of state by whether it can be rebuilt makes the problem small:

| State | After a host rebuild | |
|---|---|---|
| Container | recreated from `devcontainer.json` | fine |
| Toolchain | in the image | fine |
| Agent binary | re-downloaded | fine |
| Agent configuration | from dotfiles / user config in git | fine |
| **Agent credentials** | re-authentication required | must be preserved |
| **Event logs / conversation history** | not reconstructible | must be preserved |

Only two things need explicit treatment, and the design principle for both is
the same: **they are backed up off the host, encrypted, and restored by
bootstrap.** Everything else follows from a git repository and one config file.

Bootstrap after a rebuild should therefore be: install Docker, run one cxz
command, restore. Anything that cannot be recovered that way is a design defect,
not an inconvenience.

### 4.2 Agent credentials

**Implementation update (2026-09-12):** Account is now a global payday profile
resource, while each `(Project, Account)` keeps its own OAuth grant/config/HOME.
`cxz account login --project PROJECT ACCOUNT` prepares that project and performs
an independent login there. Session.account is immutable. This preserves option
(c) below; it does **not** distribute shared refresh tokens. See [Accounts](accounts.md).
The Codex central-token mechanism discussed below remains a capability research
result, not the implemented subscription login path.

Subscription logins use OAuth with **rotating refresh tokens**: each refresh
issues a new one and invalidates its predecessor. Three arrangements are
possible, and two of them have already been tried and rejected in cld:

| | Arrangement | Outcome |
|---|---|---|
| a | **Proxy.** The daemon holds the login; containers point at it as their API endpoint. | **Rejected — measured.** Pointing `ANTHROPIC_BASE_URL` at a non-first-party endpoint makes Claude Code treat the session as a different product: degraded UI, features disabled. |
| b | **Injection.** The daemon distributes copies of one credential set to every container. | **Rejected — measured.** Whichever container refreshes first invalidates every other copy, forcing re-authentication everywhere. This is inherent to rotation, not a bug. |
| c | **Per-container login**, each container owning and refreshing its own credentials. | **Adopted.** |

So (c) stands, and the friction it leaves is addressed without touching the
auth model:

- **Credentials persist per project, in the project's volume.** A container
  recreate does not cost a login; only a first-ever login does.
- **The credential store is backed up off the host** (§4.1), so a host rebuild
  does not cost one either. Rotation is only hazardous between *concurrently
  live* copies; restoring one project's credentials into the one container that
  project has (§2) is sequential, and safe.
- **Logging in never requires a terminal.** The OAuth flow is a URL to visit and
  a code to paste back, so the web UI renders it over the supervisor's input
  channel (§3.1) like any other interaction. This was the actual complaint; it
  is a UI problem, not an auth problem.

An API key (`ANTHROPIC_API_KEY`) or a cloud provider backend sidesteps rotation
entirely and is the vendors' own answer for multi-environment use — but it is
metered billing rather than a subscription. cxz documents it as an alternative
and does not assume it.

**Codex is different, and the difference is the agent's, not cxz's.** Its
app-server issues a server→client request — `account/chatgptAuthTokens/refresh`
— asking the client for a fresh access token when a call returns 401, and
`codex login --with-access-token` accepts one on stdin. So for Codex the daemon
*can* hold the account centrally and hand each container a short-lived token:
no credential copies to collide (the (b) failure) and no base-URL change (the
(a) failure). Verified 2026-09-11 against 0.154.0; see
[codex-verification.md](plans/codex-verification.md) §C5.

This is why §3.4 makes `AuthBackend` an optional capability rather than a shared
mechanism. One agent supports central login and the other does not, the gap is
not something cxz can paper over, and it is visible to the user as "log in once"
versus "log in per project" — so the UI must say which one a given agent is
(§1, capabilities).

**Open (§10):** whether an `apiKeyHelper`-style hook, supplying a
daemon-refreshed short-lived token *without* changing the base URL, avoids both
failure modes at once. It is untested, and unsupported territory either way.


## 5. Web UI

### 5.1 Conversation and interaction

The primary view. Rendered from normalized events: messages, collapsible tool
calls with their results, inline diffs for file edits, token and cost counters
per turn.

This is the reason for protocol-first. A terminal cannot fold a tool call, cannot
syntax-highlight a diff, cannot let you scroll a long result independently of the
conversation, and cannot be read on a phone.

**How much of this an agent supports varies**, and it is a capability, not a
detail. Both tested agents route approvals and user questions to the client.
Claude's OAuth URL initiation is verified, while completed browser login remains
to be tested. The UI shows capabilities actually established for the session's
version (§3.4).

**Interaction happens here too**, over the supervisor's input channel (§3.1):

- prompts, and files or images attached to them
- **tool approvals** — rendered as what is being asked, not as a terminal prompt
- mid-turn choices the agent puts to the user
- interrupting a running turn
- **agent login** (§4.2): a link to the vendor's authorization page and a field
  for the code that comes back

These are one mechanism, not five. That is the argument for protocol mode
carrying its cost: a reply channel exists, so anything the agent can ask can be
answered in the browser. Driving them by writing keystrokes into a terminal and
parsing the screen back — the alternative — is fragile in exactly the way that
breaks silently when a vendor adjusts its TUI.

### 5.2 Session list and notifications

The list — every session, its agent, its project, and its live state — is the
feature that makes multiple sessions usable. cld's own pain point was that
session state lived in terminal tab titles.

With protocol-first, `turn_start` / `turn_end` / `approval` are first-class, so
state is exact rather than inferred. Codex goes further and publishes the state
itself — `thread/status/changed` carries `active`, `idle`, and flags such as
`waitingOnApproval` — which is precisely the signal cld had to manufacture by
injecting lifecycle hooks into Claude Code's settings.

**Browser notifications on turn end and on approval requests are part of the MVP,
not a later polish.** Many concurrent sessions only work if you are paged rather
than watching. This is the single highest-value feature in the UI.

### 5.3 Workspace and files

Inline, alongside the conversation — for looking without leaving the session.
The deep case belongs to the IDE (§5.5); these are different jobs and both are
wanted.

- **Diff view**: the working tree against HEAD, and per-turn "what changed
  here", inline in the conversation.
- **File viewer and tree** over the container's workspace, so a path mentioned
  in a tool result opens in place.
- **Full-text search** across the workspace (ripgrep in the container). Without
  a language server this covers most of "where else is this used"; what it
  cannot do is why §5.5 exists.
- **Upload** into the workspace or a staging directory, then referenced in the
  next prompt — including pasted images, which the vendors accept as file paths.
- **Download** of anything the agent produced.

**Monaco** backs the viewer and the diff editor — one component, two entry
points: click a file in a tool result, or click a turn's changed-file count.
It is an editor component, not an IDE: syntax highlighting broadly, rich
language intelligence only for a few languages, no extensions.

**Read-only to begin with.** Editing here would race the agent over the same
tree, which needs change detection and a save-conflict story. **Open (§10):**
whether to allow edits when the session is not working, guarded by a hash check
on save.

The backend is the supervisor (§3.1), not a fresh `docker exec` per request: it
already runs in the container, already has a channel, and can watch for changes.
Its file API is confined to the workspace (§7).

### 5.4 Authentication

The daemon exposes a network port. cld deliberately did not, and stated "no TCP
listener and no token to leak" as a security property. cxz gives that up, and
what it exposes is not a dashboard — it is shell access to every project, with a
relayed ssh-agent. Compromise is equivalent to host compromise.

Therefore:

- **Loopback by default.** Remote access is an SSH tunnel or a private overlay
  network (Tailscale and similar). This covers nearly every real use and keeps
  the exposure honest.
- **A non-loopback bind without TLS is refused**, not warned about. cxz can
  generate a self-signed certificate.
- **Password (argon2id) to a server-side session, HttpOnly + Secure +
  SameSite=Strict cookie.** WebAuthn and OIDC are over-engineering at this size.
  Rate-limited, constant-time comparison.
- **Every WebSocket upgrade validates `Origin`.** WebSockets are not subject to
  the same-origin policy, so cookie authentication alone lets any website in the
  browser open a socket to cxz. This is the most likely way to get this design
  badly wrong.
- **Destructive fleet-wide operations are not exposed to the web UI at all.**
  They stay in the CLI.

### 5.5 The web IDE

**VS Code for the web is a first-class requirement, not an escape hatch.** Two
things depend on it and nothing else provides them:

- **Semantic navigation.** Reviewing agent-written code means go-to-definition,
  find-references, and type errors. Those come from a language server driven by
  a real extension host. A viewer (§5.3) cannot approximate it — the gap is
  categorical, not one of polish.
- **Running the code.** Trying a demo, driving a test, poking at a dev server:
  an integrated terminal in the project's own container, with the project's own
  toolchain.

**cxz does not build an IDE.** No editor functionality, no language support, no
extensions — VS Code is VS Code. cxz's job is to run it in the right place and
put a link to it in the conversation. What follows is hosting, not development.

This is not a roadmap item to be traded away. Architecture that forecloses it —
assuming a single origin, omitting a reverse proxy, leaving container ports
unreachable — is wrong by construction, so the preconditions are designed in
from the start even though the IDE itself ships last (§9).

#### What it actually takes

Required:

1. Put `code serve-web` in the container — the binary-injection mechanism the
   agent already uses.
2. Run it bound to container loopback.
3. Proxy it from the daemon onto its own origin.
4. Link to it from the session.

Refinements, not prerequisites: a shared server volume (disk only), extension
pre-installation (§5.5 Extensions), and the port proxy (a separate requirement
that happens to arrive with it).

#### One server per container, started on demand

The extension host must run where the toolchain is. gopls needs the project's Go
installation and module cache; those are in the container, so the server is in
the container. A single server on the daemon serving every mounted workspace
would give every project the daemon's toolchain — reintroducing precisely the
problem devcontainers exist to solve.

So it is one server per container, and the resource cost is managed in time
rather than in count: **start on first use, stop after idle.** Ten containers
then normally have zero or one server alive. This is what VS Code Desktop
already does when attaching to a container.

Splitting it the way vscode.dev does — one shared web client, many thin remote
backends — is not available. VS Code does separate the two (`remoteAuthority` is
how vscode.dev reaches a tunnel), but resolving an authority to a self-hosted
backend is not a supported extension point, and the official web client bundle
is proprietary while the MIT `code-oss` build lacks marketplace access. **Open
(§10).**

#### Self-hosted, with nothing in the middle

`code serve-web` serves the web client and the backend together, so proxying it
means cxz plays both roles that vscode.dev and the tunnel service play: the
client is served from cxz's own origin, and the daemon's authenticated proxy is
the only path to it. No third-party service, no per-container account login, no
traffic leaving the machine.

Microsoft's tunnels (`code tunnel`) exist to reach a machine behind NAT. cxz has
no such gap — the daemon shares a network with every container it created (§1).
The only reachability question is how one reaches the *daemon*, which is §5.4.
There is nothing in the middle to relay, so cxz does not implement or depend on
a tunnel service.

#### Origin isolation

An embedded VS Code runs extensions and webviews — arbitrary code — in the
browser. On the same origin as the control plane, one extension can read the
session cookie and drive the entire fleet. VS Code isolates its own webviews
onto separate origins for exactly this reason.

So the IDE is served from a **separate origin**, authenticated by a short-lived
token scoped to one session, never the control-plane cookie. It opens in its own
window, which suits a full-viewport IDE anyway.

This is the single constraint that must exist in the HTTP design from day one.
Retrofitting a second origin into an application built on the assumption of one
is expensive; reserving it costs nothing.

#### One authentication system, one copy of the server

- The IDE server runs with **its own auth disabled**, bound to container
  loopback, reachable only through the daemon's authenticated reverse proxy. Two
  credential systems for one product is a defect, not defense in depth.
- A VS Code server is hundreds of megabytes, so it is populated **once into a
  shared read-only volume** and mounted, never copied per container — the same
  reasoning as the agent binary cache, at a scale where it matters more.

#### Extensions

Semantic navigation is only as good as the language extension behind it, which
makes `customizations.vscode.extensions` load-bearing rather than cosmetic: a
project declaring `golang.go` is declaring how its code is meant to be read.
cxz installs that list (§8) with `--install-extension` before the server is
first served. Installs land in the container's own volume, so this is a
once-per-container cost, not a once-per-launch one.

This decides the implementation. **`code serve-web`** (the official VS Code CLI)
reaches the real extension marketplace; code-server and openvscode-server are
restricted to Open VSX by licensing, where language extensions lag or are absent.

One wrinkle, verified 2026-09-11: the CLI **fetches its own server on first
request** (into `~/.vscode/cli/serve-web`) rather than shipping it, and version
mismatches between the CLI and the server it pulls are a known complaint.
`code version use <stable | insiders | x.y.z | path>` exists for pinning. This
breaks cxz's "the container needs nothing preinstalled, cxz injects everything"
property unless the server is pre-warmed at provision time — which is also what
makes an offline or air-gapped container work. Treat the fetch as a provisioning
step cxz performs deliberately, not as a runtime surprise on first click.

#### Container ports

Running a demo implies reaching it. Because cxz owns the container and puts it
on the daemon's network (§1), the daemon can **proxy any container port without
publishing it to the host** — no creation-time port decisions, no host port
collisions between projects, and nothing exposed beyond the authenticated UI.
`forwardPorts` in `devcontainer.json` seeds the list; anything else is added on
demand.

Forwarded ports are proxied on their **own origin**, for the same reason the IDE
is: a dev server is untrusted code from the browser's point of view.

### 5.6 Opening the user's own VS Code

Distinct from §5.5 in the way that matters: **cxz hosts nothing here.** It
constructs a URL and renders a button. The editor is the one already installed
on the machine holding the browser.

This is worth having regardless of §5.5, and worth having *first*, because it is
a button rather than a subsystem — and because opening a local VS Code against
the container is what the operator does today anyway. The conversation should be
able to hand off to it.

- **Attach to the container.** The Dev Containers extension registers a URI
  handler: `vscode://vscode-remote/<remote-kind>+<hex>/<absolute path>`, where
  remote-kind is `dev-container` or `attached-container` and the hex payload
  identifies the container (for `dev-container`, the hex-encoded host path).
  Note the doubled slash before an absolute workspace path, so its leading `/`
  survives. cxz knows the container and the workspace path, so there is nothing
  to run and nothing to proxy. Same host only, which is the ordinary case.
  *(Shape confirmed 2026-09-11, but this is semi-documented, reverse-engineered
  territory — pin it against the installed extension rather than trusting a
  constant.)*
- **Attach over SSH.** `vscode://vscode-remote/ssh-remote+<host>/<path>`, with
  an sshd in the container reached through the daemon. This is what "our own
  relay" resolves to in practice: VS Code Desktop has no mode for connecting
  over an arbitrary authenticated HTTP proxy, so a self-hosted relay for the
  desktop client means exposing SSH. It belongs with remote exposure (§9), which
  is when the daemon becomes reachable from elsewhere anyway.

### 5.7 Delivery

The SPA is embedded into the binary (`go:embed`). cxz stays a single binary with
no runtime Node dependency; Node is a build-stage concern only.

Live updates use SSE from the event bus, not polling.

## 6. CLI

The CLI remains a first-class client, not a fallback:

- `cxz up [path]` — create/start a project's container and a session
- `cxz ls` — sessions, their agents, their states
- `cxz attach <session>` — the pty view of a session, in the terminal
- `cxz web` — serve the UI
- daemon lifecycle, config, and everything destructive

A terminal dashboard equivalent to cld's `cxz watch` is worthwhile once the
session model settles, with selection and attach from the list.

## 7. Security model

**The container is the trust boundary.** Agents execute untrusted repository
content; that is the premise of the whole design.

- **Session access is scoped.** Code running inside a container can reach only
  its own container's sessions — never enumerate, attach to, or destroy another
  project's. Identity is bound by the daemon at the transport, never supplied by
  the container. Fleet-wide destructive operations are absent from that surface
  entirely.
- **Nothing unsanitized crosses inward.** Shared agent settings are filtered of
  secret- and host-only keys before installation (environment blocks, credential
  helpers, MCP auto-trust flags). Host-only binaries referenced by config are
  stripped rather than shipped.
- **An agent's own sandbox is disabled inside the container.** Codex sandboxes
  itself with bubblewrap, which fails in a container (`bwrap: loopback: Failed
  RTM_NEWADDR: Operation not permitted`) and panics outright when its helper is
  missing — so this is a required setting, not a preference. cxz relies on the
  container instead, which is the premise of this section; the point is that
  doing so is only defensible because cxz defined that container (§1).
- **Approvals are answered in the browser (§5.1) where the agent supports it,
  and may be relaxed as a mode.** Both initial agents have verified host approval
  protocols (§3.4). Every managed container is one cxz created, whose isolation cxz
  defined, so a project may opt to run with permissions relaxed and rely on that
  isolation — worth it when the agent's work is routine and the prompts are
  friction. It is a per-project setting, not the mechanism the design leans on,
  and it is only defensible because cxz owns the container (§1).
- **The IDE and every forwarded port are separate origins** with session-scoped
  tokens (§5.5). They host untrusted code — extensions, webviews, a dev server's
  own pages — and must never share an origin with the control plane.
- **The workspace file API is confined to the workspace.** Paths are resolved,
  symlinks followed, and anything landing outside the workspace root is refused.
  Otherwise §5.3 becomes an arbitrary-read primitive into a container that also
  holds the agent's credentials.

Relaxed approvals are opt-in per project. Answering in the UI is the default,
because a design that cannot show what is being approved has not solved the
problem — it has hidden it.

## 8. devcontainer compatibility

`devcontainer.json` is the source of truth. cxz does not extend it with its own
keys; project-specific cxz settings live in cxz's own config, matched to
workspaces by glob (as cld does).

Owned containers are created by invoking the official `devcontainer up`, so
features, image builds, lifecycle hooks, and variable substitution behave exactly
as they do under VS Code. Reimplementing the specification is out of scope, and
detection depends on the labels the official CLI writes.

`customizations.vscode.extensions` and `settings` are honored by the web IDE
(§5.5) — the same list VS Code Dev Containers would install, and the mechanism
that makes semantic navigation work for a project's own language. `forwardPorts`
seeds the port proxy.

## 9. Roadmap

The order matters: each step is what makes the next one answerable.

**MVP**

| Area | In | Out |
|---|---|---|
| Containers | owned creation, foreign detection + recreate | — |
| IDE | deep link to a local VS Code; multi-origin HTTP + proxy designed in | the hosted IDE itself |
| Agents | Claude Code only, protocol mode | Codex; pty transport |
| UI | session list, conversation, inline viewer + diff | — |
| Interaction | approvals, choices, interrupts, agent login — all in the browser | — |
| Files | tree, viewer, search, upload | editing in the browser |
| Auth | per-project login, off-host credential backup | remote exposure, TLS |

**Then, in order**

1. **Codex adapter.** The first real test of §3.4. Expect the seam to change.
2. **pty transport** — second-class, same log — for vendors whose protocol mode
   is unusable, and for `cxz attach`.
3. **Remote exposure**: TLS, origin hardening, session management — and with it
   the SSH attach link (§5.6).
4. **Container port proxy**, for running what the agent wrote.
5. **Terminal dashboard.**
6. **Hosted web IDE** (§5.5) — the server, lazy start, extension installation.
   Last, because the deep link (§5.6) covers the same-machine case from the
   start, and that is the ordinary case.

The IDE shipping last does not make it optional (§Goals, §5.5); it makes it
*unblocking*. Its preconditions — the multi-origin HTTP design and the proxy —
land with the MVP, so step 6 is additive rather than a rewrite. Deferring the
work is fine; designing as though it will never come is not.

Reversing 1 and 3 is the tempting mistake: adding a second agent will change the
API that the UI consumes.

One vendor in the MVP is deliberate. Protocol mode, an approval channel, and an
in-browser login are each vendor-specific, and paying for them twice before
knowing the first one works is how the adapter seam (§3.4) gets designed against
an imagined vendor.

## 10. Open questions

Vendor-facing items were last checked on **2026-09-11**; both agent CLIs and the
VS Code CLI move quickly, so re-verify rather than trusting this list's age.

- Whether an `apiKeyHelper`-style token hook, without a base-URL change, gives
  one central login without the degradation of (a) or the rotation collisions of
  (b) (§4.2). Cheap to test, unsupported either way, and an optimization rather
  than something to build on.
- Codex session storage layout and what its rollout files contain — the one
  part of §3.4's table still unverified, and what decides whether resume and
  history restore work the way §4 assumes.
- Whether the event log lives on the state volume or in a daemon-side store.
  On the volume it survives daemon loss and keeps the container self-contained;
  daemon-side it is queryable across sessions without touching containers. §4.1
  requires it be backed up off the host either way.
- Whether pre-warming `code serve-web`'s self-downloaded server at provision
  time is reliable enough to keep containers offline-capable (§5.5). If not, the
  fallback is Open VSX via code-server, which weakens semantic navigation — the
  reason the IDE exists.
- Whether a self-hosted web client can resolve a `remoteAuthority` to a
  cxz-provided backend, which would allow one client bundle over many thin
  per-container servers (§5.5). Assumed unavailable; worth confirming, since it
  is the only path to sharing the client across projects.
- Whether the inline viewer allows edits when a session is idle, with a
  save-time hash check (§5.3).
- Concurrent sessions on one working tree, or a worktree per session (§2).
  Deferred: the motivation for cxz is environment fidelity, not parallelism, so
  one session per project container is sufficient until it demonstrably is not.
  Revisit only if concurrent agents on one repository become the normal way of
  working.

## Appendix: lessons carried from cld

Independent of architecture, and expensive to relearn:

- **Isolate the agent's config directory** from the vendor default. Shared
  mounts plus identical in-container workspace paths merge unrelated projects'
  histories.
- **Allowlist what is preserved, never denylist.** A denylist silently adopts
  every directory a vendor adds.
- **Isolate credentials per project, and never centralize a subscription login.**
  Both alternatives were measured in cld and both failed: distributing copies of
  one credential set makes the first refresh invalidate all the others, and
  proxying the API makes the vendor treat the session as a different product and
  withdraw features. See §4.2 before trying either again.
- **Sanitize configuration crossing into a container**: environment blocks,
  API-key and credential helpers, MCP auto-trust.
- **Never let session liveness depend on guessing vendor internals.** cld
  resumes with `--continue || exec <agent>` precisely so a resume the vendor
  rejects degrades to a fresh session instead of a dead pane.
- **Not measured is not zero.** A capability that is unavailable must render as
  absent. A fabricated zero teaches users to distrust every number shown.
