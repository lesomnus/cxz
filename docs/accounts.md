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

## Backends and bindings

`Account.auth_backend` chooses the strategy — how login happens, what its scope is,
and who refreshes it. `Session.auth_binding` records the concrete association a
session actually uses, fixed at creation.

Existing accounts keep the backend they were registered with; changing the default
does not migrate them.

## Account and agent are fixed

The account determines the agent, so an explicit `--agent` that disagrees is
rejected. Both are fixed for the life of a session; resume and container recreation
preserve them.

Scripts must pass `--account` when creating a session. Interactive `session new`
and the TUI's `n` offer a picker instead.

## GitHub credentials

`cxz github sync` copies the host's current `gh` credentials into every running
project. Useful when you have added a scope — for instance `workflow` — and do not
want to re-authenticate in each session, since GitHub caps how many tokens an
account may hold.

## The accounts view

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
