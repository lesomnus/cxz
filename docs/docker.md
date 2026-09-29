# Containers and Docker

## What cxz creates

| | |
|---|---|
| **Manager container** | The installation's server. One per installation. |
| **Project container** | One devcontainer per project, built from its configuration. |
| **Shared Docker engine** | A Docker-in-Docker container projects use for their own Docker work. |
| **State volumes** | Named volumes holding journals, manifests and agent configuration. |
| **Tools volume** | Checksum-verified agent and helper binaries, mounted read-only. |

Everything carries an ownership label. cxz detects containers it does not own and
**never adopts them** — a devcontainer VS Code started is not a cxz project.

## The default devcontainer

A directory with no `devcontainer.json` gets a plain Debian devcontainer with a
non-root `vscode` user. It is deliberately boring: enough to run an agent, nothing
opinionated about your toolchain.

Add a `devcontainer.json` and cxz uses it instead, delegating images, Dockerfiles,
Compose, features and lifecycle hooks to the official devcontainer CLI. Changes
need a `cxz project recreate`.

## The shared Compose override

```sh
cxz edit docker-compose
```

This edits `docker-compose.yaml` beside `settings.jsonc` and applies it to projects
in the installation — the place for host mounts and services you want everywhere
without editing each repository's devcontainer.

- `${HOME}` expands on the Linux host.
- `${DEVCONTAINER_SERVICE}` names the project's development service.
- Compose `include` pulls in further files.
- Set `devcontainer.compose` only to point at a different path.

Existing containers need recreating.

## The shared Docker engine

On by default, so an agent can build and run containers without reaching your host
daemon. It keeps its own images, build cache and volumes.

```sh
cxz docker status
cxz docker up
cxz docker down
```

`Ctrl+.` in the TUI shows status, start/stop and unused build-cache cleanup.
Cleanup removes build cache only — images and volumes are kept.

An explicit `docker.mode: "off"` in settings is respected and not re-enabled.

## Remote engines

Bind mounts resolve on the **engine**, which may not be the machine you typed the
command on. Under Docker Desktop or a remote engine it is a different filesystem
with different paths.

The installer accepts a Unix socket or a reachable non-TLS TCP engine; SSH and TLS
engine endpoints are not implemented. Paths outside the installed workspace root
fail rather than mounting something unintended.

**A Docker engine endpoint is root on that machine.** Do not expose an
unauthenticated one to a network you do not trust.

## When a container dies

Its processes die with it, and so does anything written only to its writable layer.
State volumes and your workspace survive.

`cxz project up` or `cxz session resume` creates a new run and resumes the agent
conversation. Stale approvals from the old run fail rather than applying, and old
prompts are never replayed.

Codex may assign a new thread ID to a conversation that never had a turn — there is
no transcript to lose in that case.
