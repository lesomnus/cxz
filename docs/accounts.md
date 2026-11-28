# Accounts and authentication

An **account** is an agent authentication profile — a Claude or Codex login cxz
keeps for you. It is not a cxz user or tenant. cxz has no notion of users.

```sh
cxz account add codex personal
cxz account add --name "Company Codex" codex work
cxz account login work
cxz account ls
cxz backend ls          # which agent/auth combinations exist
cxz binding ls work     # which projects this account is bound to; metadata only
```

Registering an account does not log it in. The two are separate steps, and
`account ls` shows both.

Auxiliary AI tasks — summaries and next-message suggestions — authenticate
separately. See [auxiliary AI](auxiliary-ai.md).

## Sessions are isolated from each other

Each session gets its own agent configuration directory, HOME, cache and temporary
directory. Two sessions in the same repository — even on the same account — cannot
see each other's login or configuration.

cxz does not create worktrees and does not coordinate edits. Two agents in one
workspace will happily edit the same file.

**Refresh tokens are never copied between sessions or projects.** Host credentials
are never imported implicitly: if a session has no login, it fails rather than
falling back to `ANTHROPIC_API_KEY` or whatever is in your environment.

## The two login shapes

| | Claude | Codex |
|---|---|---|
| Default backend | `project-local-oauth` | `brokered-access-token` |
| Logs in | **per session** | once, centrally |
| Token lifetime | short; refreshed per session | long-lived |

**Codex** logs in once per account. The manager then supplies access tokens to the
projects that account is bound to. Central OAuth login and refresh are delegated to
the official Codex binary; cxz implements no OAuth endpoints of its own. Codex can
also be set to `project-local-oauth` explicitly.

**Claude** keeps an independent login for every session, including sessions sharing
one account, because the account has no limit on how many tokens it issues.
Creating a session interactively opens the login flow. To log in again later, stop
that one session and:

```sh
cxz account login --session SESSION_ALIAS ACCOUNT
```

API keys are not supported.

## An alias is a label; the pin is what makes it mean an account

cxz keeps no list of which provider account an alias stands for. A project-local
OAuth login goes to the provider's own consent page, and when the browser is
already signed in to an account that page approves it without offering a choice.
So choosing a different alias does not choose a different account: the login can
land on the same one, and the session then spends the very limit it was moved
away from.

`Account.login_identity` closes that. The first login writes down which provider
account it reached, as an opaque token — no address, organization or subject, so
it reveals nothing the rest of the resource does not. Every later login for that
account has to reach the same one, or it is refused while its credential is still
in staging, leaving the account exactly as it was.

Changing accounts on purpose is therefore two steps, and the first one is saying
so:

```sh
cxz account unpin ACCOUNT
cxz account login --session SESSION_ALIAS ACCOUNT
```

The web says the same thing as a **Provider account** row on the account page,
with an Unpin button beside it.

Two cases are deliberately not refused. A brokered Codex account pins its subject
centrally instead, under its own lock, so this field stays empty for it. And a
Claude credential that carries no `accountUuid` — older releases wrote none, and
a refresh has been seen to leave it out — means *cannot tell* rather than
*mismatch*: the login is accepted and the pin is left alone, because the
alternative is that a provider change stops logins from working.

What the pin covers is the session login a person runs: the account it reaches
and the account the resource says. The auxiliary helper profile and the internal
`_account-login`/`_account-import` runtime commands write credentials without it,
the same way editing a credential file by hand does. They need a shell inside the
project container, so they are an escape hatch rather than a way to make this
mistake by accident.

## Backends and bindings

`Account.auth_backend` chooses the strategy — how login happens, what its scope is,
and who refreshes it. `Session.auth_binding` records the concrete association a
session actually uses. It is set at creation and changes only together with
the account, when the session's account is switched.

Existing accounts keep the backend they were registered with; changing the default
does not migrate them.

## Account and agent are fixed

The account determines the agent, so an explicit `--agent` that disagrees is
rejected. The agent is fixed for the life of a session; resume and container
recreation preserve both.

### Changing a session's account

`SessionService.SwitchAccount(ref, account)` rebinds a stopped session to another
account **of the same agent**. In the web UI it is **Change account** in the
session menu; a running session is stopped first, after confirmation.

The conversation stays with the session: its journal, settings and provider
history. Inside the session's own profile, the provider history folder —
`projects/` for Claude, `sessions/` for Codex, the same folder a fork copies — moves
from the old account's configuration directory to the new one, so the agent
resumes the same native conversation. Logins never move: a Claude session asks
for the new account's login on its next start, exactly as a new fork does. Other
per-account files stay behind in the old account's directory.

The switch is refused while the session runs, has pending questions or queued
input, or when the new account's directory already holds provider history for
this session. Retrying an interrupted switch completes it. The session's
`account` and `auth_binding` edges change together and only through this call;
`Patch` and `Apply` still cannot change them. Usage is recorded with the account
the session had when it was produced: turns before the switch stay with the old
account and later ones count toward the new one.

Forking into another account is a fork followed by a switch of the child.

Scripts must pass `--account` when creating a session. Interactive `session new`
and the TUI's `n` offer a picker instead.

## GitHub credentials

`cxz github sync` copies the host's current `gh` credentials into every running
project. Useful when you have added a scope — for instance `workflow` — and do not
want to re-authenticate in each session, since GitHub caps how many tokens an
account may hold.

## The accounts view

In the web UI, **Accounts** shows account cards with metadata, authentication,
recorded usage and listed sessions. The top add button opens a modal form.
Information and sessions share the left column; authentication sits above usage
on the right, with a square Erase action on its left.
The metadata's **Display name** and **Alias** values can be clicked to edit them.
Saving uses `AccountService.Patch` with the account UUID and its last-read
`date_updated`; conflicting edits are refused rather than overwritten. Agent
and authentication backend stay fixed, and general `AccountService.Apply` writes
remain closed.

An alias is a public handle, not the authentication directory's identity. Renaming
it preserves the account UUID, existing sessions, login, grants, bindings and
auxiliary AI assignments. The Manager keeps the original private profile key in
`account_runtime_identity`, which is created and backfilled automatically on
startup. Reusing an old alias for a new account gives it a separate profile and
UUID. This editing support requires the updated Manager; no database reset,
session restart or project recreation is needed.

Usage is the account's share of [managed telemetry](managed-telemetry.md): what
its sessions reported while they used it. It does not represent provider billing
or remaining quota, and it is unavailable while the telemetry store is not.

`AccountService.Status(AccountRef)` describes local authentication readiness:
saved credentials present, login required, authentication busy, or a
session-scoped login. The response contains no credentials and does not claim
provider-side verification. Central Codex profiles enable **Log in** only when
needed. Project-local OAuth profiles have independent session credentials and
cannot truthfully report a single global logged-in flag.

`AccountService.Logout(AccountRef)` removes the central saved credential and
revokes its project and auxiliary token-supply capabilities. It preserves the
account's immutable identity pin and other profiles. Already-issued provider
access tokens can remain usable until their expiry; the operation does not
revoke a grant at the provider. Session-local login removal is not supported by
this account-scoped operation. The web UI confirms logout before executing it.

`AccountService.Erase(AccountRef)` permanently erases the account and every linked
session, including stopped sessions and sessions in other projects. Each session
uses the same runtime purge as `SessionService.Erase`: its conversations, owned
memory, attachments, agent profile, derived usage and auxiliary results are removed.
Account authentication, token-supply grants, auxiliary authentication volumes,
AI task profile assignments, bindings and registry rows are removed as well.
The account alias can be registered again. Project workspaces and shared resources
remain; files written in the workspace, bytes referenced by other sessions and
shared operational logs are outside session-owned cleanup.

The web UI explains the cascade in a confirmation modal. Cancellation performs no
erase. If cleanup fails, the account remains available to retry; completed session
erasures are not rolled back. The server serializes the cascade against new
session creation and authentication. This operation requires an updated Manager.

These RPCs and the account filter on `SessionService.ConversationStats` require
an updated Manager. Older servers show status as unavailable rather than guessing
that login is required or presenting all-account usage as one account's usage.

### TUI

`a` in the project list opens accounts without leaving the TUI; `n` for a new
session opens the same view in selection mode, and an empty list offers to create
an account.

- `n` adds an account: provider (`←`/`→`), alias, optional display name, then
  **Create account**.
- `l` runs the login workflow and returns — Claude against the current project,
  Codex centrally by default.
- `/` searches by alias, name, provider or number.
- `Ctrl+R` restores a deleted session.

Creating a Claude session prompts for login inline when it needs one, including
over SSH and TCP frontends. The project runtime runs the provider's own login and
creation is retried once with the same session profile.

## Host Git configuration

On the Linux Manager host, `cxz gitconfig sync` refreshes global Git settings in
all running project containers. No agent restart or project recreation is needed:
subsequent Git commands, including commands from isolated session HOMEs, read the
new settings. Stopped projects receive the saved snapshot at their next `up`.
`cxz up` also snapshots host settings automatically.

The snapshot reads `$XDG_CONFIG_HOME/git/config` (default `~/.config/git/config`)
and `~/.gitconfig` in Git's order, or `$GIT_CONFIG_GLOBAL` when set. Referenced
`include.path` and `includeIf.*.path` files are copied recursively, with include
paths rewritten to container copies. Conditions remain conditions: host-specific
`gitdir:` patterns may need adapting to container paths. Other path-valued options
(signing keys, hooks, executables and credential helpers) are copied as settings;
the referenced files/programs are not installed by this command.

Each project keeps an immutable snapshot in `/cxz/state/data/host-git`, selected
by an atomic `active.config` include from `/etc/gitconfig`. Unchanged snapshots
reuse their generation. Existing container system settings are preserved;
container global and repository-local settings can still override these defaults.
Earlier generations remain in project state so concurrent Git reads can finish.
This copies configuration, not the host Git credential store; `cxz github sync`
remains the separate command for GitHub CLI credentials.

Run the command on the remote Linux host when using a Windows SSH frontend. A
project that fails to sync is reported, while other running projects are still
attempted. No successful all-project message is printed on partial failure.
