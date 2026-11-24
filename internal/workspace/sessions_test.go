package workspace

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lesomnus/cxz/api"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// sweepRuntime answers List the way a project runtime might.
type sweepRuntime struct {
	api.SessionsClient
	delay  time.Duration
	err    error
	states []string
	calls  atomic.Int32
}

func (r *sweepRuntime) List(ctx context.Context, _ *api.Empty, _ ...grpc.CallOption) (*api.SessionList, error) {
	r.calls.Add(1)
	if r.delay > 0 {
		select {
		case <-time.After(r.delay):
		case <-ctx.Done():
			return nil, status.Error(codes.DeadlineExceeded, ctx.Err().Error())
		}
	}
	if r.err != nil {
		return nil, r.err
	}
	out := &api.SessionList{}
	for i, state := range r.states {
		out.Sessions = append(out.Sessions, &api.Session{Id: strings.Repeat(string(rune('a'+i)), 24), State: state})
	}
	return out, nil
}

func sweepManager(t *testing.T, runtimes map[string]*sweepRuntime) *Manager {
	t.Helper()
	db, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err = db.Exec(`CREATE TABLE projects(id TEXT PRIMARY KEY,data BLOB NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	m := &Manager{DB: db, Root: t.TempDir(), Owner: strings.Repeat("o", 24)}
	m.sweepClient = func(_ context.Context, p *Project) (*grpc.ClientConn, api.SessionsClient, error) {
		r := runtimes[p.ID]
		if r == nil {
			return nil, nil, errors.New("project is stopped; run cxz project up")
		}
		return nil, r, nil
	}
	return m
}

func states(list *api.SessionList) map[string]string {
	out := map[string]string{}
	for _, s := range list.Sessions {
		out[s.ProjectId+"/"+s.Id[:1]] = s.State
	}
	return out
}

func TestSweepKeepsSlowRuntimeStatesWithinGrace(t *testing.T) {
	defer func(timeout, grace time.Duration) { sessionSweepTimeout, slowRuntimeGrace = timeout, grace }(sessionSweepTimeout, slowRuntimeGrace)
	sessionSweepTimeout, slowRuntimeGrace = 100*time.Millisecond, time.Hour
	ctx := context.Background()
	healthy := &sweepRuntime{states: []string{"idle", "working"}}
	slow := &sweepRuntime{states: []string{"idle", "stopped"}}
	m := sweepManager(t, map[string]*sweepRuntime{"fast": healthy, "slow": slow})
	for _, id := range []string{"fast", "slow", "gone"} {
		if err := m.save(ctx, &Project{ID: id, Workspace: "/" + id, Sessions: []*api.Session{{Id: strings.Repeat("a", 24), ProjectId: id, State: "idle"}}}); err != nil {
			t.Fatal(err)
		}
	}
	// Learn the slow project's sessions while it still answers.
	if _, err := m.Sessions(ctx); err != nil {
		t.Fatal(err)
	}
	slow.delay = time.Second
	start := time.Now()
	list, err := m.Sessions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// One slow project costs one deadline, not one per project.
	if elapsed := time.Since(start); elapsed > 600*time.Millisecond {
		t.Fatalf("sweep took %s", elapsed)
	}
	got := states(list)
	if got["slow/a"] != "idle" || got["slow/b"] != "stopped" {
		t.Fatalf("a slow runtime's sessions were reported as stopped: %v", got)
	}
	if got["fast/a"] != "idle" || got["fast/b"] != "working" {
		t.Fatalf("healthy project: %v", got)
	}
	// A project whose container is not running is not merely slow.
	if got["gone/a"] != "interrupted" {
		t.Fatalf("unreachable project: %v", got)
	}
}

func TestSweepInterruptsRuntimeSilentPastGrace(t *testing.T) {
	defer func(timeout, grace time.Duration) { sessionSweepTimeout, slowRuntimeGrace = timeout, grace }(sessionSweepTimeout, slowRuntimeGrace)
	sessionSweepTimeout, slowRuntimeGrace = 50*time.Millisecond, 0
	ctx := context.Background()
	slow := &sweepRuntime{states: []string{"idle"}, delay: time.Second}
	m := sweepManager(t, map[string]*sweepRuntime{"slow": slow})
	if err := m.save(ctx, &Project{ID: "slow", Workspace: "/slow", Sessions: []*api.Session{{Id: strings.Repeat("a", 24), ProjectId: "slow", State: "working", Pending: []*api.Event{{Kind: "question"}}}}}); err != nil {
		t.Fatal(err)
	}
	list, err := m.Sessions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if s := list.Sessions[0]; s.State != "interrupted" || len(s.Pending) != 0 {
		t.Fatalf("a runtime silent past its grace kept its sessions running: %v", s)
	}
	// Answering again clears the record of slowness.
	slow.delay = 0
	if list, err = m.Sessions(ctx); err != nil || list.Sessions[0].State != "idle" {
		t.Fatal(list, err)
	}
	if _, stale := m.slowSince["slow"]; stale {
		t.Fatal("recovered runtime still recorded as slow")
	}
}

func TestSweepRefusedRuntimeIsInterrupted(t *testing.T) {
	ctx := context.Background()
	refused := &sweepRuntime{err: status.Error(codes.Unavailable, "connection refused")}
	m := sweepManager(t, map[string]*sweepRuntime{"p": refused})
	if err := m.save(ctx, &Project{ID: "p", Workspace: "/p", Sessions: []*api.Session{{Id: strings.Repeat("a", 24), ProjectId: "p", State: "idle"}}}); err != nil {
		t.Fatal(err)
	}
	list, err := m.Sessions(ctx)
	if err != nil || list.Sessions[0].State != "interrupted" {
		t.Fatal(list, err)
	}
}
