package workspace

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/assets"
	"github.com/lesomnus/cxz/internal/mcpconfig"
)

type removalFake struct {
	items   map[string]map[string]any
	removed []string
	fail    string
}

func (f *removalFake) run(_ context.Context, args ...string) ([]byte, error) {
	key := args[0] + ":" + args[len(args)-1]
	switch args[1] {
	case "ls":
		var ids []string
		for key := range f.items {
			if strings.HasPrefix(key, args[0]+":") {
				ids = append(ids, strings.TrimPrefix(key, args[0]+":"))
			}
		}
		return []byte(strings.Join(ids, "\n")), nil
	case "inspect":
		v, ok := f.items[key]
		if !ok {
			return nil, errors.New("not found")
		}
		return json.Marshal([]any{v})
	case "rm":
		if f.fail == key {
			return nil, errors.New("busy")
		}
		f.removed = append(f.removed, key)
		delete(f.items, key)
		return nil, nil
	}
	return nil, errors.New("unexpected Docker command: " + strings.Join(args, " "))
}
func newRemovalFake() *removalFake {
	labels := map[string]string{"cxz.owner": "owner", "cxz.project": "p"}
	return &removalFake{items: map[string]map[string]any{
		"container:c": {"Id": "c", "Config": map[string]any{"Labels": labels}},
		"volume:v":    {"Name": "v", "Labels": labels},
		"network:n":   {"Id": "n", "Labels": labels},
	}}
}
func TestRemoveProjectDockerOwnershipAndRetry(t *testing.T) {
	f := newRemovalFake()
	f.items["volume:v"]["Labels"] = map[string]string{"cxz.owner": "other", "cxz.project": "p"}
	if err := removeProjectDocker(t.Context(), f.run, "owner", "p", ""); err == nil || len(f.removed) != 0 {
		t.Fatal("mutated before ownership validation", err, f.removed)
	}
	f = newRemovalFake()
	f.fail = "volume:v"
	if err := removeProjectDocker(t.Context(), f.run, "owner", "p", ""); err == nil {
		t.Fatal("ignored failure")
	}
	if len(f.removed) != 1 || f.removed[0] != "container:c" {
		t.Fatal(f.removed)
	}
	f.fail = ""
	if err := removeProjectDocker(t.Context(), f.run, "owner", "p", ""); err != nil {
		t.Fatal(err)
	}
	if len(f.items) != 0 {
		t.Fatal(f.items)
	}
}
func TestRemoveProjectStateIsolationAndLateEvents(t *testing.T) {
	ctx := t.Context()
	db, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("CREATE TABLE events(session_id TEXT,seq INTEGER,data BLOB,PRIMARY KEY(session_id,seq))"); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	m, err := New(db, root)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	m.Owner = "owner"
	source := t.TempDir()
	file := filepath.Join(source, "source.txt")
	os.WriteFile(file, []byte("keep"), 0600)
	for _, p := range []*Project{{ID: "p", Workspace: source, Sessions: []*api.Session{{Id: "s"}}}, {ID: "other", Sessions: []*api.Session{{Id: "other-session"}}}} {
		if err := m.save(ctx, p); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []string{"s", "archived", "other-session"} {
		if err := m.cache(ctx, &api.EventBatch{Events: []*api.Event{{SessionId: id, Seq: 1, Text: "history"}}}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := mcpconfig.SetProject(root, "p", "cxz_memory", false); err != nil {
		t.Fatal(err)
	}
	if _, err := assets.Add(ctx, filepath.Join(root, "assets"), "p", "s", "attachment.txt", 4, strings.NewReader("data")); err != nil {
		t.Fatal(err)
	}
	f := newRemovalFake()
	f.fail = "volume:v"
	if err := m.removeProject(ctx, "p", []string{"archived"}, f.run); err == nil {
		t.Fatal("ignored cleanup failure")
	}
	if _, err := m.resolve(ctx, "p"); err != nil {
		t.Fatal("lost retry metadata", err)
	}
	f.fail = ""
	if err := m.removeProject(ctx, "p", []string{"archived"}, f.run); err != nil {
		t.Fatal(err)
	}
	if _, err := m.resolve(ctx, "p"); err == nil {
		t.Fatal("project retained")
	}
	if _, err := m.resolve(ctx, "other"); err != nil {
		t.Fatal("other project deleted", err)
	}
	if _, err := os.Stat(file); err != nil {
		t.Fatal("workspace deleted", err)
	}
	if _, err := os.Stat(filepath.Join(root, "projects", "p")); !os.IsNotExist(err) {
		t.Fatal("manifest retained", err)
	}
	if err := m.CacheEvents(ctx, &api.EventBatch{Events: []*api.Event{{SessionId: "s", Seq: 2, Text: "late"}}}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"s", "archived"} {
		page, err := m.cachedHistory(ctx, &api.WatchRequest{SessionId: id})
		if err != nil || len(page.Events) != 0 {
			t.Fatal(id, page, err)
		}
	}
	page, err := m.cachedHistory(ctx, &api.WatchRequest{SessionId: "other-session"})
	if err != nil || len(page.Events) != 1 {
		t.Fatal(page, err)
	}
	cfg, err := mcpconfig.Load(root)
	if err != nil || cfg.Projects["p"] != nil {
		t.Fatal(cfg, err)
	}
	// Retried removal is harmless even after the runtime manifest is gone.
	if err := m.removeProject(ctx, "p", []string{"s", "archived"}, f.run); err != nil {
		t.Fatal(err)
	}
}

func TestRemoveProjectNetworkRefusesForeignEndpoint(t *testing.T) {
	f := newRemovalFake()
	f.items["network:n"]["Containers"] = map[string]any{"outside": map[string]string{"Name": "user-container"}}
	if err := removeProjectDocker(t.Context(), f.run, "owner", "p", "manager"); err == nil || !strings.Contains(err.Error(), "endpoint") {
		t.Fatal(err)
	}
	if _, ok := f.items["network:n"]; !ok {
		t.Fatal("removed network with foreign endpoint")
	}
}
