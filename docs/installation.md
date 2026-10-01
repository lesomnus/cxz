# Installation

## What gets installed

```sh
cxz install --workspace-root /absolute/path/holding/your/projects
```

This builds a manager image, starts it as a background container, waits for its
API and returns. The manager owns everything cxz creates: project containers,
state volumes, the shared Docker engine and an ownership identity that marks them
all as belonging to this installation.

Your host needs Docker and the `cxz` binary. It does not need Node, the
devcontainer CLI, Claude Code or Codex — those live in containers cxz manages.

`cxz manager serve` runs the same server in the foreground. That is for developing
cxz itself, not for installing it.

## The workspace root

Every project must resolve to a path under `--workspace-root`. Bind mounts are
resolved by the **Docker engine**, which is not always the machine you typed the
command on — with Docker Desktop or a remote engine it is a different filesystem.
Paths outside the root fail rather than silently mounting the wrong thing.

Reinstalling with a different root does not move existing projects.

## Client state

The client keeps a small state directory: a locator pointing at the installation,
plus your own settings and recordings. It does not hold conversations.

Default: `$XDG_STATE_HOME/cxz`, else `~/.local/state/cxz`. Override with
`--state`, before the subcommand:

```sh
cxz --state /private/client-state project up .
```

Use the same state directory for every command against one installation.

## Devcontainer configuration

`cxz up .` reads the directory's `devcontainer.json` and delegates images,
Dockerfiles, Compose, features and lifecycle hooks to the official devcontainer
CLI. It does not reuse a container VS Code already started.

- **No configuration**: you get the installation's
  [default devcontainer](docker.md#the-default-devcontainer) — your own template
  if you wrote one, otherwise the built-in one.
- **Several configurations**: interactive runs ask; scripts pass `--config`.
- **Changed configuration**: existing containers need recreating before it applies.

The remote user needs write permission on the workspace. cxz respects an explicit
`remoteUser` and will not quietly rewrite file ownership or remap an existing
user's UID — if your host UID is not the image's usual 1000, adjust the
devcontainer.

See [containers and Docker](docker.md) for the shared Compose override and extra
host mounts.

## Remote Docker engines

The installer accepts a Unix socket or a reachable non-TLS TCP engine. SSH and TLS
engine endpoints are not implemented.

**Never expose Docker's unauthenticated API to a network you do not trust.** An
engine endpoint is root on that machine.

## Configuration trust

Devcontainer configuration can run host-side initialization and ask for elevated
settings. When startup needs that trust, cxz stops and offers an inline
**Exit / Trust** choice; `--trust-config` answers it in advance.

Trusting a configuration means you have read it. It is not a sandbox — cxz is not
protecting you from a hostile `initializeCommand`.

`cxz up -x` (or `--exit-on-error`) returns errors instead of prompting.
Non-interactive and `--format json` commands never prompt.

## Verifying an installation

```sh
cxz manager doctor      # installation health
cxz project logs web    # provisioning output for one project
cxz version
```

## Removing it

```sh
cxz uninstall           # removes the manager; projects, volumes and history stay
cxz purge --dry-run     # lists everything deletable
```

`uninstall` is safe. `purge` is not — see [projects and sessions](projects.md#purge).
