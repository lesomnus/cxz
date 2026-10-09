# Auxiliary AI

Three optional background tasks that read a conversation and produce something short:
a **summary** of what has happened, and a **suggestion** for what you might send
next, and a **session title** for the project sidebar.

They run on their own account and model, separate from the agent doing the work.
All are **off by default**.

## Turning them on

`Ctrl+.` → **AI tasks** → select a task → `Enter`.
Choose a registered account, then choose a model and effort from the provider's
catalog. The account list starts with what the other tasks already use —
`use work1/sonnet-4-6/medium from Summary` — which copies that profile whole and
saves in one step, with no catalog lookup and no login probe, because the task it
came from already validated it. Setup first tries the account's stored auxiliary authentication; login
opens only when that profile needs credentials. Summary and suggestion share
authentication when they use the same account on the same Manager. Use ↑/↓ (or Tab) and Enter at each step.
Selecting an effort validates and enables the task; **Provider default** leaves
reasoning effort to the provider. Esc cancels setup without changing the saved
profile. Space toggles an already configured task.

Login opens on the selected Manager, including from a Windows client over SSH.
The TUI shows the provider URL and accepts the returned Claude code; Ctrl+Y copies
the login URL. Cancellation returns to account selection. Empty catalogs and
errors stay visible with `r` to retry; no model names need to be typed. At the
model step, `l` explicitly logs in again (for example, after credentials are revoked).
Catalog discovery does not generate a model response or prove that a stored token
will be accepted for generation; normal provider refresh still applies.
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

A summary is at most five short bullet lines — goal, current state, what remains,
what is blocked — in its own words rather than the reply's sentences, so it is
read at a glance instead of re-read. A suggestion is one imperative line you can
send as it stands. Both use the language of your recent messages.

The instructions behind that are one Markdown file,
[internal/auxiliary/instructions.md](../internal/auxiliary/instructions.md),
embedded in the binary. Every task shares it; the task name in the message
decides which fields of the reply are filled. The numbers it states are
substituted from the same constants that enforce the limits below, so the prompt
cannot ask for a reply that would then be rejected for its size.

A one-shot request reads at most 2048 retained events from an idle session and
requires the final turn's user input and response to be present.

Press `Right` while the composer is empty to copy the visible suggestion into an
editable draft, with the cursor at the end. This does not send it. Loading dots
and error messages are not accepted; existing input keeps normal cursor movement.

`Alt+G` re-checks with the server that the suggestion is still valid before copying
it. **You review and send it yourself** — nothing is ever sent on your behalf. A
draft you have already typed is never overwritten, and `Alt+Enter` keeps inserting a
newline.

A suggestion becomes inapplicable when you send new input, when the run changes, or
when the configuration changes. When there is no sensible next step, it is simply
empty.

Summary and suggestion are generated in one call when they share an account, model
and effort — one provider process and **one copy of the conversation** instead of
two, which is where the saving is. Equality is the whole profile, and the agent
and backend always come from the account, so the one that surprises people is
effort: the same account and model at a different reasoning level is two calls.
Picking `use … from …` when configuring the second task is the way to be sure,
and the AI tasks screen says which of the two you have:

```
Summary and suggestion share a profile: one call per turn, one copy of the conversation.
Summary and suggestion differ: a call each. Edit one and choose "use … from …" to share a call.
```

Separate calls include an available same-turn summary. A manual
suggestion requested while summary generation is running queues behind that call
and reuses its result without regenerating the summary.

The session title is always its own call: it runs on its own schedule and reads
different input, so sharing its profile with another task saves keystrokes but
not tokens.

## What gets collected

The manager collects new user inputs and completed turns **from the moment you
configure a profile**; only enabled tasks generate automatically. Earlier conversation is not processed in bulk. After a restart, only
events since the current manager started become new tasks. A session seen for the
first time starts from its last 512 events, so a turn already long in flight is
picked up from the next one instead.

Collection continues while no TUI is attached, and several clients watching the same
turn do not produce duplicates. Results arrive **as they are produced**: a client
subscribes to the session rather than asking repeatedly, so a summary appears when
it is written rather than up to a second later.

What ran is kept per turn rather than only for the turn in progress, so scrolling
back finds the summaries of earlier turns, and `cxz ai status` reports the recent
tasks with their usage rather than only the last one.

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
| Structured model reply | 1 MiB; refused past that |
| Asked of the model | 5 bullets of 120 characters, 500 characters of suggestion |
| Retained recent turns | 48 KiB of JSON, or 64 turns |
| Inline summaries | Last 200 turns per session, at most 4 KiB each |
| Task records | Last 200 per session, with what each one produced and cost |
| Derived state | 256 sessions, least recently used evicted |
| Concurrency | 4 tasks, serial per account, 32 sessions running or queued |
| Time | 3 minutes per task; 15 minutes for a dedicated login |

What the model is asked for and what is accepted are far apart on purpose. A
summary twice the length it was asked for is still a summary, so it is shown as
it came; overshooting a budget never costs you the reply. Past 1 MiB a reply is
refused, because it is one JSON object and half of one cannot be read — that is
the bound against a malfunction, not against a long answer. Anything in between
is shown, clipped at the limit and marked `[truncated]` if it ever gets there.

The checkpoint is the exception: it is refused when too large, because it is fed
back as context for every later turn instead of displayed, and a truncated one
would quietly rot what the model is told.

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

## Session titles

Configure **Session title** in AI tasks with the same account/model/effort picker.
For sessions created after enabling it, the first user input generates a draft;
the third successfully completed turn generates the final title. Failed turns do
not count. Titles do not periodically change afterward. Existing sessions are
not automatically renamed.

The English prompt requests a short title in the conversation's language. Only
bounded user requests and final answers from up to three turns are included;
available turn summaries can replace final answers. Tool output is excluded.
Naming uses separate, short auxiliary requests and shares existing account login.

Use `/title` to regenerate from retained completed conversation, or `/title set My title` to assign a title without AI. Both end automatic naming for that session.
The CLI equivalents are `cxz ai title SESSION` and `cxz ai title SESSION --text "My title"`. Generation needs a configured title profile and an idle session;
manual naming does not. `cxz ai status SESSION` includes title errors and state.

The TUI sidebar uses two rows per session: title, then the existing alias/agent/state
line. Empty titles display **Untitled**; long titles end with `...`. Titles persist
across Manager restarts and are independent of the session alias and UUID. No DB
reset is required. Background inventory reconciliation also publishes titles of
sessions that are not currently open (normally within 30 seconds).
