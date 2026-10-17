package cxzupdate

import (
	"context"
	"fmt"
	"github.com/lesomnus/cxz/internal/core"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGateDrainsRequestsAndRejectsLaterMutations(t *testing.T) {
	root := t.TempDir()
	if e := os.MkdirAll(filepath.Join(root, "run"), 0700); e != nil {
		t.Fatal(e)
	}
	g := NewGate(root)
	id := core.ID()
	entered, done := make(chan struct{}), make(chan struct{})
	go func() {
		_, _ = g.Unary(context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: "/sessions/Send"}, func(context.Context, any) (any, error) { close(entered); <-done; return nil, nil })
	}()
	<-entered
	held := make(chan error, 1)
	go func() { held <- g.hold(id, func() error { return nil }) }()
	select {
	case <-held:
		t.Fatal("lease did not drain in-flight request")
	case <-time.After(30 * time.Millisecond):
	}
	close(done)
	if e := <-held; e != nil {
		t.Fatal(e)
	}
	called := false
	next := func(context.Context, any) (any, error) { called = true; return nil, nil }
	_, e := g.Unary(context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: "/sessions/Send"}, next)
	if status.Code(e) != codes.Unavailable || called {
		t.Fatal("mutation crossed lease")
	}
	if _, e = g.Unary(context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: "/sessions/Get"}, next); e != nil || !called {
		t.Fatal("read blocked")
	}
	if g.Run(func() { t.Error("internal mutation crossed lease") }) {
		t.Fatal("internal mutation allowed")
	}
	if e = g.release(core.ID()); e == nil {
		t.Fatal("foreign release accepted")
	}
	if !NewGate(root).Busy() {
		t.Fatal("lease did not survive restart")
	}
	if e = g.release(id); e != nil {
		t.Fatal(e)
	}
	if NewGate(root).Busy() {
		t.Fatal("release not persisted")
	}
}
func TestGateExpiredAndCorruptLeases(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "run"), 0700)
	path := filepath.Join(root, "run", "update-lease.json")
	id := core.ID()
	core.WriteJSON(path, lease{ID: id, Until: time.Now().Add(-time.Second)})
	if NewGate(root).Busy() || leaseOwned(root, id) {
		t.Fatal("expired lease considered owned")
	}
	os.WriteFile(path, []byte("broken"), 0600)
	g := NewGate(root)
	if !g.Busy() {
		t.Fatal("corrupt lease failed open")
	}
	if e := g.hold(id, func() error { return nil }); e == nil {
		t.Fatal("overwrote corrupt lease")
	}
}
func TestGateBusyCheckDoesNotAdmitLease(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "run"), 0700)
	g := NewGate(root)
	if e := g.hold(core.ID(), func() error { return fmt.Errorf("working") }); e == nil || g.Busy() {
		t.Fatal("busy session acquired lease")
	}
}
func TestValidSessionBinaryDoesNotCollideWithToolsExecutable(t *testing.T) {
	r := testRelease(nil)
	path := Path("/cxz/tools/cxz-builds", r, "linux/amd64")
	if !ValidBinary(path) || strings.HasPrefix(path, "/cxz/tools/cxz/") {
		t.Fatal(path)
	}
	for _, p := range []string{path + "/../cxz", "/tmp/cxz", "/cxz/tools/cxz", strings.Replace(path, r.Revision, "..", 1)} {
		if ValidSessionBinary("/state", p) {
			t.Fatal(p)
		}
	}
}

// An update refuses input, not reading. A conversation stays legible while its
// runtime is replaced, and the sweep that decides whether an update may start
// asks every session -- so anything misfiled as a mutation is refused for as
// long as that takes. See cxz#204.
func TestHeldGateRefusesInputAndStillAnswersReads(t *testing.T) {
	root := t.TempDir()
	if e := os.MkdirAll(filepath.Join(root, "run"), 0700); e != nil {
		t.Fatal(e)
	}
	g := NewGate(root)
	if e := g.hold(core.ID(), func() error { return nil }); e != nil {
		t.Fatal(e)
	}
	reads := []string{
		"Get", "List", "Watch", "Events", "History", "Projects",
		"Transcript", "Rows", "EventDetails", "Search", "Background",
		"GetHistoryFloor", "GetHistoryPolicy", "Models",
		"AuxStatus", "AuxEvents",
	}
	for _, name := range reads {
		served := false
		_, e := g.Unary(context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: "/cxz.SessionService/" + name}, func(context.Context, any) (any, error) {
			served = true
			return nil, nil
		})
		if e != nil || !served {
			t.Fatalf("%s was refused during an update: %v", name, e)
		}
	}
	for _, name := range []string{"Send", "Reply", "Resume", "Attach", "PutSecretFile"} {
		if _, e := g.Unary(context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: "/cxz.SessionService/" + name}, func(context.Context, any) (any, error) {
			t.Fatalf("%s was accepted during an update", name)
			return nil, nil
		}); status.Code(e) != codes.Unavailable {
			t.Fatalf("%s was not refused: %v", name, e)
		}
	}
}

// The lease is taken before the readiness sweep runs, so input is refused
// while it runs rather than queued behind it. Holding the gate's lock across
// the sweep stopped the server for as long as the slowest session took to
// answer, every time the rollout ticked.
func TestLeaseIsHeldWhileReadinessIsChecked(t *testing.T) {
	root := t.TempDir()
	if e := os.MkdirAll(filepath.Join(root, "run"), 0700); e != nil {
		t.Fatal(e)
	}
	g := NewGate(root)
	id := core.ID()
	refused := make(chan error, 1)
	if e := g.hold(id, func() error {
		// Mid-sweep: an input must be answered, and the answer must be no.
		_, e := g.Unary(context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: "/cxz.SessionService/Send"}, func(context.Context, any) (any, error) {
			return nil, fmt.Errorf("input was accepted during the sweep")
		})
		refused <- e
		return nil
	}); e != nil {
		t.Fatal(e)
	}
	if e := <-refused; status.Code(e) != codes.Unavailable {
		t.Fatal("input was not refused while the sweep ran:", e)
	}

	// A sweep that says no gives the lease back, so the next attempt is not
	// told that another update owns the server.
	if e := g.release(id); e != nil {
		t.Fatal(e)
	}
	next := core.ID()
	want := fmt.Errorf("session s: mid-turn")
	if e := g.hold(next, func() error { return want }); e == nil || !strings.Contains(e.Error(), "mid-turn") {
		t.Fatal("the sweep's reason was lost:", e)
	}
	if g.Busy() {
		t.Fatal("a refused update kept the server held")
	}
	if _, e := os.Stat(filepath.Join(root, "run", "update-lease.json")); !os.IsNotExist(e) {
		t.Fatal("the lease file outlived the refused update")
	}
}
