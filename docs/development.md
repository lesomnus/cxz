# Development

## Building

```sh
CGO_ENABLED=0 go build -o cxz ./cmd/cxz
```

Go 1.27. One binary is the CLI, the TUI, the manager and every in-container entry
point; which one it acts as depends on the subcommand.

`cxz manager serve` runs the server in the foreground against a local state
directory — the usual way to iterate on server code. `--agent /path/to/claude` points
it at a specific agent binary.

## Tests

```sh
go vet ./...
go test ./...
CXZ_TEST_RACE=1 go test -race ./internal/... -count=1
go run ./tools/genproto
go tool pd gen --check .
```

Generated code is checked in. `genproto` and `pd gen --check` verify it still
matches the `proto/` definitions — run them when you touch a `.proto`.

Cross-compile the frontend before pushing; CI builds Windows:

```sh
GOOS=windows GOARCH=amd64 go build -o /dev/null ./cmd/cxz
GOOS=windows GOARCH=arm64 go build -o /dev/null ./cmd/cxz
```

Build `./cmd/cxz`, not `./...` — some packages are Linux-only by design and will
fail for Windows without that meaning anything.

### Live probes

```sh
node scripts/probes/owned-session-live.mjs CLIENT_STATE PROJECT codex ACCOUNT
```

Probes use a real, explicitly selected, disposable project and spend real provider
usage. They are not part of `go test`.

## Images

CI uses a two-stage Bake: `build` exports static amd64 and arm64 binaries, then
`app` packages them with `internal/installer/image.Dockerfile`. The same Dockerfile
is embedded in `cxz install`, so a local install and a released image are built the
same way.

```sh
TAG=edge BUILD_HASH=$(git rev-parse HEAD) docker buildx bake build

# local smoke test, assuming an amd64 engine
TAG=local docker buildx bake app --set app.platform=linux/amd64 --load
docker run --rm ghcr.io/lesomnus/cxz:local version

# publish; needs registry credentials and both binaries prepared
TAG=edge BUILD_HASH=$(git rev-parse HEAD) docker buildx bake app --push
```

After tests pass, pushes to `main` publish `ghcr.io/lesomnus/cxz:edge` and
`ghcr.io/lesomnus/cxz:sha-COMMIT` for both platforms. Pull requests build without
registry login.

**`edge` moves.** Record a digest when you need an exact build.

The build context excludes workspace state and credentials. Tests run on the CI
runner, not inside the Docker build. `docker-bake.hcl` defines outputs, platforms,
tags and image metadata.

## Diagnostic recordings

`F9`, or the button in `Ctrl+.` settings, records what the TUI did. `/record status`
prints the path; the file is JSONL under the client state directory.

Recorded: decoded key categories, focus and cursor movement, window size, event
types, RPC status codes, and timings — update and render duration, transcript
rebuild cost and what it walked, window eviction, quota recomputation, history
requests.

Not recorded: typed or pasted text, conversation content, credentials, raw screen
output.

Up to 12,000 events are kept with a dropped count. A recording covers the whole TUI
process across session switches, not one conversation.

This is the tool for "the TUI feels slow" or "this key does nothing". A timing
without a recording is a guess.

## Pinned versions

Claude 2.1.267, Codex 0.154.0, devcontainer CLI 0.89.0. Codex integration follows
its generated schema and the official
[app-server protocol](https://learn.chatgpt.com/docs/app-server).

## Conventions

Commit subjects are capitalised and imperative, with no conventional-commit prefix.
The body says why, not what. Pull requests merge with `--rebase`, keeping history
linear.

There is no hand-written changelog: git history is the record of what changed, and
the pages in this directory are the reference for how things work.

## Not in scope

Web and IDE interfaces, roster authentication, dotfile and SSH forwarding, and
encrypted off-host backup are separate work. Large journals will eventually need
segmentation and indexing.
