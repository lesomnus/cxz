# TUI editor ownership

The conversation composer uses [`github.com/lesomnus/bed`](https://github.com/lesomnus/bed),
an independent Go module built on Bubble Tea's textarea. `go.mod` pins an exact
published commit; no local `replace` is required to build cxz.

bed owns text selection, rune/cell coordinate mapping, logical line layout,
visible-row Home/End, mouse hit testing and drag selection, whitespace display,
and scrollbar rendering. It does not import cxz or query sessions/RPCs.

cxz owns focus/modal routing, screen coordinates, session drafts, message sending,
slash commands, session/path completion, paste payloads/dialogs, and AI suggestion
fetching/display. The adapter in `internal/tui/composer_selection.go` supplies an
opaque document key and paste labels, translates mouse coordinates, and applies
cxz focus/resize policy. Clipboard requests arrive as `bed.CopyMsg` and use cxz's
existing clipboard transport.

The composer routes selection keys through `HandleKey`, then forwards remaining
input with `UpdateText`. Rendering decorates the embedded textarea in this order:
paste labels, logical gutter/whitespace, selection, scrollbar. This keeps current
chip and ghost presentation without embedding cxz behavior in the editor.

For a new independent TUI view, create `bed.New()`, own its focus/size, translate
mouse coordinates to the widget origin, and call `Update`/`View`. Intercept submit
keys in the parent. The bed repository's `examples/two-editors` demonstrates two
independent document editors.

Changes to editor mechanics belong in bed with its unit tests. Publish a commit
there, update the pinned dependency here, and run the TUI regression tests,
including composer selection/display, paste chips and session completion.
