# Development

## User-visible changes

For a user-visible behavior change, update the stable feature ID in the
[feature specification](feature-spec.md) and the affected CLI/TUI/web cells in
the [support matrix](client-support.md) in the same PR. Link implementation
evidence and explain partial support or platform gaps; an open PR or a server
RPC does not count as an implemented client feature.

Update the [mobile web plan](plans/mobile-web.md) only when delivery scope,
sequencing or open design decisions change. Keep proposed priorities out of the
support matrix. Use the specification's shared contracts and the plan's mobile
acceptance scenarios when validating equivalent client operations.

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

For web UI edits against a real local Manager, run `cxz web up` on that host and
`npm run dev` in `ts/`. Vite reads the installed web settings and signs in using
the local token file. See [web development](web.md#developing-the-ui-against-the-installed-manager).

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

## Web client

The pinned payday Go module provides Connect/gRPC-Web serving. `ts/` contains
its generated TypeScript contracts and the React client, using payday
Store/Queries. To regenerate, build and test:

```sh
npm ci --prefix ts
go tool pd gen --ts .
npm run --prefix ts build
npm run --prefix ts test
cd ts
npx playwright install --with-deps chromium
npm run test:browser
npm run test:browser:plaintext
```

Commit `ts/gen/` and `internal/webui/assets/` after changes. The latter is the
production build embedded in the Go binary: Go-only builds and source-based
self-updates need no Node toolchain. CI rebuilds and rejects asset/schema drift.
Keep generated files untouched by formatting tools. `npm run --prefix ts format`
formats authored client source only.

Playwright launches an opt-in gateway fixture backed by in-memory gRPC services;
it does not start Docker or call a real agent. See
`internal/webui/browser_test.go` and `ts/e2e/`. `test:browser:plaintext` runs the
same fixture over loopback http, which is the desktop gateway's own transport:
what it checks is a browser rule rather than ours, that Chrome treats
`http://127.0.0.1` as a secure context and so keeps the `__Host-` session
cookie. Ordinary `go test ./...` skips both fixtures. See [web.md](web.md) for
actual gateway usage.

## Browser-only design sandbox

`npm run --prefix ts sandbox` builds the Go/WASM fake backend and opens the shared
web UI without Docker or accounts. See [web design sandbox](web-sandbox.md) for
scenarios, reset/seed controls, static builds and test boundaries.
