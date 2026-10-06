package lifecycle

import (
	"github.com/lesomnus/cxz/api"
	"google.golang.org/protobuf/proto"
	"testing"
)

func TestResponseSnapshotResourceProjection(t *testing.T) {
	r := &api.ResponseMetadata{Model: "m", Effort: "high", ModelSource: "response", EffortSource: "requested", TurnId: "turn", Phase: "final_answer", CompletionJson: []byte(`{"response_seq":"1"}`)}
	v := event(&api.Event{Kind: "assistant", Response: r})
	// Exercise the wire encoding consumed by the web client.
	b, err := proto.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	v.Reset()
	if err := proto.Unmarshal(b, v); err != nil {
		t.Fatal(err)
	}
	x := v.GetResponse()
	if x == nil || x.GetModel() != r.Model || x.GetEffort() != r.Effort || x.GetModelSource() != r.ModelSource || x.GetEffortSource() != r.EffortSource || x.GetTurnId() != r.TurnId || x.GetPhase() != r.Phase || string(x.GetCompletionJson()) != string(r.CompletionJson) {
		t.Fatal(v)
	}
	if event(&api.Event{Kind: "assistant"}).GetResponse() != nil {
		t.Fatal("legacy snapshot invented")
	}
}
