package cxzupdate

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A forced rollout still closes admission -- a replacement must not run beside
// ordinary work -- but the readiness sweep stops being a veto. Without this the
// only way past a session that never looks quiet was to stop that session,
// which is a strange thing to have to do to install an update. See cxz#253.
func TestAForcedPrepareDoesNotNeedTheSweepsConsent(t *testing.T) {
	root := t.TempDir()
	if e := os.MkdirAll(filepath.Join(root, "run"), 0700); e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	g := NewGate(root)
	refuse := func(context.Context) (map[string]string, error) {
		return map[string]string{"s": "run"}, fmt.Errorf("session s: waiting for 5 minutes of continuous idle")
	}
	server, e := Maintenance(ctx, root, g, refuse)
	if e != nil {
		t.Skip("maintenance socket unavailable here:", e)
	}
	defer server.Close()

	// What the rollout sees today: refused, and told why.
	h, e := Call(ctx, root, "status", "")
	if e != nil {
		t.Fatal(e)
	}
	if h.Ready || h.Reason == "" {
		t.Fatalf("a refusing sweep reported ready: %+v", h)
	}
	id := "f0rced0000000000000000aa"
	if _, e = Call(ctx, root, "prepare", id); e == nil {
		t.Fatal("an ordinary prepare ignored the sweep")
	}
	if g.Busy() {
		t.Fatal("a refused prepare kept admission closed")
	}

	h, e = Call(ctx, root, "prepare-force", id)
	if e != nil {
		t.Fatal("the forced prepare was refused:", e)
	}
	if !h.Ready || h.Reason != "forced" {
		t.Fatalf("the forced prepare did not say so: %+v", h)
	}
	if h.Lease != id || !g.Busy() {
		t.Fatalf("the forced rollout did not hold admission: %+v", h)
	}
	// The sweep still runs, because its session list is how the rollout knows
	// which runs it is replacing.
	if h.Sessions["s"] != "run" {
		t.Fatalf("the session list was dropped: %+v", h.Sessions)
	}
	if _, e = Call(ctx, root, "release", id); e != nil {
		t.Fatal(e)
	}
	if g.Busy() {
		t.Fatal("admission stayed closed after release")
	}
}
