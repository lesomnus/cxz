# The TUI

`cxz` with no subcommand opens the TUI, and it is the only way in. Subcommands
print and exit; there is no `attach`, no `watch`, no `connect`.

Everything here is local rendering and input. The agent, its history and its
pending requests live on the server, so detaching changes nothing about the
conversation and two frontends can watch the same session.

Needs at least 40 × 14 cells. Follows terminal resizes.

## Keys

**Project list**

| | |
|---|---|
| `n`, `Ctrl+N` | New session: pick an account, log in if needed |
| `a` | Accounts |
| `↑` `↓`, `Enter` | Select and open a session |
| `r` | Rename the selected session |
| `s` | Stop it |
| `m` | Browse its retained memory and history |
| `Ctrl+X` twice within 3s | Stop and delete it; the journal is kept. `cxz session purge` destroys it |
| `Ctrl+.` | Settings |

**Conversation**

| | |
|---|---|
| `Ctrl+S` | Send |
| `Enter`, `Alt+Enter`, `Ctrl+J` | Newline |
| `Ctrl+Enter` | Send, where the terminal can distinguish it |
| `Ctrl+X` | Clear the draft |
| `Tab` / `Shift+Tab` | Between pending approvals and the composer |
| `F2` / `F3` | Allow / deny the pending approval |
| `F4` | Interrupt the turn |
| `Esc` twice within 3s | Confirm the interruption |
| `Ctrl+R` | Resume a stopped session |
| `Ctrl+Q` | Back to the project list; the agent keeps running |
| `Ctrl+C` | Copy the selection, a focused tool's contents, or a report |
| `Ctrl+D` | Detach |
| `PgUp` `PgDn`, wheel | Scroll |
| `Ctrl+Home` / `Ctrl+End` | First line / follow the latest |

Paste never submits, however it arrives.

On Unix, `Ctrl+Enter` needs a terminal that emits CSI-u or xterm modified-Enter
sequences. Where it does not, `Enter` and `Ctrl+Enter` are the same byte — use
`Ctrl+S`. Windows reads console modifier flags directly and does not have this
problem.

## Slash commands

Typing `/` overlays fuzzy-matched hints above the composer: arrows select, `Tab`
completes, `Esc` dismisses.

| | |
|---|---|
| `/help` | Shortcut help, locally, without prompting the agent |
| `/view` | Select tool rows with `↑` `↓`; `Enter` opens a preview |
| `/details` | The latest full tool input and result |
| `/approval` | The selected request's complete payload |
| `/answer` | Reopen the pending question dialog |
| `/usage` | Tokens, cost and elapsed time from the whole journal |
| `/context` | The agent's own context report |
| `/compact` | Native compaction; needs an idle session, keeps the cxz journal |
| `/permission full`, `/permission ask` | Saved approval policy |
| `/logs`, `/logs project` | Diagnostics; `r` refreshes |
| `/restart`, `/stop` | Restart or terminate this session's agent |
| `/record` | Diagnostic recording status and path |
| `/memory` | Retained agent memory |
| `/summary`, `/suggest` | Auxiliary AI result and next-message suggestion |

## Reading a conversation

The first two columns are an indicator gutter; everything else is indented. Agent
replies begin with `• CLAUDE` or `• CODEX` there.

Your own messages carry a local timestamp instead of a `YOU` label, on a shaded
background. Reply text is plain; only the speaker badge keeps the agent's colour.

Assistant text is rendered as CommonMark/GFM when it actually contains Markdown
structure, and left alone when it does not — neither provider marks its text as
Markdown, so the detection is a heuristic. HTML is not rendered, images are not
fetched, escape sequences are stripped. Code blocks get syntax highlighting, a
shaded background and a copy button; copies preserve the original code rather than
the screen wrapping.

State changes do not accumulate in the transcript. Only `working` animates a
spinner; `idle` is silent, and stopping, failing or interrupting shows up in the
notice row. All of it stays in the journal regardless.

A finished turn ends in a dim metrics footer: duration, cost, then tokens with
`↑` input, `↓` output, `↺` cache read, `⊕` cache write, `∑` total. Missing values
are omitted rather than guessed. Costs below ten cents switch to `¢`. Claude
cumulative cost is counted once per run, not re-added per turn; Codex metrics are
per-turn. This is measured usage, not your bill.

## Tool activity

A tool call and its result share one row, and the marker changes in place:

| | |
|---|---|
| `[ ]` | Requested or queued |
| `[•]` | Running |
| `[✓]` | Completed |
| `[×]` | Failed, denied or cancelled |

`[•]` requires evidence — an approval, or the provider saying so. A Claude request
on its own does not imply execution, so the row stays `[ ]` until something
confirms it.

File edits show paths and change counts, never file bodies: green `+N`, red `-N`.
`/match` means a replacement span per occurrence, not a file-wide total. Shell
calls show a compact command line; the first two wrapped lines appear inline and
the rest is in `/details`, which has Input and Output tabs with independent scroll.

## Scrolling and history

The TUI loads the last 128 events first and pages backwards as you scroll. New
output never pulls you away from where you are reading. If your latest prompt has
scrolled off the top, up to two of its lines stay pinned over the top edge.

The status row while scrolling shows a journal coordinate — `5,842/9,284`, counted
from the oldest retained event — not loaded line numbers. The loaded window slides
as you page, so row numbers say nothing about where you are; the coordinate only
moves when the journal does.

`Ctrl+End` returns to following. See [history and retention](history.md) for the
window's size and what evicts from it.

## Status bar

Left: a selection cell, the session alias, agent and model, `◉` account, title.

Right: provider-reported **remaining** account quota — percentage, an eight-cell
bar, the window label and a reset countdown. Bars colour only as quota drops: pastel
peach at 50%, coral at 30%, pink-red at 15%.

A `~` prefix means the snapshot is old. Expired windows show `refresh` rather than
assuming they reset to full. When there is no quota at all you get `quota waiting`,
`unsupported`, `unavailable` or `error`, and `/usage` explains which.

Quota comes from the already-authenticated agent process — Codex rate-limit RPCs,
Claude's experimental usage events — refreshed at startup, after turns and every
minute. An agent version that does not support it degrades without failing turns.

## Attaching host files

Type a backtick then `!`, then paste or drop a host path:

```
Compare `!/home/me/report.pdf` with this result.
```

After a short pause, or a closing backtick, a valid path becomes a chip with an
upload bar and size. **Host** means the machine running the client; an ordinary
backtick path browses the container instead. Bare paths are never attached
automatically.

Attachments are stored by the daemon in a content-addressed store with per-session
namespaces, exposed to the project through a read-only mount. Projects created
before this existed need recreating to get the mount.

## Notices, alerts and recordings

A notice row sits above the composer. Click one to copy its full text, including
lines truncated on screen.

The TUI plays a sound when a session finishes a turn or needs attention, including
sessions you are not looking at. Startup history and automatically approved
requests are silent. Bundled WAV files play through `afplay`, `paplay`/`aplay` or
PowerShell; SSH sessions use the terminal bell. Failures fall back to the bell with
a three-second timeout. Whether the bell is audible is your terminal's business,
and a muted OS cannot be detected.

`F9`, or the button in `Ctrl+.` settings, starts a diagnostic recording — a red
`⬤ REC` appears above the composer. It covers the whole TUI across session
switches. See [development](development.md#diagnostic-recordings) for what it
contains; typed text and conversation content are excluded.
