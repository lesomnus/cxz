package workspace

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/quotashare"
	_ "github.com/lesomnus/payday/config/dbsqlite3"
)

func TestQuotaAuthorizationBoundToProjectAndAccount(t *testing.T) {
	root := t.TempDir()
	db, err := sql.Open("sqlite3", filepath.Join(root, "registry.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	m, err := New(db, root)
	if err != nil {
		t.Fatal(err)
	}
	p := &Project{ID: "p", Workspace: "/project", Token: "project-token", Sessions: []*api.Session{{Id: "s", Agent: "claude", Account: "one"}, {Id: "codex", Agent: "codex", Account: "two"}}}
	if err = m.save(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		session, account, token string
		want                    bool
	}{{"s", "one", "project-token", true}, {"s", "two", "project-token", false}, {"other", "one", "project-token", false}, {"codex", "two", "project-token", false}, {"s", "one", "wrong", false}} {
		got := m.AuthorizeQuota(context.Background(), quotashare.Request{Project: "p", Session: tc.session, Account: tc.account}, tc.token)
		if got != tc.want {
			t.Fatalf("%+v: %v", tc, got)
		}
	}
}
