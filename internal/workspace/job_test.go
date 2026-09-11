package workspace

import (
	"context"
	"database/sql"
	"github.com/lesomnus/cxz/internal/dockerx"
	_ "github.com/lesomnus/payday/config/dbsqlite3"
	"path/filepath"
	"testing"
)

func TestCheckpointRecoveryAndInventory(t *testing.T) {
	t.Setenv("CXZ_MANAGER_CONTAINER", "")
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
	p := &Project{ID: "project", Workspace: "/workspaces/test", Token: "preserved", Job: ProvisionJob{Attempt: 1}}
	if err = m.checkpoint(context.Background(), p, "devcontainer-up"); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("DELETE FROM projects"); err != nil {
		t.Fatal(err)
	}
	m, err = New(db, root)
	if err != nil {
		t.Fatal(err)
	}
	p, err = m.resolve(context.Background(), "project")
	if err != nil {
		t.Fatal(err)
	}
	if p.Job.State != "interrupted" || p.Job.Step != "devcontainer-up" || p.Token != "preserved" || p.Job.Attempt != 1 {
		t.Fatalf("lost recovery metadata: %+v", p)
	}
	m.Owner = "owner"
	c := dockerx.Container{ID: "recovered"}
	c.Config.Labels = map[string]string{"cxz.owner": "owner", "cxz.project": "project", "devcontainer.local_folder": p.Workspace}
	if err = m.reconcile(context.Background(), p, []dockerx.Container{c}); err != nil {
		t.Fatal(err)
	}
	if p.ContainerID != c.ID {
		t.Fatal("did not recover resource created before checkpoint")
	}
	other := c
	other.ID = "duplicate"
	if m.reconcile(context.Background(), p, []dockerx.Container{c, other}) == nil {
		t.Fatal("accepted ambiguous ownership")
	}
	c.Config.Labels["cxz.owner"] = "foreign"
	if err = m.reconcile(context.Background(), p, []dockerx.Container{c}); err != nil {
		t.Fatal(err)
	}
	if p.ContainerID != "" {
		t.Fatal("adopted foreign container")
	}
}
