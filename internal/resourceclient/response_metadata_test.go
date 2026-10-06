package resourceclient

import (
	"github.com/lesomnus/cxz/resource"
	"testing"
)

func TestResponseSnapshotAdapter(t *testing.T) {
	r := resource.ResponseMetadata_builder{Model: "m", Effort: "high", ModelSource: "response", EffortSource: "requested", TurnId: "turn"}.Build()
	e := event("s", resource.SessionEvent_builder{Kind: "assistant", Response: r}.Build())
	if e.Response == nil || e.Response.Model != r.GetModel() || e.Response.Effort != r.GetEffort() || e.Response.ModelSource != r.GetModelSource() || e.Response.EffortSource != r.GetEffortSource() || e.Response.TurnId != r.GetTurnId() {
		t.Fatal(e)
	}
	if event("s", resource.SessionEvent_builder{Kind: "assistant"}.Build()).Response != nil {
		t.Fatal("legacy snapshot invented")
	}
}
