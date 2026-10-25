package websandbox

import (
	"context"
	"github.com/lesomnus/cxz/internal/projectid"
	"google.golang.org/grpc"
	"testing"
	"time"

	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func TestProjectMetadataEdits(t *testing.T) {
	s := New(1, time.Millisecond)
	defer s.Close()
	x := &Projects{S: s}
	original := proto.Clone(s.projects[0]).(*resource.Project)
	ref := resource.ProjectRef_builder{Id: original.GetId()}.Build()
	updated, err := x.Patch(t.Context(), resource.ProjectPatchRequest_builder{Ref: ref, DateUpdated: original.GetDateUpdated(), Name: proto.String("New workspace"), Alias: proto.String("New-Alias"), Desc: proto.String("Details")}.Build())
	if err != nil {
		t.Fatal(err)
	}
	if updated.GetName() != "New workspace" || updated.GetAlias() != "new-alias" || updated.GetDesc() != "Details" {
		t.Fatal(updated)
	}
	if string(updated.GetId()) != string(original.GetId()) || projectid.String(updated.GetId()) != projectid.String(original.GetId()) || updated.GetWorkspace() != original.GetWorkspace() || !proto.Equal(updated.GetStatus(), original.GetStatus()) {
		t.Fatal("metadata edit changed project identity or runtime")
	}
	if s.projectRev != 1 {
		t.Fatal("project watchers were not notified")
	}
	cleared, err := x.Patch(t.Context(), resource.ProjectPatchRequest_builder{Ref: ref, DateUpdated: updated.GetDateUpdated(), Desc: proto.String("")}.Build())
	if err != nil || cleared.GetDesc() != "" || cleared.GetName() != updated.GetName() || cleared.GetAlias() != updated.GetAlias() {
		t.Fatal("description clear lost other fields", err)
	}
	_, err = x.Patch(t.Context(), resource.ProjectPatchRequest_builder{Ref: ref, DateUpdated: original.GetDateUpdated(), Name: proto.String("Stale")}.Build())
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatal("stale version accepted", err)
	}
	for _, r := range []*resource.ProjectPatchRequest{
		resource.ProjectPatchRequest_builder{Ref: ref, DateUpdated: cleared.GetDateUpdated(), Name: proto.String("Not applied"), Alias: proto.String("project-2")}.Build(),
		resource.ProjectPatchRequest_builder{Ref: ref, DateUpdated: cleared.GetDateUpdated(), Name: proto.String("bad\nname")}.Build(),
		resource.ProjectPatchRequest_builder{Ref: ref, DateUpdated: cleared.GetDateUpdated(), Alias: proto.String("bad_alias")}.Build(),
		resource.ProjectPatchRequest_builder{Ref: ref, DateUpdated: cleared.GetDateUpdated(), Name: proto.String("Not applied"), Config: proto.String("/other")}.Build(),
	} {
		if _, err = x.Patch(t.Context(), r); err == nil {
			t.Fatal("invalid update accepted")
		}
		actual, _ := x.Get(t.Context(), resource.ProjectGetRequest_builder{Ref: ref}.Build())
		if !proto.Equal(cleared, actual) {
			t.Fatal("rejected update changed the project")
		}
	}
}

type projectWatchStream struct {
	grpc.ServerStream
	ctx     context.Context
	replies chan *resource.ProjectWatchResponse
}

func (s *projectWatchStream) Context() context.Context { return s.ctx }
func (s *projectWatchStream) Send(r *resource.ProjectWatchResponse) error {
	select {
	case s.replies <- r:
		return nil
	case <-s.ctx.Done():
		return s.ctx.Err()
	}
}
func TestProjectMetadataWatch(t *testing.T) {
	s := New(1, time.Millisecond)
	defer s.Close()
	x := &Projects{S: s}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	stream := &projectWatchStream{ctx: ctx, replies: make(chan *resource.ProjectWatchResponse, 2)}
	done := make(chan error, 1)
	go func() { done <- x.Watch(resource.ProjectWatchRequest_builder{}.Build(), stream) }()
	receive := func() *resource.ProjectWatchResponse {
		t.Helper()
		select {
		case r := <-stream.replies:
			return r
		case <-ctx.Done():
			t.Fatal("project watch did not deliver", ctx.Err())
			return nil
		}
	}
	initial := receive()
	if len(initial.GetItems()) != 2 {
		t.Fatal("incomplete initial snapshot")
	}
	p := initial.GetItems()[0].GetValue()
	_, err := x.Patch(t.Context(), resource.ProjectPatchRequest_builder{Ref: resource.ProjectRef_builder{Id: p.GetId()}.Build(), DateUpdated: p.GetDateUpdated(), Name: proto.String("Watch updated")}.Build())
	if err != nil {
		t.Fatal(err)
	}
	next := receive()
	if len(next.GetItems()) != 2 || next.GetItems()[0].GetValue().GetName() != "Watch updated" {
		t.Fatal("watch lost metadata change")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("watch did not stop")
	}
}
