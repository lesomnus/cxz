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

```sh
kubectl create namespace cxz
kubectl -n cxz create secret generic cxz-manager \
  --from-literal=owner=$(openssl rand -hex 12) \
  --from-literal=token=$(openssl rand -hex 32) \
  --from-literal=web-token=$(openssl rand -hex 32)
kubectl -n cxz apply -f deploy/kubernetes/cxz.yaml
```

[`deploy/kubernetes/cxz.yaml`](../deploy/kubernetes/cxz.yaml) creates:

- a ServiceAccount whose Role is this namespace's pods, `pods/exec`, volume
  claims and secrets, and nothing else;
- a 10 GiB volume claim for the manager's state;
- the manager Deployment: one replica, `Recreate`, because its databases are
  SQLite on that one volume. Beside the manager run its token relay (for the
  TUI) and the browser gateway, both reading only the manager's sockets;
- a NodePort Service for the relay (30749) and a ClusterIP Service for the web.

The manifest names `ghcr.io/lesomnus/cxz:edge`. Pin a `sha-…` tag for anything
you rely on; a project pod copies its cxz from the manager's image when it
starts, so the two always match.

`owner` identifies this installation's resources: every pod, volume and secret
it creates carries `cxz.owner` and `cxz.project` labels, and the manager acts
only on objects whose labels are its own. `token` and `web-token` are what
clients present.

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
  "ready_timeout": "10m"
}
```

- `image` is the project container's image when a project names none. It must
  contain `/etc/passwd` with `user`, and `sh` for terminals.
- `user` runs the project runtime, every session and every terminal. Empty runs
  them as the image's user; Claude refuses to bypass permissions as root, so a
  non-root user is the useful default.
- `storage` and `storage_class` size the project's volume. Empty class is the
  cluster's default.
- `requests` are what the scheduler places by; `limits` are the most a project
  may use. Give requests for the usual load and limits for the peak.

Restart the manager after changing it (`kubectl -n cxz rollout restart
deploy/cxz-manager`); projects take it on their next start.

A project may override `image`, `user`, `storage`, `requests` and `limits` when
it is added, as JSON or just an image reference:

```sh
cxz project add web --config '{"image": "node:24", "limits": {"memory": "8Gi"}}'
cxz project add api --config golang:1.27
```

A project's name is its identity in the cluster: 1–40 lowercase letters, digits
or hyphens. Its workspace is `/workspaces/NAME`.

## Using it

**Command line.** Commands other than the TUI talk to the manager they run
beside, so run them in the manager's container:

```sh
alias kcxz='kubectl -n cxz exec -it deploy/cxz-manager -c manager -- cxz --state /var/lib/cxz'
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
kubectl -n cxz port-forward svc/cxz-web 7350
# open http://127.0.0.1:7350 and sign in with web-token
```

For a reachable HTTPS address, mount a certificate and key into the `web`
container from a Secret and give it `--origin https://HOST --tls-cert … --tls-key …`.
The gateway terminates TLS itself; it does not trust a proxy's.

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

## What does not work yet

- **Images are prebuilt.** No devcontainer.json, Dockerfile, features, Compose,
  lifecycle hooks or Docker engine inside projects.
- **Conversation history needs the project running.** The manager cannot read a
  pod's volume, so it asks the project's runtime; a stopped project's history is
  not readable until it starts.
- **Usage statistics** are unavailable: the telemetry store is not deployed on
  Kubernetes yet. See [managed telemetry](managed-telemetry.md).
- **Auxiliary AI** (summaries, suggestions, titles) is off: it runs in containers
  of a Docker engine.
- **Codex** accounts need `project-local-oauth`; the brokered backend's token
  broker is not reachable from pods.
- **Updates.** A project's runtime is the manager's image, taken when its pod
  starts; agent CLIs are fetched at start. Nothing updates a running pod.
- GitHub credential sync, host git configuration, the browser editor, file
  attachments, workspace downloads and copying memory between projects still
  use Docker and are not available.
