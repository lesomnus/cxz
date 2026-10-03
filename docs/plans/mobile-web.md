# Mobile web implementation plan

This is a proposal for delivery, separate from [feature behavior](../feature-spec.md)
and [current CLI/TUI/web support](../client-support.md). IDs below refer to that
specification; their current implementation status is never defined here.

## Confirmed direction

Start with **HTTPS inside a VPN**. The server keeps owning sessions, agent
processes, approvals, auxiliary tasks and memory. Closing a browser must not stop
work. Native apps and public Internet exposure are not initial targets.

VPN access does not choose an application authentication mechanism. Browser
transport, login/session lifecycle, certificate provisioning and draft storage
policy remain design decisions. Existing desktop SSH/TCP routing is not a browser
API implementation.

## Proposed scope

Only the VPN/HTTPS direction is confirmed. These priorities need agreement before
implementation; they are not release commitments. Definitions and exact client
support belong in the other two documents, linked above.

### First usable mobile release

| Feature ID | Proposal and reason |
|---|---|
| NAV-01 | First; down projects stay out of the ordinary TUI-style active list, without making kept data appear deleted |
| NAV-02 | First; touch navigation and browser history must not discard drafts |
| SES-03 | First; interrupt, stop, restart, and detach remain distinct |
| CHAT-01 | First; stream updates without duplicating events |
| CHAT-02 | First; copy source code without display padding; touch selection may differ from TUI drag |
| CHAT-03 | First; tap opens details instead of desktop double-click |
| CHAT-04 | First; respect server retention and render a bounded window |
| CHAT-05 | First; provider-reported fields only, missing data is not zero; auxiliary usage stays separate |
| INPUT-01 | First; preserve draft on disconnect/error; paste and dictation must not send |
| INPUT-02 | First for supported actions; equivalent buttons are allowed, unsupported actions must be explicit |
| INPUT-06 | First as standard browser input; no cxz STT engine or custom microphone UI planned |
| AUTH-01 | First via VPN + HTTPS; browser login/session mechanism still needs design, VPN membership alone is not an application login |
| APPROVAL-01 | First; bind decisions to session/run/request; stale or already-resolved replies must not apply elsewhere |
| APPROVAL-02 | First; cancellation is not denial; retain unsent answers during reconnect/navigation |
| APPROVAL-03 | First; policy remains server-owned and active while clients are closed |
| APPROVAL-04 | First; preserve supported request semantics and FULL behavior; unknown forms must not become implicit approval |
| OPS-01 | First read-only; web asset version must be distinguished from Manager/runtime version |
| WEB-01 | First; VPN-only deployment, transport/auth design pending |
| WEB-02 | First; page suspension/disconnection must not affect agent lifetime |
| WEB-03 | First; do not blindly resend mutations or assume a disconnected send failed |
| WEB-04 | First; no keyboard-only actions in the first-release scope |

### Follow-up scope

| Feature ID | Proposal and reason |
|---|---|
| SES-01 | Later; initially use sessions/accounts prepared on desktop |
| SES-02 | Later |
| SES-04 | Later; preview scope and retain existing confirmation semantics |
| PRJ-01 | Later; show workspace/container/data consequences before mutation |
| CHAT-06 | Later; preserve provider availability and idle requirements |
| CHAT-07 | Later; not journal retention or shared-memory compaction |
| CHAT-08 | Later; catalog-driven choices, report unsupported capabilities |
| CHAT-09 | Later |
| INPUT-03 | Later; preserve original content and insertion location, do not silently truncate |
| INPUT-04 | Later; paths refer to the selected project container |
| AUTH-02 | Later; show authentication that the selected session actually needs |
| MEM-01 | Later; distinguish another session's memory, own memory and frozen copies |
| MEM-02 | Later; preserve source ownership and existing confirmation semantics |
| MEM-03 | Later; distinguish native files from shared cxz memory |
| MCP-01 | Later; project override never changes another project |
| MCP-02 | Later |
| SKILL-01 | Later; host-side skill files remain host-managed |
| AI-01 | Later; reuse per-account auxiliary auth, no silent account/model fallback |
| AI-02 | Later; retain session on/off and one-shot semantics; accept suggestion into draft without sending |
| AI-03 | Later; cancel derived work without interrupting the agent |
| OPS-02 | Later; web can choose its rendering window but cannot override server retention silently |
| OPS-03 | Later; separate read-only status from disruptive maintenance |
| OPS-06 | Later; bounded, scrollable output with source identity |
| WEB-05 | Later; permission, subscription lifecycle and notification content need design; not required for initial foreground web use |

### Browser adaptation required

| Feature ID | Proposal and reason |
|---|---|
| FILE-01 | Adapt; browser file picker replaces arbitrary client filesystem paths; attach to the selected project |
| FILE-02 | Adapt; browser download behavior replaces the CLI's local Downloads directory contract |
| INPUT-05 | Adapt; current local/SSH transport rules do not establish a browser implementation; do not substitute a normal text field |
| OPS-07 | Adapt; current TUI local Docker access does not provide a browser terminal transport |

### Host responsibilities and shared behavior

| Feature ID | Proposal and reason |
|---|---|
| PRJ-02 | Host; Later for read-only inspection of effective configuration |
| MEM-04 | Shared agent behavior; no separate AI compact UI planned ([#36](https://github.com/lesomnus/cxz/issues/36) closed) |
| OPS-04 | Host for installation/replacement; Later for explicitly scoped Manager operations |
| OPS-05 | Host; browser must not pretend mobile files are Manager-host configuration |
| OPS-08 | Host; a web diagnostic facility would be a separate capability |
| OPS-09 | Host; not part of the initial mobile application |

### Deferred connection design

| Feature ID | Proposal and reason |
|---|---|
| WEB-06 | Deferred design; do not infer support from existing desktop named connections |

## Design decisions before implementation

- **Access (AUTH-01, WEB-01):** define the authenticated HTTPS API boundary,
  VPN address/certificate setup, browser session expiry/logout and authorization
  for mutations. Do not expose the existing plaintext TCP endpoint as a website.
- **Recovery (WEB-02, WEB-03):** define event cursor/gap handling and mutation
  identity/reconciliation after a lost response. Coordinate with concurrent TUI
  actions; reconnect must not duplicate a message or answer an obsolete request.
- **Drafts (INPUT-01, INPUT-05, WEB-04):** choose in-memory versus persisted drafts,
  reload/update behavior and secret exclusions. Preserve ordinary drafts during
  navigation and reconnect; do not assume mobile background execution continues.
- **Adapted surfaces (FILE-01, FILE-02, INPUT-05, OPS-07):** design file selection,
  browser downloads, protected secret transport and shell transport separately.
  A chat API does not establish any of these features automatically.
- **Notifications (WEB-05):** later define subscriptions, permission flow and
  notification content. A push subscription is not an open event connection.
- **Rollout (OPS-01):** distinguish web asset version from Manager/runtime version;
  define incompatible-client handling without discarding drafts.

## Acceptance for the first release

Agree the proposed First rows, then map each to implementation issues and tests.
Use an actual target phone and the TUI on the same Manager:

1. Read the same session in both clients and observe convergent messages, state
   and pending approvals. A decision made in either client is reflected in both.
2. Suspend/disconnect during streaming and resume without duplicate output. A
   pruned cursor produces an explicit history-gap recovery path.
3. Lose the response to a send or approval and recover without duplicate work or
   applying a decision to a different run.
4. Switch sessions, encounter a transient error and reload/update the page under
   the chosen draft policy; ordinary drafts survive and secrets are excluded.
5. Inspect tool input/output and approval payloads, copy code without display
   padding, and keep missing metrics visibly unavailable.
6. Complete all First-scope actions using touch and the soft keyboard. Unsupported
   Later/Adapt/Host actions must not silently call an unrelated fallback.

File transfers, secret input, terminal access and notifications get separate
acceptance scenarios when scheduled. A generic chat smoke test does not establish
parity for them. Record implementation evidence in the support matrix, not by
turning this plan into another support checklist.
