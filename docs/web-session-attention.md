# Session attention and drafts

The web app marks unread completed responses with a fluorescent circle in the
session row's indicator position. The Sessions icon shows a count badge.
The browser tab title counts sessions needing attention, and its icon receives
a quiet badge. New questions also contribute while their session is not being
viewed. These indicators use no OS notification permission, prompt or modal.

The Manager publishes Payday notifications for native `turn_end completed`,
`turn_end failed`, and actual user Questions. Tool failures, interruptions and
unrelated settings or usage records do not create notifications. Projects are
resource Topics; Sessions are sources. The app uses Payday's global tenancy
declaration because these resources have no Tenant.

An installation has a persistent Operator identity, independent of provider
Accounts. Authenticated Web and TUI clients use that same reader. The Manager
stores notification receipts and conversation read cursors in `resources.db`;
clearing browser storage, signing out, or restarting a client does not erase
them. The current installation authentication model has one human operator,
so connecting with another machine or transport does not create another reader.

The supervisor remains the authority for conversation events. A Manager worker
reads the committed conversation files, without opening an RPC stream for every
session. It resumes a durable ingestion checkpoint after restart, publishes with
an idempotency key, and advances the checkpoint only after publication succeeds.
One unavailable conversation does not block collection from other sessions.
Fork boundaries exclude copied historical turns. Existing history establishes
the initial deployment baseline; later offline completions are collected.

Clients subscribe to `NotificationService.Listen` before reconciling `Read`.
Listen carries readiness and invalidation hints, so reconnects always query the
durable state again. Unread pages determine markers, and a bounded recent window
also preserves sounds for live completions that were immediately read. Initial
and reconnect snapshots are silent. No completion History probes or tab-local
read-state storage remain. The UI keeps only current unread and recent sound
deduplication state in memory.

An open, focused conversation acknowledges replies only when its latest history
is visible. Reading older messages, switching away, hiding the tab or focusing
the workspace terminal leaves the new reply unread. `SessionService.ReadState`
identifies the conversation generation; `MarkRead` records the range actually
shown, rejects stale generations and future sequences, and never moves a read
cursor backwards. A completion published after its range was read receives a
receipt too. These rules close the read/publication race and synchronize other
windows and the TUI. Purge removes that session's read and notification records.

The reading tab removes matching unread markers immediately, before ReadState
or MarkRead answers. This optimistic projection remains during navigation and
transient retries; other clients still follow committed server receipts. A
rejected read restores the markers without retrying indefinitely. Reopening the
conversation permits another attempt. Projection is limited to the observed
generation and read range, so newer replies remain unread.

Both completion and a new Question play the TUI's local PCM cues, even when the
active conversation is visible. A click or keypress unlocks browser audio; alerts
that occurred before audio was available are not replayed later. Simultaneous
alerts use a short bounded playback queue. **General → Notifications →
Notification sounds** controls `notifications.sound` in the shared settings file.
Tab markers remain available with sound disabled. Browser autoplay restrictions
can still suppress playback.

Composer drafts use `sessionStorage`, scoped to the connection and session. This
keeps reload recovery within the tab and avoids one tab overwriting another's
editor. Plain Markdown, paste-chip source and completed attachment paths are
restored; uploaded files remain in the project attachment storage. File blobs
are not copied into browser storage. An interrupted file upload is shown as a
failed chip that must be removed and selected again, rather than silently sending
an empty file. Drafts are removed after an accepted send, on purge and on sign-out;
a rejected send preserves the draft. Restricted or full browser storage still
allows in-memory editing.

An expanded composer contracts smoothly after its accepted contents depart.
Reduced-motion preferences disable the height transition. Timing and geometry
tokens in the app stylesheet are the source of truth.
The transcript keeps following latest through those viewport changes, correcting
resize-generated scroll events before treating them as reading navigation.
Wheel, drag and keyboard navigation still allow reading older messages.
If new row measurements shorten the scroll range during a wheel gesture, native
position clamping preserves the remaining movement instead of resuming latest.

Implementation: `server/lifecycle/notifications.go`,
`internal/resourceclient/notifications.go`, `internal/tui/server_attention.go`,
`features/session/model/session-attention.ts`,
`use-session-attention.ts`, `session-drafts.ts`, and
`shared/notifications`. Storybook's **Sessions/Panel/UnreadCompleted** shows the
additional row marker. The sandbox implements the same RPCs in shared memory;
reloading the sandbox creates a new simulated server and reader. Tests cover
durable restoration, cross-client receipts, connection isolation, fork/reset
boundaries, late publication, idempotent catch-up, filtering and cancellation. Browser tests
exercise audio, tab/read markers, paste recovery and intermediate composer heights.
