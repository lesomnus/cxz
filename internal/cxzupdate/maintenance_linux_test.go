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
