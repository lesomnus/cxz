package websandbox

import (
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"testing"
	"time"
)

func TestPurgePlanAndExecutionStayInOneSandboxSession(t *testing.T) {
	s := New(42, time.Millisecond)
	defer s.Close()
	x := &Sessions{S: s}
	before := len(history(t, x, "session-1"))
	other := len(history(t, x, "session-2"))
	request := resource.SessionPurgeRequest_builder{Ref: ref("session-1"), DryRun: proto.Bool(true)}.Build()
	plan, err := x.Purge(t.Context(), request)
	if err != nil || !plan.GetDryRun() || len(plan.GetTargets()) != 2 || plan.GetTargets()[0].GetBytes() <= 0 {
		t.Fatalf("plan: %v %v", plan, err)
	}
	if len(history(t, x, "session-1")) != before || len(s.sessions) != 8 {
		t.Fatal("dry run mutated sandbox")
	}
	request.SetDryRun(false)
	deleted, err := x.Purge(t.Context(), request)
	if err != nil || deleted.GetDryRun() || len(s.sessions) != 7 {
		t.Fatalf("purge: %v %v", deleted, err)
	}
	_, err = x.Get(t.Context(), resource.SessionGetRequest_builder{Ref: ref("session-1")}.Build())
	if status.Code(err) != codes.NotFound {
		t.Fatalf("purged session remains accessible: %v", err)
	}
	if len(history(t, x, "session-2")) != other || len(s.projects) != 2 {
		t.Fatal("purge affected other resources")
	}
}
