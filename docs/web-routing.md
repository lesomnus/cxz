# Web routing

The browser uses TanStack Router with a code-defined, typed route tree in
`ts/src/app/router.tsx`. Route paths and parameters are the source of truth for the
selected resource, session and settings topic. The root shell keeps authentication,
the Connection and shared resource inventory alive during navigation. RPC,
SessionHistory, draft caching and transcript scroll physics remain in their existing
components; the router does not preload session data or restore transcript scroll.

| Route                                  | View                                     |
| -------------------------------------- | ---------------------------------------- |
| `/c/_/session`                         | Session catalog                          |
| `/c/_/session/$sessionId`              | Conversation identified by alias or UUID |
| `/c/_/project`                         | Project catalog                          |
| `/c/_/project/$projectId`              | Project dashboard                        |
| `/c/_/project/$projectId/task`         | Project task table                       |
| `/c/_/project/$projectId/task/$taskId` | Task details                             |
| `/setting/general`                     | General settings                         |
| `/setting/editor`                      | Editor settings                          |

`/` redirects to `/c/_/session`, and `/setting` redirects to `/setting/general`.
Unknown routes show a recoverable page within the workspace. Signing in preserves
the requested deep link. General and Editor share their parent view so changing the
settings topic keeps the JSON editor, unsaved text and Undo history mounted.

`RouteLink` uses real anchors with TanStack's typed destinations. It shares
`ButtonContent` press behavior and the existing visual tokens, while retaining native
new-tab and copy-link behavior. Browser back/forward follows URL history; accepted
conversation drafts stay scoped to their existing Connection and session.

Task addresses nest under their project. Both path references accept aliases or
UUIDs; generated links prefer aliases. The `assignee` (session runtime ID) and
`run` query parameters filter the table. Clearing the filter returns to the
project's full table. The unscoped Tasks entry chooses the first available
project; legacy project-query bookmarks redirect to the nested address. An
unscoped task detail with several projects still requires a project choice.
Selected task identity remains stable while an alias is edited.

The HTTP server serves `index.html` for HTML navigation to client routes. Missing
assets, non-HTML requests and auth/RPC/editor/terminal paths keep their normal
responses. This fallback is needed for direct navigation and reload in installed
builds; Vite also provides the SPA fallback during local development.

The WASM sandbox shares the same route tree through hash history, preserving its
`/sandbox.html` entry point without requiring a separate server fallback. For
example, `/sandbox.html#/c/_/session/session-2` opens that scenario directly. The
scenario selector follows navigation history, and selecting a scenario navigates
to the corresponding session route.

`ts/e2e/routing.spec.ts` covers direct links, reload, auth, redirects, history,
new tabs and unknown routes. `ts/sandbox-e2e/routing.spec.ts` verifies sandbox
history and scenario synchronization. `internal/webui/static_test.go` covers the
fallback boundary and real-file responses.
