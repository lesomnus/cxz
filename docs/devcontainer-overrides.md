# Project Compose overrides

Project containers use the Compose files listed in their `devcontainer.json`.
Add an override after the base file to merge extra mounts into the development
service. There is currently no cxz setting that applies a project Compose
override globally. `docker.compose` in `cxz edit` configures the separate
[shared Docker engine](managed-docker.md), whose service is named `dind`.

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
