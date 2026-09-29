# Auxiliary AI

Two optional background tasks that read a conversation and produce something short:
a **summary** of what has happened, and a **suggestion** for what you might send
next.

They run on their own account and model, separate from the agent doing the work.
Both are **off by default**.

## Turning them on

`Ctrl+.` → **AI tasks** → select a task → `Enter`.
Choose a registered account, complete its login in the TUI, then choose a model
and effort from the provider's catalog. Use ↑/↓ (or Tab) and Enter at each step.
Selecting an effort validates and enables the task; **Provider default** leaves
reasoning effort to the provider. Esc cancels setup without changing the saved
profile. Space toggles an already configured task.

Login opens on the selected Manager, including from a Windows client over SSH.
The TUI shows the provider URL and accepts the returned Claude code; Ctrl+Y copies
the login URL. Cancellation returns to account selection. Empty catalogs and
errors stay visible with `r` to retry; no model names need to be typed.
Both the client and Manager must support the auxiliary login RPC.

An unsupported model or effort, or a failed login, is reported as an error — it
never silently falls back to another account. The first use takes a while, because
the manager has to prepare the official agent CLI.

## Authentication

Auxiliary tasks authenticate **separately from your sessions**. The original
session's credentials are never copied, even when both use the same subscription.

- A **central Codex account** reuses its existing `cxz account login`, supplied
  through an auxiliary binding distinct from any project's.
- **Claude**, and Codex accounts on `project-local-oauth`, need a dedicated login, provided by the same TUI setup flow.
  The host-side CLI remains available as an alternative:

  ```sh
  cxz ai login work
  ```

  Run that CLI command on the Linux Manager host. The TUI flow works remotely
  without opening a separate SSH shell.

Profiles live in a per-account Docker volume belonging to the installation.
Authentication persists between tasks, and tasks for one account run one at a time.
Login and model execution share a profile lock, so logging in again while a task is
running is refused as busy rather than corrupting the profile.

## Results

| Command | Behavior |
|---|---|
| `/summary on`, `/summary off` | Save automatic summary preference for this session |
| `/suggest on`, `/suggest off` | Save automatic suggestion preference for this session |
| `/summary`, `/suggest` | Generate once while off; do nothing while on |

Session preferences override the defaults in AI tasks and survive reconnection.
Turning automation off keeps the configured account/model available for one-shot use.
`on` applies to future completed turns. `off` cancels a running call containing that
 task, including a combined call. One-shot generation does not change preferences.

Summaries appear directly below the final response without a dialog. Suggestions
appear as ghost text in the empty composer and hide as soon as you type. Loading
uses animated dots; suggestion loading uses the same placeholder color.

A one-shot request reads at most 2048 retained events from an idle session and
requires the final turn's user input and response to be present.

`Alt+G` re-checks with the server that the suggestion is still valid before copying
it. **You review and send it yourself** — nothing is ever sent on your behalf. A
draft you have already typed is never overwritten, and `Alt+Enter` keeps inserting a
newline.

A suggestion becomes inapplicable when you send new input, when the run changes, or
when the configuration changes. When there is no sensible next step, it is simply
empty.

Summary and suggestion are generated in one call when they share an account, model
and effort. Separate calls include an available same-turn summary. A manual
suggestion requested while summary generation is running queues behind that call
and reuses its result without regenerating the summary.

## What gets collected

The manager collects new user inputs and completed turns **from the moment you
configure a profile**; only enabled tasks generate automatically. Earlier conversation is not processed in bulk. After a restart, only
events since the current manager started become new tasks. A session seen for the
first time starts from its last 512 events, so a turn already long in flight is
picked up from the next one instead.

Collection continues while no TUI is attached, and several clients watching the same
turn do not produce duplicates.

## Context and limits

Every task starts a fresh provider conversation. cxz assembles the context: a
per-session checkpoint plus recent user inputs and the agent's final replies. Tool
results, reasoning, and the agent's interim commentary before running a tool are all
excluded. The original conversation and its memory are never modified.

Limits are **UTF-8 byte** budgets, not token counts — there is no tokenizer here,
and these numbers do not promise a particular cost saving.

| | |
|---|---|
| User input / final reply | 8 KiB each; truncation is marked |
| Checkpoint refresh trigger | over 20 KiB of JSON, or 32 turns |
| Checkpoint | 6 KiB |
| Context sent to the model | 32 KiB plus fixed instructions |
| Structured model reply | 12 KiB |
| Retained recent turns | 48 KiB of JSON, or 64 turns |
| Inline summaries | Last 32 per session, at most 4 KiB each |
| Derived state | 256 sessions, least recently used evicted |
| Concurrency | 4 tasks, serial per account, 32 sessions running or queued |
| Time | 3 minutes per task; 15 minutes for a dedicated login |

A checkpoint is refreshed only when the budget is exceeded, and a failed refresh
keeps the old one. When the storage limit drops older turns, the model is told the
context is incomplete rather than left to assume otherwise.

These defaults exist to be measured and adjusted — long conversations, and Korean
text in particular, are where they will need revisiting.

## Isolation

Tasks run in a dedicated helper container. It does **not** mount your workspace,
your project state, or the Docker socket. Tools and MCP are disabled, and unexpected
requests are refused.

A task never waits for the source session and never blocks it. Failures are not
retried automatically, and a paid call whose outcome is unknown after a manager
restart is not re-sent.

Usage is available through `cxz ai status SESSION_ID`, using only what the provider returned. It is never
added to the original turn's metrics.

Deleting a session cleans up its derived context and results. The account's
auxiliary login profile stays — other sessions use it.

## CLI

```sh
cxz ai list
cxz ai models work
cxz ai login work
cxz ai set summary    --account work --model MODEL [--effort EFFORT]
cxz ai set suggestion --account work --model MODEL [--effort EFFORT]
cxz ai disable suggestion
cxz ai status SESSION_ID
cxz ai cancel SESSION_ID
```

Use values from `cxz ai models`. Omitting `--effort` takes the provider's default.
`cancel` cancels the auxiliary task only, never the conversation.

Design notes: [plans/auxiliary-ai.md](plans/auxiliary-ai.md) (Korean).
