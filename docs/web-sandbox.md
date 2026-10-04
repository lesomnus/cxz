# Web design sandbox

Run the actual cxz web UI against an in-browser Go/WASM simulator. It needs no
Manager, Docker, SSH tunnel, TLS certificate, provider account or paid model call.
The simulator is a design fixture, not a replacement for integration testing the
real Manager, authentication, providers or containers.

## Start

Use the repository's Go toolchain (go.mod) and Node/npm, then:

```sh
npm ci --prefix ts
npm run --prefix ts sandbox
```

Open `http://127.0.0.1:5173/sandbox.html` if a browser does not open automatically.
The command compiles WASM once and starts Vite on loopback; editing React/CSS
updates the preview. After changing Go simulator code, run
`npm run --prefix ts sandbox:prepare` and reset/reload the preview. There is no
server-side cxz installation to update. Ctrl+C stops the static development server.
The first WASM load is roughly 23 MiB; a loading indicator reports bytes loaded.

The script works from Windows or Linux with Go and Node installed. It uses Node
process APIs rather than POSIX shell environment syntax, and copies wasm_exec.js
from the same Go toolchain that compiled the module.

## Scenarios and controls

The Scenario selector opens one of the sample sessions:

- Korean and English answers with Markdown, tables, checklists and code.
- Long history exceeding the UI's 2,000-event window.
- An approval question using the regular answer controls.
- A simulated usage-limit/error case.
- Seeded random tool activity followed by a canned response.
- A stopped session to exercise resume and stop.
- A session in a second project to check project filtering.

Send any text to trigger timed tool events and a preset response. Tools only
produce fixture events; they never run commands or edit files. Interrupt/stop
cancel delayed work. Resume creates a new fake run. Approval answers resolve the
pending question; sending again in that scenario asks another question.

**Reset sandbox** recreates the entire Worker, discarding drafts, history and
pending work, and applies the Seed and Pace controls. Identical seeds and the same
per-session sequence of sends produce identical random tool sequences. Switching
scenarios alone preserves session state. Reloading the page also starts fresh.
History is bounded to 3,000 events per fake session; client rendering still uses
the regular 2,000-event window. Duplicate send IDs are remembered for 256 requests.

## Builds and verification

```sh
npm run --prefix ts sandbox:build
npm run --prefix ts test:sandbox
```

The first builds a standalone static preview into `ts/dist-sandbox`; the second
uses Playwright Chromium to test that build (install the Playwright browser and
OS dependencies first). It covers real WASM RPC, sending, approval, long history,
stop/resume, reset and seed replay without HTTP RPC/auth requests.

Serve a static build at the origin root, preserving its Worker/WASM assets and
application/wasm MIME type. `npx vite preview --config vite.sandbox.config.ts`
from `ts/` provides a local preview. Development/preview configuration supplies
COOP/COEP isolation headers; a different static host should supply the same.
The local simulator uses Go memory rather than SQLite/OPFS; no conversation
state survives a page reload. payday may cache the WASM executable bytes,
which is separate from conversation state.

Normal `npm run --prefix ts build` still writes only the authenticated production
web client into `internal/webui/assets`. Sandbox binaries, Worker and controls are
not shipped in that build. The generated `.sandbox` and `dist-sandbox` directories
are ignored by Git.

## Implementation boundaries

- `cmd/cxz-sandbox`: WASM-only entry point, grpc-dgram MessagePort gateway.
- `internal/websandbox`: in-memory ProjectService/SessionService subset and
  deterministic fake-agent jobs. Generated protobuf services are shared with the
  real server, but the fixture lifecycle is deliberately simplified. Unimplemented
  RPCs remain unimplemented; it does not exercise the production lifecycle stack.
- `ts/src/sandbox.tsx`: payday sandbox startup, seed/pace controls, reset and a
  sandbox Connection. No browser-auth bypass is added to the production app.
- `ts/src/app.tsx`: shared production workspace/conversation UI.
- `ts/src/connection.ts`: accepts a Connect Transport; normal connections use HTTPS,
  while the sandbox supplies payday's Worker transport.

Use the existing HTTPS browser fixture and Go integration tests for real gateway
behavior. This sandbox helps iterate on layout and interaction without real work.
