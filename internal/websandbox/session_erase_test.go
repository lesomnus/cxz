package websandbox

import (
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"testing"
	"time"
)

func TestEraseOnlyDeletesOneSandboxSession(t *testing.T) {
	s := New(42, time.Millisecond)
	defer s.Close()
	x := &Sessions{S: s}
	other := len(history(t, x, "session-2"))
	// Relative to what the fixture seeds, so adding a scenario is not a failing
	// test.
	seeded := len(s.sessions)
	deleted, err := x.Erase(t.Context(), ref("session-1"))
	if err != nil || !deleted.GetErased() || len(s.sessions) != seeded-1 {
		t.Fatal(deleted, err)
	}
	again, err := x.Erase(t.Context(), ref("session-1"))
	if err != nil || again.GetErased() {
		t.Fatal("erase is not idempotent", again, err)
	}
	_, err = x.Get(t.Context(), resource.SessionGetRequest_builder{Ref: ref("session-1")}.Build())
	if status.Code(err) != codes.NotFound {
		t.Fatalf("purged session remains accessible: %v", err)
	}
	if len(history(t, x, "session-2")) != other || len(s.projects) != 2 {
		t.Fatal("purge affected other resources")
	}
}
