package websandbox

import (
	"testing"
	"time"

	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func TestProjectRemoveRequiresConfirmationAndPreservesOtherProjects(t *testing.T) {
	s := New(1, time.Millisecond)
	defer s.Close()
	x := &Projects{S: s}
	other := proto.Clone(s.projects[1]).(*resource.Project)
	r := resource.ProjectRemoveRequest_builder{Target: proto.String("project-1")}.Build()
	preview, err := x.Remove(t.Context(), r)
	if err != nil || preview.GetRemoved() || len(s.projects) != 2 || len(s.sessions) != 9 {
		t.Fatal("preview changed fixtures", err)
	}
	r.SetConfirmed(true)
	removed, err := x.Remove(t.Context(), r)
	if err != nil || !removed.GetRemoved() || !proto.Equal(removed.GetProject(), preview.GetProject()) {
		t.Fatal("remove did not confirm target", err)
	}
	if len(s.projects) != 1 || !proto.Equal(s.projects[0], other) || len(s.sessions) != 1 || s.sessions[0].value.GetProject().GetRuntimeId() != "project-2" {
		t.Fatal("remove affected another project")
	}
	if s.projectRev != 1 {
		t.Fatal("project watch was not notified")
	}
	if _, err = x.Remove(t.Context(), r); status.Code(err) != codes.NotFound {
		t.Fatal("removed project remains accessible", err)
	}
}
