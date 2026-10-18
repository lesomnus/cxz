package server

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/cxzupdate"
	"github.com/lesomnus/cxz/internal/supervisor"
)

// supervisorStub listens where a session's supervisor would and answers the
// readiness probe with what it is given, so that "it said no" can be told apart
// from "it was not there".
func supervisorStub(t *testing.T, root, id string, status supervisor.UpdateStatus) *http.Server {
	t.Helper()
	sock := core.Socket(root, id)
	if err := os.MkdirAll(filepath.Dir(sock), 0700); err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The sweep asks two things of a supervisor, and they are different
		// answers: what state it is in, then whether an update may take it.
		if strings.HasSuffix(r.URL.Path, "/update-status") {
			_ = json.NewEncoder(w).Encode(status)
			return
		}
		_ = json.NewEncoder(w).Encode(core.Snapshot{State: "idle", RunID: "run"})
	})}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	return srv
}

// The readiness sweep asks every session whether an update may take it, and the
// caller gives up after thirty seconds. Asking them one after another made the
// sweep grow with the installation until it could not answer at all -- an owner
// with twenty-five sessions against a five-second probe each. See cxz#204.
//
// A session whose supervisor answers "not ready" is named, so the caller knows
// what it waited for rather than timing out with nothing to report. A session
// whose supervisor is not listening at all is a different thing: no agent is
// running, so it is quiet -- counting it as busy blocked every update of an
// installation with no way to clear it (cxz#223).
func TestReadinessSweepIsBoundedAndNamesWhatItWaitedFor(t *testing.T) {
	s, m, l := projectionFixture(t)
	// A session the sweep has to ask about: stopped ones are quiet by
	// definition and cost nothing.
	if _, err := l.AppendBatch([]core.Event{{SessionID: m.ID, Kind: "state", Text: "idle", RunID: "run"}}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 24; i++ {
		addSession(t, s, core.Session{ID: strings.Repeat(string(rune('b'+i)), 24), Kind: "claude"})
	}
	// A supervisor that is there and says no. The sweep must name this one.
	busy := supervisorStub(t, s.root, m.ID, supervisor.UpdateStatus{Reason: "unfinished tool calls", Protocol: cxzupdate.Protocol})
	start := time.Now()
	runs, err := s.quietForUpdate(context.Background())
	waited := time.Since(start)
	if err == nil {
		t.Fatal("a supervisor that said it was not ready was reported as quiet", runs)
	}
	if !strings.Contains(err.Error(), "session "+m.ID) || !strings.Contains(err.Error(), "unfinished tool calls") {
		t.Fatal("the sweep did not say which session it was waiting for, or why:", err)
	}
	busy.Close()
	// With nothing listening, the same sweep is quiet: the twenty-four other
	// sessions never had a supervisor at all.
	if _, err = s.quietForUpdate(context.Background()); err != nil {
		t.Fatal("sessions with no supervisor blocked an update:", err)
	}
	// Bounded by the sweep itself rather than by sessions times a probe. The
	// probes here fail at the dial, so this mostly pins that nothing serialised
	// twenty-five five-second waits behind each other.
	if waited > quietSweep {
		t.Fatal("the sweep outlived its own bound:", waited)
	}
	if runs[m.ID] != "run" {
		t.Fatal("a session that was asked about lost its run", runs)
	}
}

// A stopped session has nothing to ask: no supervisor to probe and no turn to
// interrupt. That is what keeps a long-lived installation's sweep cheap.
func TestStoppedSessionsAreNotProbed(t *testing.T) {
	s, m, _ := projectionFixture(t)
	runs, err := s.quietForUpdate(context.Background())
	if err != nil {
		t.Fatal("a stopped session blocked an update:", err)
	}
	if len(runs) != 0 {
		t.Fatal("a stopped session was reported as a running one", runs)
	}
	_ = m
}

func addSession(t *testing.T, s *Server, m core.Session) {
	t.Helper()
	b, e := json.Marshal(m)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.db.Exec("INSERT OR REPLACE INTO sessions VALUES(?,?)", m.ID, b); e != nil {
		t.Fatal(e)
	}
}
