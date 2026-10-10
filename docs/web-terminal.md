# Conversation workspace terminal

Press **Ctrl+`** (`Ctrl+Backquote`) in a conversation, including its composer or
terminal, to toggle the shell panel below the composer. The terminal icon to the
left of Send provides the same action on touch devices. The shortcut uses the
physical Backquote key or a backtick character, ignores key repeats and IME composition,
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
Panel sizing follows [style.css](../ts/src/app/styles/style.css), bounded by the conversation
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
rejected. Output flows continuously within a bounded byte window, rather than
waiting for an acknowledgement after every chunk. The gateway pauses when the
unparsed output reaches the high watermark and resumes at the low watermark.
The browser returns accumulated credit only after xterm's write callbacks parse
the bytes; a short timer also acknowledges the final partial batch. Small RPC
outputs are briefly coalesced before transmission to reduce WebSocket messages
and xterm writes. An exit/error status waits for preceding output to be parsed.
The socket reader processes acknowledgements independently of upstream input
writes, so a blocked PTY cannot prevent output from draining. Input queues and
the output window remain bounded, including while folded; stalled output and
writes time out. Protocol limits and batching intervals live in
[terminal_flow.go](../internal/webui/terminal_flow.go) and
[terminal-acknowledger.ts](../ts/src/features/workspace/terminal/terminal-acknowledger.ts).
Browser input buffering is bounded to 1 MiB, with an
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

The standard ANSI colors preserve their hue families with pastel/neon variants on dark
backgrounds and deeper variants on light backgrounds. Theme changes recolor the
existing buffer without reconnecting the shell. Indexed and truecolor escape
sequences keep xterm's native handling. Palette values live in
`terminalTheme` in [terminal-theme.ts](../ts/src/features/workspace/terminal/terminal-theme.ts).
No addon interprets terminal output as HTML, opens links or accesses the clipboard.

Selecting terminal text copies it automatically by default. A drag copies the
final selection on pointer release, rather than each intermediate position.
Keyboard selection changes also copy. The header shows **Copied** only after
the browser confirms the write, or **Copy failed** if clipboard access fails.
**Settings → General → Terminal → Copy on selection** controls this behavior
through the boolean `terminal.copyOnSelect` in the same browser settings file.
Changing it updates an open terminal without reconnecting its PTY. Native context
menu copying remains available; Ctrl+C continues to interrupt the shell. Browser
shortcuts such as Ctrl+Shift+C may be intercepted before the terminal sees them.

## Keyboard editing and selection

Ctrl+V passes through to the browser's trusted paste event, so xterm handles
clipboard text, line endings and bracketed paste without a separate clipboard
read. Ctrl+Backspace sends the usual Ctrl+W word-erase input; the connected shell
or terminal application determines its word boundaries and key bindings.

In the normal screen, Shift+arrows extend or shrink a selection from the terminal
cursor, or extend an existing mouse selection. Vertical movement preserves the
target column, selection follows scrollback when needed, and wide glyphs remain
whole. Escape dismisses a keyboard selection. Selection never edits the command
line; it uses the same Copy on selection preference and feedback as dragging.
Alternate-screen applications retain their Shift+arrow input. Ctrl/Alt/Meta
modified arrow combinations also remain available to the terminal application.

## WASM sandbox

The in-process WASM transport supports bidirectional RPC directly. It uses the
same `ProjectService.Terminal` request/reply messages and UI, with a simulated
shell rather than a Linux VM. Supported fixture commands are `help`, `pwd`, `ls`,
`cat <file>`, `echo <text>`, `clear`, `exit`, plus Ctrl+C/Ctrl+D, backspace and word erase.
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
Flow-control tests additionally delay acknowledgements, block upstream input,
verify the hard output bound and low-watermark resumption, preserve UTF-8 bytes
across frames, coalesce small fragments, reject excessive credit and drain final
output before exit. Large-paste browser coverage checks the received byte count
and checksum through a real PTY, followed by a sustained output burst.

References: [xterm.js API](https://xtermjs.org/docs/api/terminal/classes/terminal/),
[flow control](https://xtermjs.org/docs/guides/flowcontrol/),
[Connect browser streaming](https://connectrpc.com/docs/web/getting-started/).
