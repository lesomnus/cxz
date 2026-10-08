package eventwire

import (
	"github.com/lesomnus/cxz/api"
	"google.golang.org/protobuf/proto"
	"testing"
)

func TestStructuredFileSummaryRoundTrip(t *testing.T) {
	e := &api.Event{SessionId: "s", Seq: 1, Kind: "tool_call", RunId: "run", RequestId: "file", ToolSummary: &api.ToolSummary{Name: "Files", State: "completed", OmittedFiles: 2, Files: []*api.ToolFileSummary{{Path: "a", MovePath: "b", Action: "update", Added: 25, Removed: 11, Measure: "diff"}, {Path: "c", Action: "write", Lines: 4, Measure: "content", PerMatch: true}, {Path: "d", Action: "add", Measure: "unknown"}}}}
	if got := FromResource("s", ToResource(e)); !proto.Equal(got, e) {
		t.Fatal(got)
	}
}
