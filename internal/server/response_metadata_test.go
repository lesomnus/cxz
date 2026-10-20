package server

import (
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/eventwire"
	"testing"
)

func TestResponseSnapshotProjection(t *testing.T) {
	r := &core.ResponseMetadata{Model: "m", Effort: "high", ModelSource: "response", EffortSource: "requested", TurnID: "turn", Phase: "final_answer", CompletionJSON: `{"response_seq":"1"}`}
	e := eventwire.FromCore(core.Event{Kind: "assistant", Response: r})
	if e.Response == nil || e.Response.Model != r.Model || e.Response.Effort != r.Effort || e.Response.ModelSource != r.ModelSource || e.Response.EffortSource != r.EffortSource || e.Response.TurnId != r.TurnID || e.Response.Phase != r.Phase || string(e.Response.CompletionJson) != r.CompletionJSON {
		t.Fatal(e)
	}
	if eventwire.FromCore(core.Event{Kind: "assistant"}).Response != nil {
		t.Fatal("legacy snapshot invented")
	}
}
