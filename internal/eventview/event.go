// Package eventview is a journal record as a reader sees it.
//
// It is here rather than in eventwire because eventwire is reached from the
// WASM sandbox and this is not: a record comes from internal/core, which locks
// files and so cannot be built for js/wasm. The sandbox serves the reading
// projection from records it already has in the shape a reader wants, which is
// why it never needs this direction.
package eventview

import (
	"encoding/json"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/agentview"
	"github.com/lesomnus/cxz/internal/core"
)

// FromCore is a journal record as a reader sees it. Two kinds are rewritten
// rather than passed through, which is why this is a conversion and not a field
// copy: a history checkpoint becomes the boundary a client can show, and a raw
// background frame becomes the background event derived from it. It lives here
// so that the store a session writes and the server that reads it apply the
// same rules -- there is one view of a record, not one per caller.
func FromCore(e core.Event) *api.Event {
	if e.Kind == core.HistoryCheckpointKind {
		e.Kind = core.HistoryTrimmedKind
		e.Text = "Earlier display history was removed by the size limit."
		e.Payload, _ = json.Marshal(core.HistoryBoundary{Through: e.Seq})
	}

	if e.Kind == "raw" && agentview.IsBackgroundEvent(e.Raw) {
		e.Kind, e.Payload = "background", e.Raw
	}
	v := &api.Event{SessionId: e.SessionID, RunId: e.RunID, Seq: e.Seq, TimeMs: e.TimeMS, Kind: e.Kind, Text: e.Text, RequestId: e.RequestID, Payload: e.Payload}
	if r := e.Response; r != nil {
		v.Response = &api.ResponseMetadata{Model: r.Model, Effort: r.Effort, ModelSource: r.ModelSource, EffortSource: r.EffortSource, TurnId: r.TurnID, Phase: r.Phase, CompletionJson: []byte(r.CompletionJSON)}
	}
	return v
}
