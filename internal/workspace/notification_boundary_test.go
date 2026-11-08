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
)

func TestNotificationBoundaryReadsForkSessionManifest(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "manager.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	m, err := New(db, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	id := strings.Repeat("a", 24)
	p := &Project{ID: "project-uuid", StorageID: "legacy-project", Sessions: []*api.Session{{Id: id}}}
	if err = m.save(ctx, p); err != nil {
		t.Fatal(err)
	}
	dir, _ := m.projectState(p.storageID())
	sessionDir := core.Dir(filepath.Join(dir, "data"), id)
	if err = os.MkdirAll(sessionDir, 0700); err != nil {
		t.Fatal(err)
	}
	// Fork creation persists session.json, not manifest.json. No running project
	// or retained history is needed to establish the inherited notification floor.
	manifest := core.Session{ID: id, Fork: &core.ForkOrigin{SessionID: "parent", ThroughSeq: 150}}
	if err = core.WriteJSON(filepath.Join(sessionDir, "session.json"), manifest); err != nil {
		t.Fatal(err)
	}
	floor, err := m.NotificationBoundary(ctx, id)
	if err != nil || floor != 151 {
		t.Fatalf("fork notification floor = %d, %v; want 151", floor, err)
	}
	manifest.Fork = nil
	if err = core.WriteJSON(filepath.Join(sessionDir, "session.json"), manifest); err != nil {
		t.Fatal(err)
	}
	floor, err = m.NotificationBoundary(ctx, id)
	if err != nil || floor != 0 {
		t.Fatalf("ordinary session notification floor = %d, %v; want 0", floor, err)
	}
}
