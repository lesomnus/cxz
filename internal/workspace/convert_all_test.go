package workspace

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/journal"
	"github.com/lesomnus/cxz/internal/sessionstore"
)

// Every session the manager knows ends up with a store, which is what has to be
// true before the journal can stop being written: a session that stopped before
// stores existed and that nobody opens again would otherwise keep its record in
// a file nothing writes any more. Idempotent, so the pass can run on every
// start. See cxz#268.
func TestEverySessionEndsUpWithAStore(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	m, err := New(db, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	m.StateVolume = "cxz-owner-state"

	journalled := strings.Repeat("a", 24)  // a journal and no store
	stored := strings.Repeat("b", 24)      // a store already
	silent := strings.Repeat("c", 24)      // nothing recorded at all
	p := &Project{ID: strings.Repeat("p", 24), Sessions: []*api.Session{{Id: journalled}, {Id: stored}, {Id: silent}}}
	if err = m.save(ctx, p); err != nil {
		t.Fatal(err)
	}
	dir, _ := m.projectState(p.ID)
	sessionDir := func(id string) string { return core.Dir(filepath.Join(dir, "data"), id) }
	for _, id := range []string{journalled, stored, silent} {
		if err = os.MkdirAll(sessionDir(id), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []string{journalled, stored} {
		l, err := journal.Open(filepath.Join(sessionDir(id), "events.jsonl"))
		if err != nil {
			t.Fatal(err)
		}
		if _, err = l.Append(core.Event{SessionID: id, RunID: "run", Kind: "input", Text: "said"}); err != nil {
			t.Fatal(err)
		}
		if err = l.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = sessionstore.Convert(ctx, sessionDir(stored), stored); err != nil {
		t.Fatal(err)
	}

	m.ConvertStores(ctx)

	for _, id := range []string{journalled, stored} {
		if _, err = os.Stat(sessionstore.Path(sessionDir(id))); err != nil {
			t.Fatal("no store for", id, err)
		}
		page, err := m.History(ctx, &api.WatchRequest{SessionId: id})
		if err != nil || len(page.Events) != 1 || page.Events[0].Text != "said" {
			t.Fatal("the conversation is not readable for", id, page, err)
		}
	}
	if _, err = os.Stat(sessionstore.Path(sessionDir(silent))); !os.IsNotExist(err) {
		t.Fatal("a session that recorded nothing was given a store", err)
	}
	// Running again changes nothing, which is what lets it run on every start.
	before, err := os.Stat(sessionstore.Path(sessionDir(journalled)))
	if err != nil {
		t.Fatal(err)
	}
	m.ConvertStores(ctx)
	after, err := os.Stat(sessionstore.Path(sessionDir(journalled)))
	if err != nil {
		t.Fatal(err)
	}
	if !after.ModTime().Equal(before.ModTime()) {
		t.Fatal("a session that already had a store was converted again")
	}
}
