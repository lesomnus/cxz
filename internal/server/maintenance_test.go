package server

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/lesomnus/cxz/internal/core"
)

// The readiness sweep asks every session whether an update may take it, and the
// caller gives up after thirty seconds. Asking them one after another made the
// sweep grow with the installation until it could not answer at all -- an owner
// with twenty-five sessions against a five-second probe each. See cxz#204.
//
// A session whose supervisor is not listening is not an answer, so the sweep
// says which one it was waiting for rather than letting its caller time out
// with nothing to report.
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
	start := time.Now()
	runs, err := s.quietForUpdate(context.Background())
	waited := time.Since(start)
	if err == nil {
		t.Fatal("a session whose supervisor never answered was reported as quiet", runs)
	}
	if !strings.Contains(err.Error(), "session "+m.ID) {
		t.Fatal("the sweep did not say which session it was waiting for:", err)
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
