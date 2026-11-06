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
	"github.com/lesomnus/cxz/internal/agentview"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/sessionstore"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func backgroundFixture(t *testing.T) (*Manager, *Project, string) {
	t.Helper()
	db, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "manager.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	m, err := New(db, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Close)
	m.StateVolume = "manager-state"
	p := &Project{
		ID: strings.Repeat("a", 32), StorageID: strings.Repeat("b", 24),
		Sessions: []*api.Session{{Id: strings.Repeat("c", 32), RunId: "run"}},
	}
	if err = m.save(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	dir, _ := m.projectState(p.storageID())
	sessionDir := core.Dir(filepath.Join(dir, "data"), p.Sessions[0].Id)
	if err = os.MkdirAll(sessionDir, 0700); err != nil {
		t.Fatal(err)
	}
	return m, p, sessionDir
}

func TestBackgroundReadsStoredJobsWithoutAContainerConnection(t *testing.T) {
	ctx := context.Background()
	m, p, dir := backgroundFixture(t)
	id := p.Sessions[0].Id
	store, err := sessionstore.Open(ctx, sessionstore.Path(dir), id, true)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err = store.Append(ctx, []core.Event{
		{SessionID: id, RunID: "run", Seq: 1, Kind: "background", Payload: []byte(`{"type":"background","provider":"codex","subtype":"background_tasks_changed","tasks":[{"task_id":"42"}]}`)},
		{SessionID: id, RunID: "run", Seq: 2, Kind: "assistant", Text: "Working"},
	}); err != nil {
		t.Fatal(err)
	}
	if err = store.SetState(ctx, core.Snapshot{LastSeq: 2}); err != nil {
		t.Fatal(err)
	}
	// A reader's fence must not skip events appended after the saved state.
	if err = store.Append(ctx, []core.Event{{SessionID: id, RunID: "run", Seq: 3, Kind: "assistant", Text: "Still working"}}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", t.TempDir())
	for _, container := range []string{"", "stopped-container", "running-container"} {
		p.ContainerID = container
		if err = m.save(ctx, p); err != nil {
			t.Fatal(err)
		}
		reply, err := m.Background(ctx, &api.SessionRef{Id: id})
		if err != nil {
			t.Fatalf("container %q: %v", container, err)
		}
		var states map[string]agentview.BackgroundState
		if err = json.Unmarshal(reply.Data, &states); err != nil {
			t.Fatal(err)
		}
		if reply.LastSeq != 2 || !states["run"].Tasks["42"].Active {
			t.Fatal(reply.LastSeq, string(reply.Data))
		}
	}
	if err = store.Append(ctx, []core.Event{{SessionID: id, RunID: "run", Seq: 4, Kind: "background", Payload: []byte(`{"type":"background","provider":"codex","subtype":"background_tasks_changed","tasks":[]}`)}}); err != nil {
		t.Fatal(err)
	}
	if err = store.SetState(ctx, core.Snapshot{LastSeq: 4}); err != nil {
		t.Fatal(err)
	}
	reply, err := m.Background(ctx, &api.SessionRef{Id: id})
	if err != nil || reply.LastSeq != 4 || !strings.Contains(string(reply.Data), `"active":false`) {
		t.Fatal(reply, err)
	}
}

func TestBackgroundDistinguishesEmptyUnknownAndCorruptStores(t *testing.T) {
	ctx := context.Background()
	m, p, dir := backgroundFixture(t)
	id := p.Sessions[0].Id
	t.Setenv("PATH", t.TempDir())
	reply, err := m.Background(ctx, &api.SessionRef{Id: id})
	if err != nil || reply.LastSeq != 0 || string(reply.Data) != `{}` {
		t.Fatal(reply, err)
	}
	if _, err = m.Background(ctx, &api.SessionRef{Id: strings.Repeat("d", 32)}); err == nil {
		t.Fatal("an unknown session returned empty background state")
	}
	store, err := sessionstore.Open(ctx, sessionstore.Path(dir), id, true)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err = store.DB().ExecContext(ctx, `INSERT OR REPLACE INTO meta VALUES('background','broken-json')`); err != nil {
		t.Fatal(err)
	}
	if _, err = m.Background(ctx, &api.SessionRef{Id: id}); err == nil {
		t.Fatal("corrupt recorded state was silently treated as empty")
	}
}

func TestRuntimeClientRejectsAnEmptyContainerBeforeInvokingDocker(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	_, _, err := (&Manager{}).client(context.Background(), &Project{})
	if status.Code(err) != codes.FailedPrecondition || strings.Contains(err.Error(), "docker") {
		t.Fatal(err)
	}
}
