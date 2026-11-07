# Web browser state

Browser state is scoped to the page's origin and browser profile; it is not a
Manager setting and does not follow the user to another device or origin.

| State                                                                                 | Storage                                                                                            | Lifetime                                                                                                                          |
| ------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------- |
| User-editable settings (theme, editor, sound, etc.)                                   | localStorage, key `settings`, a JSON document                                                      | Across refresh and browser restarts                                                                                               |
| Conversation/editor split and file explorer width                                     | localStorage, key `cxz.layout.v1`, JSON `{ "session": 940, "file": 240 }` in CSS pixels            | Across refresh and browser restarts                                                                                               |
| Session drafts, paste contents and cursor selections                                  | sessionStorage, `cxz.draft.v1:` keys scoped by connection/session, with separate selection records | Across refresh within the tab                                                                                                     |
| Unread completion/error state and notification cursors                                | sessionStorage, `cxz.attention.v1:` key scoped by connection                                       | Across refresh within the tab                                                                                                     |
| Browser diagnostics                                                                   | OPFS `cxz-client-logs-v1`, rotating JSONL; bounded memory fallback                                 | Across reload/restart until size rotation, browser eviction, site-data clearing or explicit Clear; [contract](web-client-logs.md) |
| Memory document bodies and revisions                                                  | Payday Queries + Store in the authenticated Connection                                             | Across route changes until the connection is recreated; no IndexedDB mirror enabled                                               |
| Project collapse state, file tree expansion, open editor state and conversation cache | React state / authenticated Connection memory                                                      | Until the owning component or connection is recreated                                                                             |

The two saved widths are shared across projects and sessions on that origin.
Rendering clamps widths to the current viewport without replacing the saved
preference, so temporarily shrinking the window does not lose the wider layout.
Double-clicking a divider removes only its saved width and restores the CSS
default. Invalid/unavailable browser storage falls back to defaults; resizing
continues to work if a save is denied.

Layout geometry is deliberately separate from `settings`: dragging a divider
does not rewrite a settings document the user may be editing. Conversation
history and provider payloads are not written to browser storage by the web
conversation cache; the server remains the source of the journal.

## Memory document reads

Memory detail calls `MemoryService.method.read` through the connection's Payday
`Queries`, using `useConnectionQuery` from `ts/src/shared/api/use-query.ts`.
The method and normalized protobuf request form the cache key, including its
project scope. Returning to a document reads the retained answer synchronously;
Payday then revalidates in the background. A network request on reentry therefore
does not mean the cache was bypassed. Editing pins the document's starting body
and revision until Save or Cancel, so a background response cannot overwrite a
draft or advance its conflict base.

There is no separate cache-enabled flag. Using `Queries.get` and subscribing is
the opt-in; a direct `c.memories.read(...)` does not use that query cache. The
adapter supports an undefined request to disable a read, and the document uses
`watch: false` because its project-scoped service has a custom Watch shape. This
flag controls live watching, not caching. Explicit Reload uses `Queries.forget`.
The store identity is the authenticated reader identity supplied by the server.

This store has no disk mirror: full Memory documents are retained for navigation
within a connection, and are not persisted across browser refresh. Payday's
persistent form uses `openDisk` from `@lesomnus/payday/store/idb`, passes that
`disk` to `Store.open`, and awaits `store.hydrate()` before queries are rendered.
Task persistence already uses that form separately; this change does not alter
its cache or move Memory data into it. The existing Memory collection metadata
cache remains the connection-local `ReadCache` with its own freshness policy.
