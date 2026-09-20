# Shared Docker engine

cxz can run one Docker-in-Docker engine shared by its projects. Images, build
cache, and inner Docker volumes live in a separate named volume. Project down or
recreation retains the shared engine and its data.

DinD is enabled by default when no mode is saved. Explicit `"mode": "off"`
is preserved. Use `cxz edit` to customize it:

```json
{
  "docker": {
    "mode": "dind",
    "image": "docker:29-dind",
    "compose": "docker-compose.override.yaml"
  }
}
```

`compose` is optional; its path is relative to `settings.json`, or absolute / `~/`.
The file is a Compose override for the **shared engine**, with service name `dind`:

```yaml
services:
  dind:
    mem_limit: 4g
    cpus: 4
    volumes:
      - /srv/build-cache:/cache
```

cxz generates a base Compose definition and asks Docker Compose to merge the
user override. It does not modify a project's own Compose files. The engine's
name, ownership labels, network, health check and cache volume are managed by cxz.
Supported override fields: image, command, environment, volumes, mem_limit, cpus,
shm_size, ulimits, storage_opt, dns, extra_hosts, cap_add, cap_drop, security_opt,
devices and sysctls. Additional mounts must be absolute bind paths on the Docker
engine host; missing bind sources fail instead of creating empty directories.
Environment interpolation, extra services, ports and top-level resource
declarations are not supported in this managed-engine override.

```sh
cxz docker sync    # save an edited override without restarting the engine
cxz docker up      # start, or apply changed settings by recreating the engine
cxz docker status  # endpoint / stopped / not running
cxz docker down    # remove the engine container; retain its data
```

Saving changed `docker` settings through `cxz edit` also syncs them. If only the
override file content changes, run `cxz docker sync` or `cxz docker up`. Settings
are stored on the manager and survive disconnects. Up may restart workloads
inside the engine when its configuration changes. Ordinary project startup
refuses to replace an engine whose configuration changed; it reports that an
explicit `cxz docker up` is needed.

After enabling Docker, create new projects or recreate existing project
containers to apply `DOCKER_HOST`. cxz starts the shared engine when preparing an
enabled project, connects it to the project's private network, and provides the
Docker CLI and Compose/Buildx plugins if missing. Update the host client and
manager (`cxz install --recreate`) first to publish those tools. Existing running
project processes are not silently reconfigured. Setting mode to `off` stops
adding the integration to newly created project containers; use `cxz docker down`
to stop the shared engine and recreate projects to remove the injected environment.
A subsequent project startup with mode `dind` starts the engine again.

The engine uses a privileged DinD container, with no published host ports. Its
TCP API is available on connected project networks; projects using it share
control of that engine. This is a separate Docker daemon from the host's daemon:
its named volumes and containers are a different namespace.

Bind paths in `docker run -v ...` are resolved **inside the shared engine**.
Build contexts sent by `docker build` work normally. To use bind mounts, add a
host bind to the engine override at the same path the project uses, for example
`/home/me/src/app:/workspaces/app`. Project workspaces are not automatically
mounted into the shared engine.

Manager uninstall removes the owned engine container and retains its cache
volume/network. Purge can remove those owned resources explicitly. TUI exit has
no effect on the engine.

Open TUI settings with **Ctrl+P** from the conversation or project list
(or `/settings`). Status and build cache usage refresh every ten seconds.
Buttons refresh, start/apply saved settings, stop the shared engine, or clear
unused build cache. Stop, cache cleanup and applying settings to a running
engine require confirmation. Cleanup retains images and volumes. Esc or Ctrl+P
returns to the previous view. Opening settings does not start Docker.
