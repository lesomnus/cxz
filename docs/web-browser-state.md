# Web browser state

Browser state is scoped to the page's origin and browser profile; it is not a
Manager setting and does not follow the user to another device or origin.

| State | Storage | Lifetime |
| --- | --- | --- |
| User-editable settings (theme, editor, sound, etc.) | localStorage, key `settings`, a JSON document | Across refresh and browser restarts |
| Conversation/editor split and file explorer width | localStorage, key `cxz.layout.v1`, JSON `{ "session": 940, "file": 240 }` in CSS pixels | Across refresh and browser restarts |
| Session drafts, paste contents and cursor selections | sessionStorage, `cxz.draft.v1:` keys scoped by connection/session, with separate selection records | Across refresh within the tab |
| Unread completion/error state and notification cursors | sessionStorage, `cxz.attention.v1:` key scoped by connection | Across refresh within the tab |
| Browser diagnostics | OPFS `cxz-client-logs-v1`, rotating JSONL; bounded memory fallback | Across reload/restart until size rotation, browser eviction, site-data clearing or explicit Clear; [contract](web-client-logs.md) |
| Project collapse state, file tree expansion, open editor state and conversation cache | React state / authenticated Connection memory | Until the owning component or connection is recreated |

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
