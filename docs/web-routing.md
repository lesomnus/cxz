# Web routing

The browser uses TanStack Router with a code-defined, typed route tree in
`ts/src/app/router.tsx`. Route paths and parameters are the source of truth for the
selected resource, session and settings topic. The root shell keeps authentication,
the Connection and shared resource inventory alive during navigation. RPC,
SessionHistory, draft caching and transcript scroll physics remain in their existing
components; the router does not preload session data or restore transcript scroll.

| Route                  | View                                      |
| ---------------------- | ----------------------------------------- |
| `/sessions`            | Session catalog                           |
| `/sessions/$sessionId` | Conversation identified by its runtime ID |
| `/projects`            | Project catalog                           |
| `/settings/general`    | General settings                          |
| `/settings/editor`     | Editor settings                           |

`/` redirects to `/sessions`, and `/settings` redirects to `/settings/general`.
Unknown routes show a recoverable page within the workspace. Signing in preserves
the requested deep link. General and Editor share their parent view so changing the
settings topic keeps the JSON editor, unsaved text and Undo history mounted.

`RouteLink` uses real anchors with TanStack's typed destinations. It shares
`ButtonContent` press behavior and the existing visual tokens, while retaining native
new-tab and copy-link behavior. Browser back/forward follows URL history; accepted
conversation drafts stay scoped to their existing Connection and session.

The Go gateway serves `index.html` for HTML navigation to client routes. Missing
assets, non-HTML requests and auth/RPC/editor/terminal paths keep their normal
responses. This fallback is needed for direct navigation and reload in installed
builds; Vite also provides the SPA fallback during local development.

The WASM sandbox shares the same route tree through hash history, preserving its
`/sandbox.html` entry point without requiring a separate server fallback. For
example, `/sandbox.html#/sessions/session-2` opens that scenario directly. The
scenario selector follows navigation history, and selecting a scenario navigates
to the corresponding session route.

`ts/e2e/routing.spec.ts` covers direct links, reload, auth, redirects, history,
new tabs and unknown routes. `ts/sandbox-e2e/routing.spec.ts` verifies sandbox
history and scenario synchronization. `internal/webui/static_test.go` covers the
fallback boundary and real-file responses.
