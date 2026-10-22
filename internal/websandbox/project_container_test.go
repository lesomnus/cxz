package websandbox

import (
	"testing"
	"time"

	"github.com/lesomnus/cxz/resource"
	"google.golang.org/protobuf/proto"
)

func TestProjectDownRetainsListingAndConversation(t *testing.T) {
	s := New(1, time.Millisecond)
	defer s.Close()
	x := &Projects{S: s}
	before := proto.Clone(s.projects[0]).(*resource.Project)
	events := history(t, &Sessions{S: s}, "session-1")
	p, err := x.StopContainer(t.Context(), resource.ProjectControl_builder{Ref: resource.ProjectRef_builder{Id: before.GetId()}.Build(), ClientId: proto.String("stop")}.Build())
	if err != nil {
		t.Fatal(err)
	}
	if !p.GetListed() || p.GetStatus().GetContainerState() != resource.ContainerState_CONTAINER_STATE_STOPPED || string(p.GetId()) != string(before.GetId()) || p.GetAlias() != before.GetAlias() || p.GetName() != before.GetName() {
		t.Fatal("stop changed project identity/listing", p)
	}
	if len(s.projects) != 2 || len(s.sessions) != 9 || s.projects[1].GetStatus().GetContainerState() != resource.ContainerState_CONTAINER_STATE_RUNNING {
		t.Fatal("stop removed records or affected another project")
	}
	for _, st := range s.sessions {
		if st.value.GetProject().GetRuntimeId() == p.GetRuntimeId() && (!st.value.GetListed() || st.value.GetStatus().GetState() != "stopped") {
			t.Fatal("session unlisted or not stopped", st.value)
		}
	}
	after := history(t, &Sessions{S: s}, "session-1")
	if len(after) < len(events) {
		t.Fatal("conversation lost")
	}
	for i, e := range events {
		if !proto.Equal(e, after[i]) {
			t.Fatal("previous event changed")
		}
	}
	if s.projectRev != 1 {
		t.Fatal("project watch not notified")
	}
}
