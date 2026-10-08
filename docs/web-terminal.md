# Conversation workspace terminal

Press **Ctrl+`** (`Ctrl+Backquote`) in a conversation, including its composer or
terminal, to toggle the shell panel below the composer. The title bar's session
menu provides the same action on touch devices. The shortcut uses the physical
Backquote key or a backtick character, ignores key repeats and IME composition,
and excludes Alt/Meta/Shift combinations. A browser or OS shortcut intercepted
before delivery cannot be handled by the page; the button remains available.

The panel uses an xterm.js terminal with a login shell in the selected
session's **project** workspace, as its resolved remote user. It reuses
`ProjectService.Terminal`, the same PTY lifecycle and ownership validation used by
the TUI. It requires an existing running devcontainer; opening it does not start
a stopped project or connect the IDE. Input, shell output and resizing are separate
from agent conversations and never enter the event journal.

Opening focuses the shell. Folding focuses the composer and preserves the PTY,
shell variables, running commands, scrollback and the draft. Switching sessions,
leaving the conversation, resetting the sandbox, reloading or signing out closes
that panel's connection and shell; it is not a persistent server terminal. An
exited or disconnected shell shows **Reconnect**, which starts a new shell.
Scrollback is bounded to 2000 terminal lines independently of virtualized messages.
Panel sizing follows [style.css](../ts/src/style.css), bounded by the conversation
viewport. ResizeObserver updates the PTY within the project service's validated
dimension limits. The wide-view editor remains a separate adjacent pane.

## Browser transport

Browser fetch transports do not support bidirectional Connect streams, so the
gateway exposes a narrow same-origin WebSocket at
`/terminal/PROJECT?columns=N&rows=N`. Every upgrade requires the owner's existing
HttpOnly session cookie and an exact Origin. Logout, cookie expiry and gateway
shutdown cancel the WebSocket and upstream RPC. The gateway has no Docker socket,
database, arbitrary command or user/container selector; the Manager resolves the
registered project and verifies its owned running devcontainer.

Binary messages carry input/output bytes. Text controls carry `{columns, rows}`
or `{ack}`; text responses carry `{ready}`, `{exited}` or `{error}`. Input messages
are at most 32768 bytes, dimensions are validated, and unknown control fields are
rejected. Only one output chunk is in flight: the browser acknowledges its byte
count after xterm's write callback parses it. This backpressure bounds output
buffers when a command floods stdout, even while folded. Writes/acknowledgements
time out after 30 seconds. Browser input buffering is bounded to 1 MiB, with an
explicit disconnected state on congestion. xterm and its styles load lazily on
first use. The optional WebGL addon also loads on demand. It renders through the
GPU when available; failed downloads, unsupported WebGL or initialization errors
leave the DOM renderer in place. When the addon cannot recover a lost context,
it is disposed to restore the DOM renderer without restarting the shell or
clearing its buffer or selection. That terminal keeps using DOM until it closes;
opening a new terminal or reconnecting tries WebGL again.
Renderer cleanup runs before terminal disposal, and pending imports cannot attach
to a terminal that has already been closed. Text selection and copying remain
managed by xterm independently of the renderer. Hidden panels keep receiving
output while xterm defers drawing until they are visible again.

The standard ANSI colors use a subdued palette, with brighter variants on dark
backgrounds and deeper variants on light backgrounds. Theme changes recolor the
existing buffer without reconnecting the shell. Indexed and truecolor escape
sequences keep xterm's native handling. Palette values live in
`terminalTheme` in [workspace-terminal.tsx](../ts/src/workspace-terminal.tsx).
No addon interprets terminal output as HTML, opens links or accesses the clipboard.

## WASM sandbox

The in-process WASM transport supports bidirectional RPC directly. It uses the
same `ProjectService.Terminal` request/reply messages and UI, with a simulated
shell rather than a Linux VM. Supported fixture commands are `help`, `pwd`, `ls`,
`cat <file>`, `echo <text>`, `clear`, `exit`, plus Ctrl+C/Ctrl+D and backspace.
`cat` reads the same project-specific fixture files as the file viewer.
Each opened stream has independent input state. Unsupported commands report that
they are unavailable; no host commands or files are accessed.

## Verification

Gateway integration tests run a real Unix PTY through gRPC and the authenticated
WebSocket, checking command output, resize, origin/session rejection and revocation
while output is awaiting acknowledgement. Production browser tests cover real
PTY commands, Ctrl+C, shell state across folding, resizing, reconnect, sign-out
and the gateway CSP. Sandbox browser tests cover desktop shortcuts, focus/drafts,
project isolation, wide-view coexistence, reconnect and mobile sizing. Renderer
tests cover actual WebGL drawing and drag selection, unavailable WebGL, and forced
context loss with continued output and selection on the DOM renderer.

References: [xterm.js API](https://xtermjs.org/docs/api/terminal/classes/terminal/),
[flow control](https://xtermjs.org/docs/guides/flowcontrol/),
[Connect browser streaming](https://connectrpc.com/docs/web/getting-started/).
