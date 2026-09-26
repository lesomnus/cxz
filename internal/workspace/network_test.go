package workspace

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

func TestWorkspacesShareTransportButKeepSeparateState(t *testing.T) {
	t.Setenv("CXZ_MANAGER_CONTAINER", "")
	t.Setenv("CXZ_OWNER", "123456789012345678901234")
	root := t.TempDir()
	workspaces := t.TempDir()
	t.Setenv("CXZ_WORKSPACE_ROOT", workspaces)
	db, err := sql.Open("sqlite3", filepath.Join(root, "registry.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	m, err := New(db, root)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	var projects []*Project
	for _, name := range []string{"one", "two"} {
		path := filepath.Join(workspaces, name)
		if err := os.Mkdir(path, 0700); err != nil {
			t.Fatal(err)
		}
		v, err := m.Register(ctx, path, "")
		if err != nil {
			t.Fatal(err)
		}
		p, err := m.resolve(ctx, v.Id)
		if err != nil {
			t.Fatal(err)
		}
		projects = append(projects, p)
	}
	if projects[0].Network != projects[1].Network || projects[0].Volume == projects[1].Volume || projects[0].Token == projects[1].Token {
		t.Fatal("network not shared or private state no longer isolated")
	}
	projects[0].Network = "legacy-project-network"
	if err = m.save(ctx, projects[0]); err != nil {
		t.Fatal(err)
	}
	m, err = New(db, root)
	if err != nil {
		t.Fatal(err)
	}
	p, err := m.resolve(ctx, projects[0].ID)
	if err != nil || p.Network != "legacy-project-network" {
		t.Fatal("restart silently migrated legacy networking", err)
	}
}
