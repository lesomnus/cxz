# Session attention and drafts

The web app marks unread completed responses with a `+` on the session row.
The browser tab title counts sessions needing attention, and its icon receives
a quiet badge. New questions also contribute while their session is not being
viewed. These indicators use no OS notification permission, prompt or modal.

Completion is confirmed by a successful native `turn_end`, including fast turns
coalesced into a single idle resource update. Errors, interruptions and unrelated
settings or usage records do not count. The existing shared resource inventory
drives checks; the app does not add a live event subscription per session.
History probes have bounded concurrency and cancellation. Initial snapshots
establish a baseline instead of replaying historical sounds.

An open, focused conversation acknowledges replies only when its latest history
is visible. Reading older messages, switching away, hiding the tab or focusing
the workspace terminal leaves the new reply unread. Read and notification cursors
are tab-local, survive refresh and deduplicate reconnects. Purge clears that
session's attention state, and sign-out removes the saved state.

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

Implementation: `features/session/model/session-attention.ts`,
`use-session-attention.ts`, `session-drafts.ts`, and
`shared/notifications`. Storybook's **Sessions/Panel/UnreadCompleted** shows the
additional row marker. Unit tests cover baseline/deduplication, scoped recovery,
completion filtering, cancellation and interrupted attachments. Browser tests
exercise audio, tab/read markers, paste recovery and intermediate composer heights.
