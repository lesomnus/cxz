package websandbox

import (
	"testing"
	"time"

	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func TestSessionIdentityEditsPreserveRunAndTranscript(t *testing.T) {
	s := New(1, time.Millisecond)
	defer s.Close()
	x := &Sessions{S: s}
	before, err := x.Get(t.Context(), resource.SessionGetRequest_builder{Ref: ref("session-1")}.Build())
	if err != nil {
		t.Fatal(err)
	}
	oldRev := s.rev
	_, err = x.Patch(t.Context(), resource.SessionPatchRequest_builder{Ref: ref("session-1"), Alias: proto.String("oak-tree")}.Build())
	if err != nil {
		t.Fatal(err)
	}
	_, err = x.Patch(t.Context(), resource.SessionPatchRequest_builder{Ref: ref("session-1"), Name: proto.String("  My new title  ")}.Build())
	if err != nil {
		t.Fatal(err)
	}
	after, err := x.Get(t.Context(), resource.SessionGetRequest_builder{Ref: resource.SessionRef_builder{Alias: proto.String("oak-tree")}.Build()}.Build())
	if err != nil {
		t.Fatal(err)
	}
	if after.GetName() != "My new title" || after.GetAlias() != "oak-tree" || !proto.Equal(before.GetStatus(), after.GetStatus()) || string(before.GetId()) != string(after.GetId()) || before.GetRuntimeId() != after.GetRuntimeId() || s.rev != oldRev+2 {
		t.Fatal("identity edit changed runtime/transcript or failed to notify watchers", after)
	}
	if _, err = x.Patch(t.Context(), resource.SessionPatchRequest_builder{Ref: ref("session-2"), Alias: proto.String("oak-tree")}.Build()); status.Code(err) != codes.AlreadyExists {
		t.Fatal("duplicate alias accepted", err)
	}
	if _, err = x.Patch(t.Context(), resource.SessionPatchRequest_builder{Ref: ref("session-2"), Alias: proto.String("Bad_Alias")}.Build()); status.Code(err) != codes.InvalidArgument {
		t.Fatal("invalid alias accepted", err)
	}
	if _, err = x.Patch(t.Context(), resource.SessionPatchRequest_builder{Ref: ref("session-2"), Name: proto.String("  `  `  ")}.Build()); status.Code(err) != codes.InvalidArgument {
		t.Fatal("empty normalized title accepted", err)
	}
	second, _ := x.Get(t.Context(), resource.SessionGetRequest_builder{Ref: ref("session-2")}.Build())
	if _, err = x.Patch(t.Context(), resource.SessionPatchRequest_builder{Ref: ref("session-2"), Name: proto.String("Invalid combined update"), Alias: proto.String("oak-tree")}.Build()); status.Code(err) != codes.AlreadyExists {
		t.Fatal("combined update accepted duplicate alias", err)
	}
	unchanged, _ := x.Get(t.Context(), resource.SessionGetRequest_builder{Ref: ref("session-2")}.Build())
	if !proto.Equal(second, unchanged) {
		t.Fatal("invalid combined update changed the title")
	}
	if _, err = x.Patch(t.Context(), resource.SessionPatchRequest_builder{Ref: ref("session-2"), Name: proto.String("Combined title"), Alias: proto.String("maple-tree")}.Build()); err != nil {
		t.Fatal("combined update rejected", err)
	}
	combined, _ := x.Get(t.Context(), resource.SessionGetRequest_builder{Ref: resource.SessionRef_builder{Alias: proto.String("maple-tree")}.Build()}.Build())
	if combined.GetName() != "Combined title" {
		t.Fatal("combined title lost", combined)
	}
}
