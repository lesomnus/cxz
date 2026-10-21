package workspace

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/journal"
	"github.com/lesomnus/cxz/internal/sessionstore"
)

// A session that stopped before stores existed has only its journal: the
// conversion runs when a supervisor starts, and a stopped session's supervisor
// never will. Seven such sessions were on the installation when the manager
// stopped keeping its own copy, so reading one has to convert it. It is also
// what has to be true of every session before the journal can stop being
// written at all. See cxz#268.
func TestAJournalOnlySessionIsConvertedWhenItIsRead(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec("CREATE TABLE events(session_id TEXT, seq INTEGER, data BLOB, PRIMARY KEY(session_id,seq))"); err != nil {
		t.Fatal(err)
	}
	m, err := New(db, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	m.StateVolume = "cxz-owner-state"
	session := strings.Repeat("s", 24)
	p := &Project{ID: strings.Repeat("p", 24), Sessions: []*api.Session{{Id: session}}}
	if err = m.save(ctx, p); err != nil {
		t.Fatal(err)
	}

	dir, _ := m.projectState(p.ID)
	sessionDir := core.Dir(filepath.Join(dir, "data"), session)
	if err = os.MkdirAll(sessionDir, 0o700); err != nil {
		t.Fatal(err)
	}
	l, err := journal.Open(filepath.Join(sessionDir, "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range []core.Event{
		{SessionID: session, RunID: "run", Kind: "input", Text: "a question"},
		{SessionID: session, RunID: "run", Kind: "assistant", Text: "an answer", Response: &core.ResponseMetadata{Phase: "final_answer"}},
		{SessionID: session, RunID: "run", Kind: "tool_call", RequestID: "t1", Text: "Bash", Payload: json.RawMessage(`{"input":{"command":"go test ./..."}}`)},
	} {
		if _, err = l.Append(e); err != nil {
			t.Fatal(err)
		}
	}
	if err = l.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(sessionstore.Path(sessionDir)); !os.IsNotExist(err) {
		t.Fatal("the session already had a store", err)
	}

	t.Setenv("PATH", t.TempDir())
	page, err := m.History(ctx, &api.WatchRequest{SessionId: session})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Events) != 3 || page.Events[0].Text != "a question" {
		t.Fatal("the journal was not read", page.Events)
	}
	if _, err = os.Stat(sessionstore.Path(sessionDir)); err != nil {
		t.Fatal("reading did not leave a store behind:", err)
	}
	// And the transcript reads the rows the conversion built.
	transcript, err := m.Transcript(ctx, &api.TranscriptRequest{SessionId: session})
	if err != nil {
		t.Fatal(err)
	}
	if len(transcript.Events) == 0 {
		t.Fatal("the conversion left no reading projection")
	}
}
