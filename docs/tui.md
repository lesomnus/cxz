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
| `?` | Open the same help dialog as `/help` |
| `a` | Accounts |
| `↑` `↓`, `Enter` | Select and open a session |
| `r` | Rename the selected session |
| `s` | Stop it |
| `m` | Browse its retained memory and history |
| `Ctrl+X` twice within 3s | Stop and delete it; the journal is kept. `cxz session purge` destroys it |
| `Ctrl+.` | Settings |

The sidebar footer shows only **new**, **? help**, and **Ctrl+D detach**.
Hover a project title to reveal **+** on its right; click it to open the
new-session account picker for that project. Other shortcuts remain available.

**Conversation**

| | |
|---|---|
| `Ctrl+S` | Send |
| `Enter`, `Alt+Enter`, `Ctrl+J` | Newline |
| `Ctrl+Enter` | Send, where the terminal can distinguish it |
| `Ctrl+X` | Clear the draft; with nothing to clear, take back the message waiting to be sent |
| `Tab` / `Shift+Tab` | Indent/outdent; active completions and visible approval/error focus take priority |
| `Alt+↑` / `Alt+↓` | Move current or selected logical lines |
| `Alt+D` | Duplicate selection or current line |
| `Ctrl+Z` / `Ctrl+Y` | Undo / redo edits |
| Double / triple click | Select word / logical line; drag to extend |
| `F2` | Edit the selected project title; Enter saves, Esc cancels |
| `F4` | Interrupt the turn |
| `Esc` twice within 3s | Confirm the interruption |
| `Ctrl+R` | Resume a stopped session; a yellow dot blinks beside it until the request finishes |
| `Ctrl+Q` | Back to the project list; the agent keeps running |
| `Ctrl+C` | Copy the selection, a focused tool's contents, or a report |
| `Ctrl+D` | Detach |
| `PgUp` `PgDn`, wheel | Scroll |
| `Ctrl+Home` / `Ctrl+End` | First line / follow the latest |
| `Home` / `End` | Start / end of the row you can see; again steps to the next row |
| `Shift` + arrows | Select by character or row |
| `Shift` + `Home` / `End` | Select to the start / end of the visible row |
| `Ctrl+Shift+←` / `→` | Select by word |

The composer grows to twelve rows — or a third of a short screen — and then
holds; past that a bar on its right edge shows where in the draft you are, and
the wheel over the composer moves through it without moving the cursor or
selection. Typing returns to the cursor. Tab uses four-column space-based stops;
Enter, Alt+Enter and Ctrl+J repeat leading indentation. Pasted text is not
automatically indented. The bar's column is reserved
whether or not a bar is in it, so adding a line never rewraps what is already
written.

Paste never submits, however it arrives.

On Unix, `Ctrl+Enter` needs a terminal that emits CSI-u or xterm modified-Enter
sequences. Where it does not, `Enter` and `Ctrl+Enter` are the same byte — use
`Ctrl+S`. Windows reads console modifier flags directly and does not have this
problem.

## Sending while the agent works

`Ctrl+S` during a turn does not bounce. cxz takes the message and holds it, and
the row above the composer says it is waiting; it reaches the agent at the first
moment that agent can take one. Claude takes it when its turn ends. Codex takes
it into the turn already running, at its next step, so it usually lands within
seconds.

One message waits at a time. A second is refused and comes straight back to the
composer, so nothing you typed is lost. `Ctrl+X` with an empty composer takes
the waiting one back and returns it to the composer as well.

The message is the session's, not this window's: another frontend on the same
session shows the same one waiting, and detaching does not lose it. Interrupting
drops it — you interrupted to change direction — and says so rather than
delivering it the moment the turn stops.

It is not in the conversation until the agent has it. That is also where the
journal records it, so reading the transcript later shows the message where the
agent actually saw it rather than where it was typed.

## Slash commands

Typing `@` shows a grid of other session aliases in the current project.
Type to filter; arrow keys select, Enter/Tab insert, and Esc closes. Mouse hover,
click and wheel work too. The selected session's title and agent appear below
the grid. Completion only inserts text; it never sends the message.

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
| `/model`, `/effort` | Provider catalog; `Alt+R` asks the provider again |

### What the model selector shows

`/model` and `/effort` open a selector over the **capability record** this run
published: the catalog the agent reported, what cxz asked it to use, and — where
the provider reports it — what it confirmed is applied. The record is served by
the manager in one request; the selector does not read the conversation to find
it, and `/model` followed by `/effort` asks once.

Under the title it says where the answer came from, how old it is, and whether
the provider confirmed the applied value. That last part matters: Claude reads
its settings back, Codex reports none, so for Codex the selector shows what was
requested and labels it `applied value not confirmed by the provider`.

`Alt+R` asks the agent again, which is the only way to learn that a provider
changed something by itself — a fallback to a smaller model after a limit, for
example. It needs an idle session, does not wait for the answer, and the selector
updates when the new record is published.

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

Every spinner turns at the same rate but starts from its own phase, taken from
when that work began, so two of them read as two things working rather than one
animation. The bright green marks where the keyboard is; while the terminal
window itself is blurred it steps back to the quiet green, since the keyboard is
then not in cxz at all. A spinner stays bright either way — the work continues
in a window you are not looking at — and the composer cursor holds still instead
of blinking.

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
Clicking those pinned lines scrolls back to that message.

The status row while scrolling shows a journal coordinate — `5,842/9,284`, counted
from the oldest retained event — not loaded line numbers. The loaded window slides
as you page, so row numbers say nothing about where you are; the coordinate only
moves when the journal does.

`Ctrl+End` returns to following. See [history and retention](history.md) for the
window's size and what evicts from it.

## Status bar

Left: the model the agent is running, and its reasoning level once the provider
confirms one — `opus-4-6 · high`. Before any confirmation it is the model the
session was created with, with no effort shown rather than a guessed one.

Permission mode is not here. It is a policy saved on the server and read back,
not something that changes while you type; `/permission` reports it.

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

Conversation text supports double-click word selection and drag selection. Ctrl+C copies the selected text without the assistant response's two-column display indent; indentation inside code blocks is preserved. Selection currently copies rendered text, including visual line wraps, rather than Markdown source.
