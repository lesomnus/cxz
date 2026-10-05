package lifecycle

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/accounts"
	"github.com/lesomnus/cxz/internal/conversation"
	"github.com/lesomnus/cxz/resource"
	"github.com/lesomnus/payday/config"
)

func TestConversationRegistryUsesCanonicalAliasAndUUID(t *testing.T) {
	db, _, err := (config.DbConfig{Driver: "sqlite3", Dsn: "file:" + filepath.Join(t.TempDir(), "resources.db"), MaxOpenConns: 1}).Open(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	f := &fixture{p: &api.Project{Id: "project", Workspace: "/workspaces/test", Name: "test"}, s: &api.Session{Id: "session", ProjectId: "project", Agent: "codex", Account: "work", CreateId: "create", CreatedAt: time.Now().UnixMilli(), AuthBackend: accounts.ProjectLocalOAuth}}
	f.s.AuthBinding = accounts.BindingID(f.p.Id, f.s.Account, f.s.AuthBackend)
	stack, err := Build(t.Context(), db, f)
	if err != nil {
		t.Fatal(err)
	}
	list := func(ctx context.Context, project string) ([]conversation.Session, error) {
		return conversation.RegistryFromResources(ctx, project, stack.Session().List)
	}
	first, err := list(t.Context(), "project")
	if err != nil || len(first) != 1 {
		t.Fatal(first, err)
	}
	_, err = stack.Session().Patch(t.Context(), resource.SessionPatchRequest_builder{Ref: sessionRef("session"), Alias: ptr("seal")}.Build())
	if err != nil {
		t.Fatal(err)
	}
	next, err := list(t.Context(), "project")
	if err != nil || len(next) != 1 || next[0].Alias != "seal" || next[0].ID != first[0].ID {
		t.Fatal(next, err)
	}
	other, err := list(t.Context(), "outside")
	if err != nil || len(other) != 0 {
		t.Fatal(other, err)
	}
}
