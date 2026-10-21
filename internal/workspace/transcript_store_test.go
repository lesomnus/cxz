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
	"github.com/lesomnus/cxz/internal/sessionstore"
)

// The transcript and one event's details come from the session's own store, with
// no project running and no Docker to reach one. The rows are there because the
// writer builds them: sessionstore syncs the projection on every append, which
// is what lets a reader open the file read-only. See cxz#268.
func TestTranscriptAndDetailsReadTheSessionsOwnStore(t *testing.T) {
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

	// Nothing recorded yet is an empty transcript rather than an error.
	empty, err := m.Transcript(ctx, &api.TranscriptRequest{SessionId: session})
	if err != nil || len(empty.Events) != 0 {
		t.Fatal("a session with no store was not empty", empty, err)
	}

	dir, _ := m.projectState(p.ID)
	sessionDir := core.Dir(filepath.Join(dir, "data"), session)
	if err = os.MkdirAll(sessionDir, 0o700); err != nil {
		t.Fatal(err)
	}
	store, err := sessionstore.Open(ctx, sessionstore.Path(sessionDir), session, true)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Append(ctx, []core.Event{
		{SessionID: session, RunID: "run", Seq: 1, Kind: "input", Text: "a question"},
		{SessionID: session, RunID: "run", Seq: 2, Kind: "assistant", Text: "an answer"},
	}); err != nil {
		t.Fatal(err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", t.TempDir())
	page, err := m.Transcript(ctx, &api.TranscriptRequest{SessionId: session})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Events) == 0 {
		t.Fatal("the writer's projection was not read")
	}
	if page.SnapshotSeq != 2 {
		t.Fatal("the snapshot is not the last recorded event:", page.SnapshotSeq)
	}

	details, err := m.EventDetails(ctx, &api.EventDetailsRequest{SessionId: session, Seq: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(details.Events) != 1 || details.Events[0].Text != "an answer" {
		t.Fatal("details are not the event the supervisor wrote", details.Events)
	}
	if _, err = m.EventDetails(ctx, &api.EventDetailsRequest{SessionId: session, Seq: 99}); err == nil {
		t.Fatal("an event that was never recorded was returned")
	}
}
