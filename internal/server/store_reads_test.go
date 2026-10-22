package server

import (
	"context"
	"strings"
	"encoding/json"
	"testing"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/supervisor"
)

// A conversation served from the session's own store is the same conversation,
// and the server keeps nothing of its own: there is no table left for it to
// keep it in. See cxz#278.
func TestAPageComesFromTheSessionsStore(t *testing.T) {
	ctx := context.Background()
	s, m, l := projectionFixture(t)
	for _, e := range []core.Event{
		{SessionID: m.ID, RunID: "run", Kind: "input", Text: "build it"},
		{SessionID: m.ID, RunID: "run", Kind: "assistant", Text: "on it"},
		{SessionID: m.ID, RunID: "run", Kind: "models", Payload: json.RawMessage(`{"models":[{"id":"m1"}]}`)},
		{SessionID: m.ID, RunID: "run", Kind: "state", Text: "idle"},
	} {
		if _, err := l.Append(e); err != nil {
			t.Fatal(err)
		}
	}
	if err := l.Store().SetState(ctx, supervisor.Replay(l.All())); err != nil {
		t.Fatal(err)
	}

	page, err := s.Transcript(ctx, &api.TranscriptRequest{SessionId: m.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Events) == 0 {
		t.Fatal("no rows came from the store")
	}
	var texts []string
	for _, e := range page.Events {
		texts = append(texts, e.Text)
	}
	if len(texts) != 2 || texts[0] != "build it" || texts[1] != "on it" {
		t.Fatal("the store answered with", texts)
	}
}

// Reading the recorded state means replaying nothing, so everything a replay
// used to leave behind has to be found another way: the session's state, the
// model catalog of each run, and the trim floor.
func TestReadingTheRecordedStateKeepsWhatTheReplayLeft(t *testing.T) {
	ctx := context.Background()
	s, m, l := projectionFixture(t)
	for _, e := range []core.Event{
		{SessionID: m.ID, RunID: "run", Kind: "input", Text: "go"},
		{SessionID: m.ID, RunID: "run", Kind: "models", Payload: json.RawMessage(`{"models":[{"id":"old"}]}`)},
		{SessionID: m.ID, RunID: "run", Kind: "models", Payload: json.RawMessage(`{"models":[{"id":"new"}]}`)},
		{SessionID: m.ID, RunID: "run", Kind: "state", Text: "idle"},
	} {
		if _, err := l.Append(e); err != nil {
			t.Fatal(err)
		}
	}
	if err := l.Store().SetState(ctx, supervisor.Replay(l.All())); err != nil {
		t.Fatal(err)
	}

	p, err := s.lockProjection(ctx, m)
	if err != nil {
		t.Fatal(err)
	}
	defer p.mu.Unlock()
	if p.snapshot.State != "idle" || p.snapshot.LastSeq != l.All()[len(l.All())-1].Seq {
		t.Fatal("state", p.snapshot.State, p.snapshot.LastSeq)
	}
	// Last one wins, as the replay did: a run republishes its catalog whenever a
	// setting is applied and only the newest describes the agent now.
	if c, ok := p.catalogs["run"]; !ok {
		t.Fatal("no catalog for the run")
	} else if string(c.payload) == "" || !strings.Contains(string(c.payload), "new") {
		t.Fatal("the catalog is not the newest one:", string(c.payload))
	}
}
