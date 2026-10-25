package workspace

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/projectid"
)

func TestProjectUUIDMigrationPreservesStateAndRetries(t *testing.T) {
	for _, interrupted := range []bool{false, true} {
		t.Run(map[bool]string{false: "legacy", true: "manifest-committed"}[interrupted], func(t *testing.T) {
			m, p := migrating(t)
			ctx := context.Background()
			legacy := p.ID
			p.Token = "private-token"
			p.ContainerID = "existing-container"
			p.Sessions = []*api.Session{{Id: "session", ProjectId: legacy, AuthBinding: "existing-binding"}}
			if err := m.save(ctx, p); err != nil {
				t.Fatal(err)
			}
			dir, _ := m.projectState(legacy)
			if err := os.MkdirAll(dir, 0700); err != nil {
				t.Fatal(err)
			}
			data := filepath.Join(dir, "history.db")
			if err := os.WriteFile(data, []byte("existing conversation"), 0600); err != nil {
				t.Fatal(err)
			}
			before, _ := os.Stat(data)
			if interrupted {
				p.StorageID, p.ID = legacy, projectid.Canonical(legacy)
				if err := core.WriteJSON(filepath.Join(m.Root, "projects", legacy, "project.json"), p); err != nil {
					t.Fatal(err)
				}
			}
			for range 2 {
				if err := m.migrateProjectIdentities(ctx); err != nil {
					t.Fatal(err)
				}
			}
			all, err := m.all(ctx)
			if err != nil || len(all) != 1 {
				t.Fatal(all, err)
			}
			got := all[0]
			if got.ID != projectid.Canonical(legacy) || got.StorageID != legacy || got.Volume != p.Volume || got.ContainerID != p.ContainerID || got.Token != p.Token {
				t.Fatal("identity migration changed physical resources", got)
			}
			if got.Sessions[0].ProjectId != got.ID || got.Sessions[0].AuthBinding != "existing-binding" {
				t.Fatal("session association lost", got.Sessions)
			}
			for _, ref := range []string{legacy, got.ID} {
				if v, err := m.resolve(ctx, ref); err != nil || v.ID != got.ID {
					t.Fatal(ref, v, err)
				}
			}
			after, _ := os.Stat(data)
			if !os.SameFile(before, after) {
				t.Fatal("state was moved or replaced")
			}
		})
	}
}
