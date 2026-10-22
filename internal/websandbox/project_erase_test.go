package websandbox

import (
	"testing"
	"time"

	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func TestProjectEraseDeletesOnlyItsOwnFixtures(t *testing.T) {
	s := New(1, time.Millisecond)
	defer s.Close()
	x := &Projects{S: s}
	other := proto.Clone(s.projects[1]).(*resource.Project)
	target := resource.ProjectRef_builder{RuntimeId: proto.String("project-1")}.Build()
	erased, err := x.Erase(t.Context(), target)
	if err != nil || !erased.GetErased() {
		t.Fatal("erase did not report what it did", err)
	}
	if len(s.projects) != 1 || !proto.Equal(s.projects[0], other) || len(s.sessions) != 1 || s.sessions[0].value.GetProject().GetRuntimeId() != "project-2" {
		t.Fatal("erase affected another project")
	}
	if s.projectRev != 1 {
		t.Fatal("project watch was not notified")
	}
	if _, err = x.Erase(t.Context(), target); status.Code(err) != codes.NotFound {
		t.Fatal("erased project remains accessible", err)
	}
}
