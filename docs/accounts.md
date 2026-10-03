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
