package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/maintenance"
	"github.com/lesomnus/cxz/internal/projectid"
	"github.com/lesomnus/payday/config"
)

func TestMaintenanceUsesEveryProjectReferenceAndPropagatesDisabledPolicy(t *testing.T) {
	ctx := t.Context()
	db, _, err := (config.DbConfig{Driver: "sqlite3", Dsn: "file:" + filepath.Join(t.TempDir(), "registry.db"), MaxOpenConns: 1}).Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	m, err := New(db, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	p := &Project{ID: projectid.New(), Workspace: "/project"}
	if err = m.save(ctx, p); err != nil {
		t.Fatal(err)
	}
	state, _ := m.projectState(p.storageID())
	root := filepath.Join(state, "data")
	session := filepath.Join(root, "sessions", "s")
	os.MkdirAll(session, 0700)
	if err = core.WriteJSON(filepath.Join(session, "session.json"), map[string]string{"id": "s", "agent": "/cxz/tools/codex/0.1.0/bin/codex"}); err != nil {
		t.Fatal(err)
	}
	tools := t.TempDir()
	for i := 1; i <= 4; i++ {
		dir := filepath.Join(tools, "codex", fmt.Sprintf("0.%d.0", i))
		os.MkdirAll(dir, 0755)
		old := time.Now().Add(-time.Duration(20-i) * 24 * time.Hour)
		os.Chtimes(dir, old, old)
	}
	cs, err := m.maintenanceVersions(ctx, tools, maintenance.Default(), time.Now(), true)
	if err != nil || len(cs) != 1 || filepath.Base(cs[0].Path) != "0.2.0" {
		t.Fatal(cs, err)
	}
	if err = core.WriteJSON(filepath.Join(m.Root, "maintenance.json"), map[string]bool{"disabled": true}); err != nil {
		t.Fatal(err)
	}
	if err = m.SyncMaintenancePolicy(ctx); err != nil {
		t.Fatal(err)
	}
	policy, err := maintenance.Load(root)
	if err != nil || !policy.Disabled {
		t.Fatal(policy, err)
	}
	st, err := os.Stat(filepath.Join(root, "maintenance.json"))
	if err != nil || st.Mode().Perm()&0044 != 0044 {
		t.Fatal("remote user cannot read propagated policy", st, err)
	}
	p.Volume = "legacy-state"
	if err = m.save(ctx, p); err != nil {
		t.Fatal(err)
	}
	if _, err = m.maintenanceVersions(ctx, tools, maintenance.Default(), time.Now(), true); err == nil {
		t.Fatal("deleted tools with an unknown legacy project reference set")
	}
}

func TestPressureReclaimsStoppedProjectCacheWithoutStartingItsRuntime(t *testing.T) {
	ctx := t.Context()
	db, _, err := (config.DbConfig{Driver: "sqlite3", Dsn: "file:" + filepath.Join(t.TempDir(), "registry.db"), MaxOpenConns: 1}).Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	m, err := New(db, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	project := &Project{ID: projectid.New(), Workspace: "/offline"}
	if err = m.save(ctx, project); err != nil {
		t.Fatal(err)
	}
	state, _ := m.projectState(project.storageID())
	root := filepath.Join(state, "data")
	profile := filepath.Join(root, "session-profiles", strings.Repeat("a", 32), "accounts", "work")
	cache := filepath.Join(profile, "home", ".cache", "go-build")
	os.MkdirAll(cache, 0700)
	os.WriteFile(filepath.Join(cache, "object"), []byte("reproducible"), 0600)
	credential := filepath.Join(profile, "config", "auth.json")
	os.MkdirAll(filepath.Dir(credential), 0700)
	os.WriteFile(credential, []byte("fixture credential"), 0600)
	tools := t.TempDir()
	guard, err := core.Lock(filepath.Join(profile, "login.lock"))
	if err != nil {
		t.Fatal(err)
	}
	if cs, err := m.maintenanceFiles(ctx, tools, maintenance.Default(), time.Now(), true); err != nil || len(cs) != 0 {
		t.Fatal("live profile exposed to Manager cleanup", cs, err)
	}
	guard.Close()
	cs, err := m.maintenanceFiles(ctx, tools, maintenance.Default(), time.Now(), true)
	if err != nil || len(cs) != 1 || cs[0].Kind != maintenance.Cache {
		t.Fatal(cs, err)
	}
	if ok, err := cs[0].Remove(ctx); err != nil || !ok {
		t.Fatal(ok, err)
	}
	if _, err = os.Stat(cache); !os.IsNotExist(err) {
		t.Fatal("offline cache retained", err)
	}
	if _, err = os.Stat(credential); err != nil {
		t.Fatal("provider state removed", err)
	}
	os.MkdirAll(cache, 0700)
	if err = core.WriteJSON(filepath.Join(root, "maintenance.json"), map[string]bool{"disabled": true}); err != nil {
		t.Fatal(err)
	}
	if cs, err = m.maintenanceFiles(ctx, tools, maintenance.Default(), time.Now(), true); err != nil || len(cs) != 0 {
		t.Fatal("disabled project was cleaned", cs, err)
	}
}
