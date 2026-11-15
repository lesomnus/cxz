package websandbox

import (
	"testing"
	"time"

	"github.com/lesomnus/cxz/resource"
	"google.golang.org/protobuf/proto"
)

func TestSessionAddThenStart(t *testing.T) {
	s := New(1, time.Millisecond)
	defer s.Close()
	x := s.Sessions()
	r := resource.SessionAddRequest_builder{Project: resource.ProjectRef_builder{Alias: proto.String("project-1")}.Build(), ClientId: "created-before-start", Name: "New conversation", Agent: "codex"}.Build()
	v, err := x.Add(t.Context(), r)
	if err != nil || v.GetStatus().GetState() != "created" || v.GetStatus().GetRunId() != "" || v.GetStatus().GetLastSeq() != 0 {
		t.Fatal(v, err)
	}
	again, err := x.Add(t.Context(), r)
	if err != nil || !proto.Equal(v, again) {
		t.Fatal("Add retry", again, err)
	}
	started, err := x.Start(t.Context(), resource.SessionControl_builder{Ref: ref(v.GetRuntimeId()), ClientId: proto.String("start")}.Build())
	if err != nil || started.GetStatus().GetState() != "idle" || started.GetStatus().GetRunId() == "" || !proto.Equal(v.GetProject(), started.GetProject()) {
		t.Fatal(started, err)
	}
}
