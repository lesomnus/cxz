package workspace

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/conversation"
	_ "github.com/lesomnus/payday/config/dbsqlite3"
)

func TestConversationAuthorization(t *testing.T) {
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
	p := &Project{ID: "p", Workspace: "/project", Token: "token", Sessions: []*api.Session{{Id: "claude", Agent: "claude"}, {Id: "codex", Agent: "codex"}}}
	if err = m.save(t.Context(), p); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		project, caller, token string
		want                   bool
	}{{"p", "claude", "token", true}, {"p", "codex", "token", true}, {"p", "other", "token", false}, {"p", "claude", "bad", false}, {"other", "claude", "token", false}, {"p", "claude", "", false}} {
		if got := m.AuthorizeConversation(t.Context(), conversation.RegistryRequest{Project: tc.project, Caller: tc.caller}, tc.token); got != tc.want {
			t.Fatalf("%+v: %v", tc, got)
		}
	}
}
