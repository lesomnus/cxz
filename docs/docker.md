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
2. the **built-in default**, which is itself a template:
   [`internal/projectconfig/devcontainer/`](../internal/projectconfig/devcontainer/).
   Read it rather than taking this page's word for it — it is a real
   `devcontainer.json` and `docker-compose.yaml`, comments and all, embedded
   into the binary. It is Compose-based, like this repository's own
   `.devcontainer`: a Go development image, a non-root user, CPU/memory limits,
   per-project shell history and machine-wide Go module and build caches.

Either way, changes need a `cxz project recreate`.

### Seeing what applies

Your `.devcontainer` is read, never written. What runs is that file with cxz's
own keys added and its Compose override merged after yours, and all of it lands
in cxz's state directory rather than in your repository. To read the result
instead of inferring it:

```sh
cxz devcontainer render            # the project in this directory
cxz devcontainer render PROJECT
```

It writes a directory and prints the path:

```
/tmp/cxz-devcontainer-1864239057
  devcontainer.json              what cxz passed to the devcontainer CLI
  source/devcontainer.json       the project's own configuration
  compose/01-docker-compose.yaml a Compose file from the project's devcontainer
  compose/02-compose.user.json   the installation's shared Compose override
  compose/03-compose.json        cxz's own override, merged last
  compose/resolved.yaml          the merge, as Compose resolves it
  sources.txt                    where each file came from
```

The numbers are the merge order, and `resolved.yaml` is where a question like
"which name did my cache volume end up with" is actually answered. These are the
files cxz handed the CLI, read back rather than rendered again, so the project
has to have been started at least once; before that there is no answer to give,
only a guess.

The built-in default is a template so that there is one code path and one set of
rules: whatever this page says about templates below is equally true of it, and
replacing it for your installation means putting your own template where cxz
looks first. There is no command that copies it out — read it in the repository
and write the version you want.

### Your own default template

The template is a directory, not a file, because a real devcontainer rarely is
one. Create `devcontainer/` beside `settings.jsonc` in your state directory
(`$CXZ_STATE`, or `$XDG_STATE_HOME/cxz`, by default `~/.local/state/cxz`):

```
~/.local/state/cxz/
├── settings.jsonc
└── devcontainer/
    ├── devcontainer.json      # required
    ├── docker-compose.yaml    # or a Dockerfile, or neither
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

Two variables make one template usable by every project. Both are substituted
in `devcontainer.json` and in the template's own Compose files:

| | |
|---|---|
| `${cxz:projectName}` | the repository name from `remote.origin.url`, falling back to the directory name |
| `${cxz:workspace}` | the workspace path on the host |

```jsonc
{
  "name": "${cxz:projectName}",
  "dockerComposeFile": "docker-compose.yaml",
  "service": "dev",
  "workspaceFolder": "/workspace"
}
```

```yaml
services:
  dev:
    image: ghcr.io/you/dev:latest
    command: sleep infinity
    volumes:
      - ${cxz:workspace}:/workspace
      - go.cache.mod:/home/you/go/pkg/mod
volumes:
  go.cache.mod:
```

**A Compose template must mount the workspace itself**, and `${cxz:workspace}`
is how. The template is materialized outside the workspace, so the `..` a
repository's own `.devcontainer` would use points at the snapshot instead of your
sources. Values are substituted through the YAML parser rather than into the
text, so a path containing a quote or a colon cannot rewrite the document.

An image or `build` template is simpler: leave `workspaceFolder` and
`workspaceMount` out and cxz binds the workspace for you. It never rewrites
either for a Compose template, where the mount is the Compose file's business.

#### Choosing a volume's scope

Compose decides scope by how a volume is declared, and the default uses both:

```yaml
volumes:
  command.history:            # per project: Compose prefixes it
  go.cache.mod:
    name: cxz-go-cache-mod    # one for the machine: the name escapes the prefix
```

A bare declaration becomes `cxz-<owner>-<project>_command.history`, so each
project keeps its own. A pinned `name:` is used verbatim, so every project
sharing that name shares the volume — which is what a content-addressed cache
wants and what shell history does not.

Compose **creates** a pinned volume if it is missing. Only `external: true`
demands that it already exist, which would make a project fail to start on a
fresh machine, so the default does not use it. A devcontainer `mounts` entry
(`type=volume,source=NAME`) behaves like a pinned name — verbatim, created on
demand, shared by every project naming it — which is why an image-based template
cannot get per-project volumes this way.

These volumes carry Compose's labels, not cxz's, so `cxz purge` leaves them
alongside everything else it did not label. Remove them with `docker volume rm`
if you want them gone.

Limits: 256 files and 1 MiB total, regular files only — no symlinks, since they
could not be snapshotted meaningfully. The executable bit is preserved, so
lifecycle scripts work.

There is no `cxz edit devcontainer`: an editor opening a single file is the wrong
shape for a directory of several. Create the files yourself; `cxz up` validates
and publishes them, and refuses with an explanation if the manager predates the
feature.

The built-in default is the worked example — it is a template, under
[`internal/projectconfig/devcontainer/`](../internal/projectconfig/devcontainer/).

### Not the same as the shared Compose override

Both the built-in default and most templates have a `docker-compose.yaml`, so the
filename appears twice with two different jobs:

| | |
|---|---|
| **inside** `devcontainer/` | part of the template; used only by projects with no devcontainer of their own |
| **beside** `settings.jsonc` | the shared override below; layered onto every project, including those with their own devcontainer |

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

### Projects that do not use Compose

The override is written as Compose because most devcontainers are, including the
default. A project whose own devcontainer is image- or Dockerfile-based never
invokes Compose, so cxz translates the override for it instead: the development
service's `volumes` become devcontainer mounts and its `environment` becomes
`containerEnv`. Both spellings of a volume work, short (`/host/src:/workspaces:ro`)
and long, and a source that is not a path is treated as a named volume exactly as
Compose would.

Anything with no equivalent is **refused by name** rather than dropped — a
sidecar service, `privileged`, a `command`, a top-level `networks` block. An
override that quietly did less than it says would be worse than one that does
not start, and the fix is to give that project a Compose devcontainer of its own.

cxz's own environment (`CXZ_STATE` and friends) is applied after the override, so
an override cannot redefine it.

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
