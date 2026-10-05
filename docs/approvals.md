# Approvals and questions

Agents ask for two different things: **permission to act**, and **an answer to a
question**. Both are server state — they outlive the TUI, and any frontend attached
to the session sees them.

Standalone approval records in the conversation can be double-clicked to inspect
their original request and recorded resolution, including after completion. A
single click still starts text selection. MCP records show the server name and a
compact request message; the preview includes the full message and raw payload.
The check mark means permission was granted, not that the tool executed
successfully. If the resolution is absent from loaded history, the preview says
so rather than inferring that the request is still pending.

## The approval box

Pending approvals get their own box above the composer. `Tab` moves into it in
physical order (approvals, then composer); `Shift+Tab` reverses.

| | |
|---|---|
| `↑` `↓` | Select a request |
| `Enter` | Allow. On a question, opens its dialog |
| `Backspace` | Deny |
| `PgUp` `PgDn`, `Ctrl+↑` `Ctrl+↓`, wheel | Scroll the focused request |

The box shows a provider-specific title, command and reason first, then the full
native payload — nothing is truncated away, it just needs scrolling. `/approval`
puts the whole payload in the conversation instead.

After you decide, focus returns to the composer. That is deliberate: a second
`Enter` should not approve the next request.

Resolved approvals update their original row in place, with distinct colours for
allowed, denied and cancelled.

## MCP confirmations

A fieldless MCP permission prompt (for example, “Allow cxz_memory to run
memory_read?”) is a decision, not a question. FULL mode accepts it in the session
supervisor, including without a connected TUI.

In ASK mode the approval box shows **Accept**, **Decline**, and **Cancel**. Click a
button to send immediately; there is no separate Submit step. With the box focused,
`1`/`2`/`3` send the corresponding choice, or use `←`/`→` then `Enter`.

MCP forms that request actual field values still use the question dialog. URL-flow
confirmations use the same three buttons, but remain manual even in FULL mode:
complete the external flow before accepting. `/approval` shows the original payload.

## Questions

A question opens a focused dialog by itself. Dismissing it does not answer it —
`/answer`, or `Enter` on the pending row, reopens it.

| | |
|---|---|
| Arrows, `Tab` | Move |
| `Space`, `Enter` | Select an option |
| `Ctrl+S` | Next, or submit on the last question |
| `Esc` | Close without answering; the request stays pending |

Claude supports multiple selections and per-option previews; Codex uses its native
question IDs and its own policy for free text. **Other** accepts free text, and a
draft you typed under one option survives choosing a different one — only the
selected one is sent.

Several questions navigate with Next and Back, and Submit sends all the answers
together.

Answers keep their structure through the runtime as `{selected: [...], other:
"..."}` per question. Claude receives its native string answer plus annotations
preserving the exact selection and Other text; Codex receives native answer arrays.
Free text is trimmed, and text that exactly matches an option becomes that
selection rather than a duplicate.

The provider-neutral model lives in `internal/agentview/questions.go`. Original
payloads stay in the journal and remain visible through `/approval`.

The CLI equivalent still exists:

```sh
cxz session reply ID REQUEST_ID allow
cxz session reply ID REQUEST_ID allow '{"question text or id":"answer"}'
```

## Permission modes

```sh
/permission full     # automatic approval for this session
/permission ask      # manual again
```

The policy is saved in the session's journal, not in the TUI. `full` means the
**session supervisor** approves known tool, command, file and permission requests —
including while you are looking at another session, and including while every
frontend is closed. It survives agent restart, resume and container recreation.

New sessions default to `full`. A session where you explicitly saved `ask` stays
manual. A policy is never inherited from another session.

**Questions and unknown request types always need a human**, in either mode.

Neither mode changes the agent's own sandbox configuration. Codex runs with its
`untrusted` approval policy and no nested sandbox inside the devcontainer cxz owns;
its permission requests are surfaced to you rather than auto-answered by it.

Automatic and manual decisions share the same run and request validation and the
same delivery tracking. A decision already dispatched cannot be retracted, and an
ambiguous delivery is never retried automatically — `delivery_unknown` in the
history means check before you repeat the task.

## Restarting an agent

`/restart` opens a Confirm/Cancel dialog. Restarting stops active work and clears
pending approvals; session ID, account, model and history are kept, and the
conversation resumes in a new run. The last input is not resent.
