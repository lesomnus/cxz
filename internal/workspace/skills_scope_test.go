package workspace

import (
	"database/sql"
	"testing"

	"github.com/lesomnus/cxz/api"
)

// Who refuses an unknown project moved here. The resource API used to resolve a
// `ProjectRef` against its own rows before the runtime saw it; the skills and
// MCP services now pass the scope through as the id a client resolved, so the
// refusal is this side's. An empty scope is still the installation itself,
// which is a scope and not a missing argument.
// See docs/plans/service-boundaries.md.
func TestAnUnknownScopeIsRefused(t *testing.T) {
	ctx := t.Context()
	db, err := sql.Open("sqlite3", t.TempDir()+"/cxz.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec("CREATE TABLE events(session_id TEXT,seq INTEGER,data BLOB,PRIMARY KEY(session_id,seq))"); err != nil {
		t.Fatal(err)
	}
	m, err := New(db, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if err = m.save(ctx, &Project{ID: "p", Workspace: t.TempDir(), Sessions: []*api.Session{{Id: "s"}}}); err != nil {
		t.Fatal(err)
	}

	if got, err := m.projectScope(ctx, ""); err != nil || got != "" {
		t.Fatal("the installation's own scope was refused:", got, err)
	}
	if got, err := m.projectScope(ctx, "p"); err != nil || got != "p" {
		t.Fatal("a registered project did not resolve:", got, err)
	}
	if _, err := m.projectScope(ctx, "/nowhere"); err == nil {
		t.Fatal("accepted a project the installation does not hold")
	}
}
