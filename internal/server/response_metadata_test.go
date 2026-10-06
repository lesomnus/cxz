package server

import (
	"github.com/lesomnus/cxz/internal/core"
	"testing"
)

func TestResponseSnapshotProjection(t *testing.T) {
	r := &core.ResponseMetadata{Model: "m", Effort: "high", ModelSource: "response", EffortSource: "requested", TurnID: "turn"}
	e := pbEvent(core.Event{Kind: "assistant", Response: r})
	if e.Response == nil || e.Response.Model != r.Model || e.Response.Effort != r.Effort || e.Response.ModelSource != r.ModelSource || e.Response.EffortSource != r.EffortSource || e.Response.TurnId != r.TurnID {
		t.Fatal(e)
	}
	if pbEvent(core.Event{Kind: "assistant"}).Response != nil {
		t.Fatal("legacy snapshot invented")
	}
}
