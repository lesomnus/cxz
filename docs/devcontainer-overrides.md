# Project Compose overrides

Use `cxz edit` on the Linux daemon host to set `devcontainer.compose`. It accepts
an inline Compose object or a YAML/JSON file path. The host CLI snapshots the
contents and publishes them to the manager, where they survive TUI exit and
manager restart. `cxz up`, `project up/new/recreate` and `session new` reread and
publish the settings, including edits to an external override file.

This configures project containers. `docker.compose` configures the separate
[shared Docker engine](managed-docker.md), whose service is named `dind`.

## Inline configuration

```jsonc
{
  "devcontainer": {
    "compose": {
      "services": {
        "${DEVCONTAINER_SERVICE}": {
          "volumes": [
            {
              "type": "bind",
              "source": "${HOME}/workspaces",
              "target": "/workspaces",
              "bind": { "create_host_path": false }
            }
          ]
        }
      }
    }
  }
}
```

`${DEVCONTAINER_SERVICE}` selects the `service` from each project's
`devcontainer.json`; a literal service name such as `dev` is also supported.
Do not specify both the placeholder and the same literal service in one override.
`${HOME}` in override values expands to the home of the Linux user running the
host CLI, before publishing. Escaped `$${HOME}` and other Compose variables keep
their usual Compose meaning. Create the source directory first.

## File configuration

```jsonc
{
  "devcontainer": {
    "compose": "project.override.yaml"
  }
}
```

Paths are relative to the directory containing `settings.jsonc`. Absolute paths,
`~/` and `${HOME}` paths also work. YAML and JSON files must contain one Compose
object, up to 256 KiB. For example, `project.override.yaml`:

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

## Application and compatibility

Projects must use `dockerComposeFile`. Image/Dockerfile-only configurations fail
with an explanation if a Compose override is configured; it is never silently
ignored. No project source files are modified. Merge order is the project's
Compose files, then the user override, then cxz's internal runtime configuration.
cxz's ownership labels, runtime mounts and connection settings retain priority.
Relative paths inside the override follow Compose's normal first-file rule:
they are relative to the project's first Compose file, not the settings directory.

`cxz edit` saves and publishes changed settings. `cxz up WORKSPACE` also rereads
and publishes them, then creates a new project container or reuses the running
one. Existing containers are not automatically replaced. Use
`cxz project recreate WORKSPACE` to apply changed mounts; recreation replaces the
writable layer and disconnects editors, retaining source and named volumes.
The manager checks the combined Compose configuration and elevated-setting trust
before removing a container. Removing `devcontainer.compose` clears the saved
override the next time settings are published; existing containers still need
recreation to remove its effects.

TUI project preparation/recreation uses the manager's last published snapshot.
Windows/remote frontend editing remains local: publish host settings using cxz on
the Linux daemon host. Update the host CLI and run `cxz install --recreate` once
to install a manager that supports this feature. Older managers explicitly reject
configured overrides instead of silently ignoring them.

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

References: [Compose merge rules](https://docs.docker.com/compose/how-tos/multiple-compose-files/merge/)
and [variable interpolation](https://docs.docker.com/compose/how-tos/environment-variables/variable-interpolation/).
