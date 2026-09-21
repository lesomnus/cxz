# Project Compose overrides

Run this on the Linux daemon host:

```sh
cxz edit docker-compose
```

It opens `docker-compose.yaml` beside `settings.jsonc` in `$VISUAL` / `$EDITOR`,
creating a file with commented examples if needed. The default file is optional:
if it is absent, empty, or contains only `services: {}`, no override is applied.
The settings file does not need an entry for this default path.

This configures project containers. `docker.compose` still configures the separate
[shared Docker engine](managed-docker.md), whose service is named `dind`.

## Mount a host directory

Replace the default template with:

```yaml
services:
  ${DEVCONTAINER_SERVICE}:
    volumes:
      - type: bind
        source: ${HOME}/workspaces
        target: /workspaces
        bind:
          create_host_path: false
```

`${DEVCONTAINER_SERVICE}` selects the `service` from each project's
`devcontainer.json`; a literal service name such as `dev` is also supported.
Do not specify both the placeholder and the same literal service in one override.
Compose variables such as `${HOME}` are resolved using the environment of the
Linux user running the host CLI and the override directory's optional `.env` file.
The host environment takes precedence. Use `$${NAME}` for a literal `${NAME}`.
Create the source directory first.

## Choose a different file

Only if needed, set a single path using `cxz edit`:

```jsonc
{
  "devcontainer": {
    "compose": "~/cxz/project-compose.yaml"
  }
}
```

`cxz edit docker-compose` then edits that file. Relative setting paths are based
on the directory containing `settings.jsonc`; absolute paths, `~/` and `${HOME}`
also work. A missing explicitly configured file is an error during project
preparation; the editor creates it. Removing this setting restores the default
path. The selected YAML/JSON file and its resolved snapshot each have a 256 KiB
limit.

## Include other files

The override uses Compose's `include` syntax:

```yaml
include:
  - ./compose-mounts.yaml
  - ./compose-services.yaml
```

Each included file resolves its own relative paths from its directory.
To merge a base and an override for the same included service, use one entry
with an ordered `path` list:

```yaml
include:
  - path:
      - ./shared/base.yaml
      - ./shared/local.yaml
```

cxz uses Compose's loader to resolve local includes, interpolation and relative
paths before publishing. The manager receives the resolved contents, so it does
not need the original Compose files. Relative bind sources and build paths in the
main override are based on that override's directory; paths in included files
use their respective directories. Referenced bind directories, build contexts
and other runtime files must still exist where Docker/the manager can access them.

## Application and compatibility

Projects must use `dockerComposeFile`. Image/Dockerfile-only configurations fail
with an explanation if a Compose override is configured. No project source files
are modified. Merge order is the project's Compose files, then the user override,
then cxz's internal runtime configuration. cxz's ownership labels, runtime mounts
and connection settings retain priority.

`cxz edit docker-compose` validates the draft before saving and publishes the
resolved override. Invalid edits leave the original untouched and retain a draft
for recovery. `cxz up`, `project up/new/recreate` and `session new` also reread and
publish the file and its includes. Editing included files separately therefore
takes effect on the next preparation command. Snapshots survive TUI exit and
manager restart.

Existing containers are not automatically replaced. Use
`cxz project recreate WORKSPACE` to apply changed mounts; recreation replaces the
writable layer and disconnects editors, retaining source and named volumes.
The manager checks the combined Compose configuration and elevated-setting trust
before removing a container. Emptying the override (or removing the default file)
clears its saved snapshot the next time settings are published; existing
containers still need recreation to remove its effects.

TUI project preparation/recreation uses the manager's last published snapshot.
Windows/remote frontend editing remains local: publish host settings using cxz on
the Linux daemon host. Update the host CLI and run `cxz install --recreate` once
to install a manager that supports project overrides. Older managers explicitly
reject configured overrides instead of silently ignoring them.

Old inline `devcontainer.compose` objects are retained for migration only. Run
`cxz edit docker-compose` to move one into the default YAML file and change the
setting to its path. If that file already exists, neither version is overwritten;
merge the contents and change the setting manually using `cxz edit`.

## Project-local alternative

Project containers also use all Compose files listed in their `devcontainer.json`.
Add an override after the base file to merge extra mounts into one project only.

For example, keep the project's other devcontainer settings and change its
`dockerComposeFile` to:

```json
{
  "dockerComposeFile": [
    "docker-compose.yaml",
    "docker-compose.local.yaml"
  ],
  "service": "dev",
  "workspaceFolder": "/workspace"
}
```

Paths in this list are relative to `devcontainer.json`. Use the existing service
and workspace folder from your project. For a host user's `~/workspaces` directory,
create `.devcontainer/docker-compose.local.yaml` with its actual absolute path:

```yaml
services:
  dev:
    volumes:
      - type: bind
        source: /home/me/workspaces
        target: /workspaces
        bind:
          create_host_path: false
```

Replace `/home/me/workspaces` with the directory on the Docker host and create it
first if needed. For a remote cxz connection this is the Linux server's path,
not the frontend machine's path. An override mount replaces an existing mount
with the same container target; mounts at other targets remain.

Do not rely on `${HOME}` in a project Compose file to mean the cxz user's host
home. cxz runs the devcontainer CLI and Compose inside its manager, inheriting
that container's environment. The default manager does not receive the host
user's `HOME`, so it can expand to `/root` instead. An absolute bind source avoids
that ambiguity. Adding `HOME` under a service's `environment` does not change
Compose's interpolation environment.

Run `cxz up WORKSPACE` to prepare a new project, then `cxz` to open the TUI.
For an existing container, apply mount changes with `cxz project recreate WORKSPACE`;
this replaces its writable layer and disconnects attached editors while keeping
workspace sources and named volumes. Plain `cxz up` reuses a running project and
does not apply these changes to it.

The mount above exposes the directory to the project container. If Docker
containers started by the project also need `/workspaces` as a bind source,
mount the same host directory at `/workspaces` in the shared engine's `dind`
override too; bind sources there are resolved inside that engine.

References: [Compose merge rules](https://docs.docker.com/compose/how-tos/multiple-compose-files/merge/),
[Compose include](https://docs.docker.com/reference/compose-file/include/)
and [variable interpolation](https://docs.docker.com/compose/how-tos/environment-variables/variable-interpolation/).
