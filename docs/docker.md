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

A project's own `devcontainer.json` always wins — cxz delegates images,
Dockerfiles, Compose, features and lifecycle hooks to the official devcontainer
CLI. Projects without one fall back, in this order:

1. your **default template**, if the installation has one;
2. a plain Debian devcontainer with a non-root `vscode` user. Deliberately
   boring: enough to run an agent, nothing opinionated about your toolchain.

Either way, changes need a `cxz project recreate`.

### Your own default template

The template is a directory, not a file, because a real devcontainer rarely is
one. Create `devcontainer/` beside `settings.jsonc` in your state directory
(`$CXZ_STATE`, or `$XDG_STATE_HOME/cxz`, by default `~/.local/state/cxz`):

```
~/.local/state/cxz/
├── settings.jsonc
└── devcontainer/
    ├── devcontainer.json      # required
    ├── Dockerfile             # whatever it references, in here too
    └── post-create.sh
```

`devcontainer.json` is required and needs one of `image`, `build`, `dockerFile`
or `dockerComposeFile`. JSONC comments are fine.

**Everything it references must live inside the directory.** The template is
snapshotted and sent to the manager, so a `Dockerfile`, build `context` or
`dockerComposeFile` outside it would not travel. cxz checks this when it
publishes the template — at `cxz up`, or when you save settings with `cxz edit` —
and names the missing reference, rather than letting the build fail later.
Absolute paths and native devcontainer variables (`${localWorkspaceFolder}`, …)
are left for the devcontainer CLI to resolve, so they are not checked here. A
Compose template additionally needs `service` and `workspaceFolder`, since cxz
cannot infer either.

`${cxz:projectName}` expands to the project's name — the repository name from
`remote.origin.url`, falling back to the directory name. It is substituted in
string values throughout `devcontainer.json`, which is what makes one template
usable by every project:

```jsonc
{
  "name": "${cxz:projectName}",
  "build": { "dockerfile": "Dockerfile" },
  "remoteUser": "vscode"
}
```

Leave `workspaceFolder` and `workspaceMount` out and cxz binds the real
workspace for you. It never rewrites them for a Compose template, where the mount
is the Compose file's business.

Limits: 256 files and 1 MiB total, regular files only — no symlinks, since they
could not be snapshotted meaningfully. The executable bit is preserved, so
lifecycle scripts work.

There is no `cxz edit devcontainer`: an editor opening a single file is the wrong
shape for a directory of several. Create the files yourself; `cxz up` validates
and publishes them, and refuses with an explanation if the manager predates the
feature.

### Not the same as the shared Compose override

A `docker-compose.yaml` **inside** `devcontainer/` is part of your default
template, used only by projects that have no devcontainer of their own. The
`docker-compose.yaml` **beside** `settings.jsonc` is the shared override below,
applied to every project including those with their own devcontainer. Same
filename, different jobs.

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
