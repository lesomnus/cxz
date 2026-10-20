package server

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/journal"
	"github.com/lesomnus/cxz/internal/sessionstore"
	"github.com/lesomnus/cxz/internal/supervisor"
	"google.golang.org/protobuf/proto"
)

// recorded writes a session's store the way its supervisor would: the records,
// and beside them the reduced state fenced to the journal they came from.
func recorded(t *testing.T, root string, m core.Session, events []core.Event) {
	t.Helper()
	ctx := context.Background()
	dir := core.Dir(root, m.ID)
	store, err := sessionstore.Open(ctx, sessionstore.Path(dir), m.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err = store.Append(ctx, events); err != nil {
		t.Fatal(err)
	}
	size, mtime, err := journal.Fence(filepath.Join(dir, "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	snap := supervisor.ReplayFrom(supervisor.Replay(nil), events)
	if err = store.SetState(ctx, snap, sessionstore.Fence{Size: size, ModTime: mtime, Seq: events[len(events)-1].Seq}); err != nil {
		t.Fatal(err)
	}
}

// A conversation served from the session's own store is the same conversation.
// The journal is not read: what proves it is that the server answers before the
// cache it used to fill has anything in it.
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
	recorded(t, s.root, m, l.All())

	var cached int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE session_id=?`, m.ID).Scan(&cached); err != nil {
		t.Fatal(err)
	}
	if cached != 0 {
		t.Fatal("the fixture already has a cache; this test cannot tell where the answer came from")
	}

	page, err := s.Transcript(ctx, &api.TranscriptRequest{SessionId: m.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Events) == 0 {
		t.Fatal("no rows came from the store")
	}
	if err = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE session_id=?`, m.ID).Scan(&cached); err != nil {
		t.Fatal(err)
	}
	if cached != 0 {
		t.Fatal("the server filled its cache for a session that keeps its own store")
	}
	var texts []string
	for _, e := range page.Events {
		texts = append(texts, e.Text)
	}
	if len(texts) != 2 || texts[0] != "build it" || texts[1] != "on it" {
		t.Fatal("the store answered with", texts)
	}
}

// Taking the recorded state means not replaying the journal, so everything the
// replay used to leave behind has to be found another way. These are the three:
// the session's state, the model catalog of each run, and the trim floor.
func TestTakingTheRecordedStateKeepsWhatTheReplayLeft(t *testing.T) {
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
	recorded(t, s.root, m, l.All())

	p, err := s.lockProjection(ctx, m)
	if err != nil {
		t.Fatal(err)
	}
	defer p.mu.Unlock()
	if !p.owned {
		t.Fatal("the recorded state was not taken")
	}
	if p.snapshot.State != "idle" || p.snapshot.LastSeq != l.All()[len(l.All())-1].Seq {
		t.Fatal("state", p.snapshot.State, p.snapshot.LastSeq)
	}
	// Last one wins, as the replay did: a run republishes its catalog whenever a
	// setting is applied and only the newest describes the agent now.
	if c, ok := p.catalogs["run"]; !ok {
		t.Fatal("no catalog for the run")
	} else if string(c.payload) == "" || !contains(string(c.payload), "new") {
		t.Fatal("the catalog is not the newest one:", string(c.payload))
	}
	if p.cursor.Seq != p.snapshot.LastSeq {
		t.Fatal("the cursor is not where the state says", p.cursor.Seq)
	}
}

// A journal that moved after the state was recorded is a fence that no longer
// matches, and then nothing is taken: the server reads the journal exactly as
// it did before any of this existed.
func TestAMovedJournalIsNotTaken(t *testing.T) {
	ctx := context.Background()
	s, m, l := projectionFixture(t)
	for _, e := range []core.Event{
		{SessionID: m.ID, RunID: "run", Kind: "input", Text: "one"},
		{SessionID: m.ID, RunID: "run", Kind: "state", Text: "idle"},
	} {
		if _, err := l.Append(e); err != nil {
			t.Fatal(err)
		}
	}
	recorded(t, s.root, m, l.All())
	// One more record that the store never saw.
	if _, err := l.Append(core.Event{SessionID: m.ID, RunID: "run", Kind: "assistant", Text: "later"}); err != nil {
		t.Fatal(err)
	}

	p, err := s.lockProjection(ctx, m)
	if err != nil {
		t.Fatal(err)
	}
	if p.owned {
		p.mu.Unlock()
		t.Fatal("a stale fence was taken")
	}
	if p.snapshot.LastSeq != l.All()[len(l.All())-1].Seq {
		p.mu.Unlock()
		t.Fatal("the journal was not read:", p.snapshot.LastSeq)
	}
	p.mu.Unlock()
	// And the conversation is complete, from the cache the journal filled.
	page, err := s.Transcript(ctx, &api.TranscriptRequest{SessionId: m.ID})
	if err != nil {
		t.Fatal(err)
	}
	last := page.Events[len(page.Events)-1]
	if last.Text != "later" {
		t.Fatal("the last row is", last.Text)
	}
	_ = proto.Clone(last)
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
