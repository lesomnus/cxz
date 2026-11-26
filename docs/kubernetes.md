# cxz on Kubernetes

The manager runs as a Deployment in a namespace of its own, and every project is
a pod with its own persistent volume. The scheduler places each project pod on a
node with room for its resource requests, so two projects compiling at once no
longer share one machine's CPU and memory. Sessions are processes inside their
project's pod, as they are in Docker. The design record is
[kubernetes-projects](plans/kubernetes-projects.md).

The Kubernetes backend has its own project settings. A project is an image, a
user, a volume size and resources; it is not a devcontainer, and nothing is
translated from one.

## Install

`deploy/kubernetes` is a [kustomize](https://kustomize.io) base, meant to be
applied once per installation from an overlay in your own configuration
repository. One installation is one namespace: a person's, or a team's.

```text
deploy/kubernetes/
  base/                   the manager, its Role, volume, Services and settings
  components/clickhouse/  a usage database of the installation's own
  examples/alice/         one person's overlay, to copy
```

The [example overlay](../deploy/kubernetes/examples/alice/kustomization.yaml)
marks what each installation provides with `CHANGE`:

- **the namespace**;
- **the `cxz-manager` Secret**: `owner` (24 hex characters, unique per
  installation), `token` (the TUI relay's), `web-token` (the browser's) and
  `telemetry-password`. The example generates it from placeholder literals;
  keep the real values in your secret management (a SealedSecret or an
  ExternalSecret named `cxz-manager`) instead;
- **the `cxz-telemetry` ConfigMap**: the usage database's connection, below;
- **the image tag**, through `images:`. The manager reads the image it runs from
  its own pod and project pods copy cxz from that same image, by digest when
  the node reports one, so the tag is set in one place;
- **`cxz-kubernetes` settings**, when projects need other than the defaults.

```sh
kubectl apply -k overlays/alice
```

The base creates a ServiceAccount whose Role is its namespace's pods,
`pods/exec`, volume claims and secrets, and nothing else; a 10 GiB claim for the
manager's state; the manager Deployment (one replica, `Recreate`, because its
databases are SQLite on that one volume) with its token relay, usage collector
and browser gateway beside it; a NodePort Service for the relay (30749) and
ClusterIP Services for the web, the Codex token broker and the collector.

Every pod, volume and secret an installation creates carries its `cxz.owner`
and `cxz.project` labels, and a manager acts only on objects whose labels are
its own. Between a manager and its projects every request is authenticated --
the project token, the broker's capabilities, signed telemetry -- so
installations sharing a cluster cannot reach into each other. A cluster with
many installations should still bound each namespace with a ResourceQuota and
a LimitRange, and keep namespaces apart with NetworkPolicy.

Codex accounts keep their default, brokered login: the manager holds the
central login and hands each project short-lived access tokens. On Docker the
broker is a Unix socket on the shared tools volume; a pod shares no volume, so
the manager also serves it at `CXZ_BROKER_LISTEN` and issues grants naming
`CXZ_BROKER_ADDRESS` (`tcp://cxz-broker:7351`). Every request carries the
capability issued for that project and account, as on the socket; the Service
is not published on any node.

### The usage database

Installations share one ClickHouse server; each has its own account and
database on it, which whoever runs the server creates:

```sql
CREATE DATABASE cxz_alice;
CREATE USER cxz_alice IDENTIFIED BY '...';
GRANT SELECT, INSERT, ALTER, CREATE TABLE ON cxz_alice.* TO cxz_alice;
```

The installation names them in `cxz-telemetry`, with the password as the
Secret's `telemetry-password`:

| Key | |
|---|---|
| `CXZ_TELEMETRY_DB_HTTP` | the server's HTTP interface, `http[s]://host:port`, for queries and the schema |
| `CXZ_TELEMETRY_DB_NATIVE` | its native protocol, `tcp://host:port`, `?secure=true` for TLS, for the collector |
| `CXZ_TELEMETRY_DB_USER`, `CXZ_TELEMETRY_DB_NAME` | the account and database; default `cxz` and `telemetry` |
| `CXZ_TELEMETRY_DB_HOST` | instead of both endpoints: a host on the default ports, plaintext |

cxz creates its tables in that database and touches nothing else. Without a
shared server, add `components/clickhouse` to the overlay: it runs one for the
installation and points `cxz-telemetry` at it.

## Settings

The `cxz-kubernetes` ConfigMap is how projects run:

```json
{
  "image": "mcr.microsoft.com/devcontainers/base:ubuntu",
  "user": "vscode",
  "storage": "20Gi",
  "storage_class": "",
  "requests": {"cpu": "500m", "memory": "1Gi"},
  "limits": {"memory": "4Gi"},
  "image_pull_policy": "IfNotPresent",
  "ready_timeout": "10m",
  "runtime_rollout": "idle"
}
```

- `image` is the project container's image when a project names none. It must
  contain `/etc/passwd` with `user`, and `sh` for terminals.
- `user` runs the project runtime, every session and every terminal. Empty runs
  them as the image's user; Claude refuses to bypass permissions as root, so a
  non-root user is the useful default.
- `storage` and `storage_class` size the project's volume. Empty class is the
  cluster's default.
- `runtime_rollout` is what happens to project pods after the manager is
  updated: `idle` replaces them once their sessions are idle, `manual` only
  reports them. See [Updates](#updates).
- `requests` are what the scheduler places by; `limits` are the most a project
  may use. Give requests for the usual load and limits for the peak.

Restart the manager after changing it (`kubectl rollout restart
deploy/cxz-manager`); projects take it on their next start.

A project may override `image`, `user`, `storage`, `requests` and `limits`, and
add [pod templates](#pod-templates), when it is added, as JSON or just an image reference:

```sh
cxz project add web --config '{"image": "node:24", "limits": {"memory": "8Gi"}}'
cxz project add api --config golang:1.27
```

A project's name is its identity in the cluster: 1–40 lowercase letters, digits
or hyphens. Its workspace is `/workspaces/NAME`.

## Pod templates

What cxz has no setting for -- a credential from a Secret, a node selector, a
GPU, a sidecar, a CA bundle -- is added to project pods with the cluster's own
`PodTemplate`. cxz merges them over the pod it builds, in order:

1. the settings' `pod_templates`, for every project;
2. the project's `pod_templates`;
3. the project's inline `pod_template`, a template without a resource.

```json
{"pod_templates": ["git"]}
```

```sh
cxz project add --config '{"pod_templates": ["gpu"], "pod_template": {"spec": {"containers": [{"name": "runtime", "env": [{"name": "NODE_ENV", "value": "test"}]}]}}}' ml
```

A template's `runtime` container is merged into the project's own container;
any other name is a sidecar. The API requires an `image` on every container of a
PodTemplate: on `runtime` it is ignored -- the project's image comes from its
settings and builds -- so write anything, such as `"-"`.

Merging follows kubectl's strategic merge: `containers`, `initContainers`,
`volumes`, `env` and `imagePullSecrets` merge by `name`, `volumeMounts` by
`mountPath`, `ports` by `containerPort`; `tolerations`, `envFrom` and
`topologySpreadConstraints` are appended, so templates compose; maps merge and
other values replace.

A template may not change what cxz runs the project with, and a start that
would is refused, naming the template: the runtime container's command, args,
image, working directory, ports, probes and lifecycle; its environment and
mounts from cxz and anything under `/cxz`; the `tools` and `prepare` init
containers; cxz's volumes; `cxz.*` labels; `restartPolicy` and
`shareProcessNamespace`. The runtime starts as root and drops to the project's
user itself, so leave `runAsUser` and `runAsNonRoot` alone.

Templates are read when a project's pod is created: a change applies on the
project's next start, and a missing one fails it. The manager reads
PodTemplates (its Role allows `get` and `list`); who writes them is the
installation's choice. The built-in `admin` and `edit` roles do not include
`podtemplates`, so grant it to whoever should, or keep them in gitops.

### Git credentials

Docker installations copy the host's GitHub login into projects. On Kubernetes
the credential is the person's own Secret, mounted with a template:

```yaml
apiVersion: v1
kind: PodTemplate
metadata: {name: git}
template:
  spec:
    containers:
      - name: runtime
        image: "-"   # required by the API; ignored
        env:
          # The token, for gh and for git through the helper below.
          - name: GH_TOKEN
            valueFrom: {secretKeyRef: {name: alice-github, key: token}}
          # git's configuration from the environment: a helper that answers
          # with the token for github.com. Nothing is written to the home.
          - {name: GIT_CONFIG_COUNT, value: "1"}
          - {name: GIT_CONFIG_KEY_0, value: credential.https://github.com.helper}
          - name: GIT_CONFIG_VALUE_0
            value: '!f() { test "$1" = get && echo username=x-access-token && echo "password=$GH_TOKEN"; }; f'
```

A file works the same way, from a Secret volume:

```yaml
        volumeMounts: [{name: gh, mountPath: /home/vscode/.config/gh, readOnly: true}]
    volumes: [{name: gh, secret: {secretName: alice-gh}}]   # its key: hosts.yml
```

Secret files are root's, mode 0644 by default: readable by the project user,
which suits a token. Prefer an HTTPS token to an SSH key, which `ssh` refuses
when others can read it. A Secret's contents are the installation's to manage
(an ExternalSecret, a SealedSecret); cxz only names it.

## Using it

**Command line.** Commands other than the TUI talk to the manager they run
beside, so run them in the manager's container:

```sh
alias kcxz='kubectl -n cxz-alice exec -it deploy/cxz-manager -c manager -- cxz --state /var/lib/cxz'
kcxz project add demo
kcxz project start demo
kcxz project exec demo -- git clone https://github.com/you/repo .
kcxz account add claude work
```

**TUI.** Through the relay, from anywhere that reaches a node:

```sh
cxz --endpoint tcp://NODE:30749 --token-file token
```

The relay is plaintext with a token: use it on a network you trust, or put it
behind a tunnel. Project terminals are not opened for remote callers, as in
Docker.

**Browser.** The gateway serves a loopback origin, which a browser treats as
secure without a certificate:

```sh
kubectl -n cxz-alice port-forward svc/cxz-web 7350
# open http://127.0.0.1:7350 and sign in with web-token
```

For a reachable HTTPS address, give each installation a DNS name -- a record
per installation in the same gitops change that adds it, or one wildcard
`*.cxz.example` pointing at the ingress controller -- and add one of two
components to the overlay:

```yaml
components:
  - ../../components/ingress   # or ../../components/tls
```

| Component | Who terminates TLS | The gateway |
| --- | --- | --- |
| `ingress` | the Ingress, with the Secret `cxz-web-tls` | plain HTTP on the pod network, `--tls-proxy` |
| `tls` | the gateway, with the Secret `cxz-web-tls` mounted | `--tls-cert`, `--tls-key`; route it with TLS passthrough or a LoadBalancer Service |

Both replace the `web` container's `--origin` with `https://cxz.example`, and
`ingress` sets the Ingress host to the same name: patch all of them to the
installation's name. A wildcard certificate serves every installation from one
Secret.

`--tls-proxy` trusts the proxy for encryption only. Sign-in, the exact
`Origin` and the `Host` check are still the gateway's, so the Ingress must pass
the browser's `Host` through, as every common controller does by default. It
also means the hop from the controller to the pod is plaintext on the pod
network: choose `tls` when that network is not trusted.

## Building images

cxz does not run an image builder or a registry, but it uses yours. Name both
in the settings and a project can be built from its own workspace:

```json
"build": {
  "buildkit": "tcp://buildkitd.build.svc:1234",
  "registry": "registry.example.com/cxz",
  "auth_secret": "cxz-registry"
}
```

- `buildkit` is a BuildKit daemon's address, as `buildctl --addr` takes it.
- `registry` is where images are pushed; a project's image is `REGISTRY/NAME`.
- `pull_registry` is how the nodes name that registry, when it differs from how
  the builder reaches it — for example a registry the builder reaches as
  `registry:5000` and the nodes pull as `localhost:30500`. Empty is `registry`.
- `auth_secret`, optional, is a `kubernetes.io/dockerconfigjson` Secret: builds
  push with it and project pods pull with it.
- `insecure` pushes over plain HTTP.

```sh
kcxz project build --dockerfile .devcontainer/Dockerfile --context . demo
kcxz project build demo        # again, with what it last built with
```

The build runs in the project's pod, where the workspace is, with the
`buildctl` the pod copies from the manager's image. It pushes
`REGISTRY/NAME:TIMESTAMP`, and the project is restarted on that image **by
digest**, so what runs is what was built whatever the tag points to later. The
volume, and with it the workspace, sessions and tools, is kept. The output of
the last build is the project's `build.log` in the manager's state.

The image still needs `/etc/passwd` with the project user and `sh`. Builds are
asked for, never automatic. The browser UI has no build action yet.

## Usage telemetry

Usage comes from [managed telemetry](managed-telemetry.md) here as on Docker,
with the same database image, limits and schema. What differs is who runs it:
the database is the [shared server](#the-usage-database) and the collector
runs as a container beside the manager, and the manager configures and checks them
instead of creating containers. It writes the collector's configuration and the
per-project ingest registries to its own volume, which the collector reads, and
creates the schema once the database answers.

A project pod cannot reach the collector's socket, so its runtime makes that
socket itself and forwards each connection to `CXZ_TELEMETRY_COLLECTOR`
(`tcp://cxz-telemetry:4317`). Agents and the supervisor send exactly what they
send on Docker, signed with the project's credential; the collector rejects
anything that does not verify against its registry. The collector's Service is
not published on any node.

## A project's pod

| Path | From |
|---|---|
| `/workspaces/NAME` | the project volume, `workspace/` |
| `/cxz/state` | the project volume, `state/` — the runtime's databases, sessions, logins |
| `/cxz/tools` | the project volume, `tools/` — cxz and the agent CLIs |
| `/cxz/secrets` | an in-memory emptyDir |

Two init containers run before the runtime: one copies cxz from the manager's
image into `tools/`, the other — in the project's image, as root — copies the
runtime manifest from the project's Secret and gives `state/`, `tools/` and
`workspace/` to the project user. The runtime then starts as the container's
command, run as that user by cxz itself, and the pod is ready when it answers on
port 7348. `shareProcessNamespace` makes the pause container PID 1, which reaps
what agents leave behind.

The workspace starts empty. Clone into it from a terminal or a session.

| Operation | What happens |
|---|---|
| start | Create the volume claim, secret and pod if missing; wait until the runtime answers |
| stop, remove | Delete the pod; keep the volume |
| erase | Delete the pod, the volume claim and the secret |

A volume claim is bound to the node its first pod ran on when the storage class
is node-local (`local-path`); the project comes back to that node. A network
storage class lets it move.

The first start of a project downloads its agent CLI into `tools/` in the
background; creating a session waits for it.

## Updates

cxz does not update itself on Kubernetes. The manager's version is the image
tag of its Deployment, and changing it is the deployment's job: edit the tag
where the installation is declared and let the cluster roll it out.

```yaml
images:
  - name: ghcr.io/lesomnus/cxz
    newTag: sha-1a2b3c4 # the new build
```

`cxz self-update`, `cxz use`, `install`, `uninstall` and `purge` refuse to run
against a Kubernetes manager and say so. The manager's background checks for
new builds and agent CLI versions do not run either.

A project pod's runtime is the manager's image at the moment the pod was made:
the `tools` init container copies cxz out of it. After a manager update,
existing pods still run the previous runtime. cxz carries the new one down to
them the way a Deployment replaces its pods when its template changes. The
version is still the one a person declared.

- **When:** a pod is replaced once every session in it has been idle for five
  minutes (`CXZ_UPDATE_IDLE` in the project, as for Docker). The check runs
  under the runtime's maintenance lease, so no turn can start between the check
  and the replacement.
- **One at a time:** at most one pod is replaced per minute, so a manager update
  does not restart every project at once.
- **Sessions:** the workspace and conversations are on the project's volume.
  Sessions that were running before are resumed afterwards. A session someone
  stopped stays stopped. Agent CLIs are refreshed as on any start.
- **Busy projects:** a project with work in flight (a turn, a tool, a pending
  approval, input being typed) is asked again on the next pass.
- **Visible:** the pod is labelled `app.kubernetes.io/managed-by: cxz` and
  annotated with its runtime's `cxz.revision`. A replacement leaves an Event
  (`RuntimeReplaced`) on the pod, shown by `kubectl describe` and
  `kubectl get events`. Until it is replaced, the project reports
  `runtime_outdated` (`cxz project ls --format json`), and the web UI marks its
  state icon.

Set `"runtime_rollout": "manual"` in the settings to only report outdated pods.
A person then replaces them with `cxz project stop` and `cxz project start`, or
`kubectl delete pod`. The default is `"idle"`.

A pod is outdated when its `tools` image differs from the manager's own. That
image is compared by digest when the node reports one, and by reference
otherwise. A moving tag such as `edge` that is pulled again without a digest
looks unchanged, which is one more reason to pin `sha-` tags.

**Compatibility.** A manager talks to runtimes of other builds as long as they
share its protocol (`cxzupdate.Protocol`, recorded on the pod as
`cxz.protocol`), so an outdated pod keeps working until it is replaced. A
release that changes the protocol makes the manager replace older pods without
waiting for idle, because it cannot be relied on to talk to them. A pod made
before these annotations existed is taken to speak the current protocol.

## What does not work yet

- **No devcontainer.json.** An image is either prebuilt or
  [built](#building-images) from a Dockerfile; devcontainer features, Compose,
  lifecycle hooks and a Docker engine inside projects are not available.
- **Conversation history needs the project running.** The manager cannot read a
  pod's volume, so it asks the project's runtime; a stopped project's history is
  not readable until it starts.
- **Auxiliary AI** (summaries, suggestions, titles) is off: it runs in containers
  of a Docker engine.
- GitHub credential sync and host git configuration are Docker's: mount your
  own with a [pod template](#git-credentials).
- The browser editor, file attachments, workspace downloads and copying memory
  between projects still use Docker and are not available.
