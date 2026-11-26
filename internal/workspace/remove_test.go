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
	"time"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/assets"
	"github.com/lesomnus/cxz/internal/convindex"
	"github.com/lesomnus/cxz/internal/mcpconfig"
	_ "github.com/lesomnus/cxz/internal/memorymcp"
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
		var filters [][2]string
		for i, a := range args {
			// Only Compose's label is filtered: listing cxz's labels returns
			// everything, so the inspect recheck is what refuses a stranger.
			if a == "--filter" && strings.HasPrefix(args[i+1], "label=com.docker.compose.project=") {
				k, v, _ := strings.Cut(strings.TrimPrefix(args[i+1], "label="), "=")
				filters = append(filters, [2]string{k, v})
			}
		}
		var ids []string
	items:
		for key, item := range f.items {
			if !strings.HasPrefix(key, args[0]+":") {
				continue
			}
			labels, _ := item["Labels"].(map[string]string)
			if config, ok := item["Config"].(map[string]any); ok {
				labels, _ = config["Labels"].(map[string]string)
			}
			for _, f := range filters {
				if labels[f[0]] != f[1] {
					continue items
				}
			}
			ids = append(ids, strings.TrimPrefix(key, args[0]+":"))
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
	// What removal has to clear is the searchable record; the manager keeps no
	// copy of the events themselves any more. See cxz#268.
	indexRoot := filepath.Join(root, "index")
	if err := os.MkdirAll(indexRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	index, err := convindex.Open(ctx, indexRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer index.Close()
	m.Conversations = index
	// One term and one minute each: a search reads one window of time at a
	// time, so events sharing a millisecond can share a window and hide each
	// other.
	for i, id := range []string{"s", "archived", "other-session"} {
		when := time.Now().Add(-time.Duration(i) * time.Minute).UnixMilli()
		if err := m.CacheEvents(ctx, &api.EventBatch{Events: []*api.Event{{SessionId: id, Seq: 1, Kind: "input", Text: "said-" + id, TimeMs: when}}}); err != nil {
			t.Fatal(err)
		}
	}
	indexed := func(session string) int {
		t.Helper()
		visits, _, err := index.Search(ctx, convindex.Query{Query: "said-" + session, Sessions: []string{session}}, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		return len(visits)
	}
	for _, id := range []string{"s", "archived", "other-session"} {
		if indexed(id) == 0 {
			t.Fatal("nothing was indexed for", id)
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
	if err := m.CacheEvents(ctx, &api.EventBatch{Events: []*api.Event{{SessionId: "s", Seq: 2, Kind: "input", Text: "said-s again", TimeMs: time.Now().UnixMilli()}}}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"s", "archived"} {
		if n := indexed(id); n != 0 {
			t.Fatal("a removed session is still searchable:", id, n)
		}
	}
	if n := indexed("other-session"); n == 0 {
		t.Fatal("another project's session lost its record")
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

func TestRemoveProjectComposeNetwork(t *testing.T) {
	owner := strings.Repeat("a", 24)
	compose := composeProjectName(owner, "p")
	f := &removalFake{items: map[string]map[string]any{
		"network:default": {"Id": "default", "Labels": map[string]string{"com.docker.compose.project": compose}},
		"network:theirs":  {"Id": "theirs", "Labels": map[string]string{"com.docker.compose.project": compose + "x"}},
	}}
	if err := removeComposeNetworks(t.Context(), f.run, owner, "p"); err != nil {
		t.Fatal(err)
	}
	if strings.Join(f.removed, " ") != "network:default" {
		t.Fatal("removed", f.removed)
	}
	f.items["network:default"] = map[string]any{"Id": "default", "Labels": map[string]string{"com.docker.compose.project": compose}, "Containers": map[string]any{"e": map[string]string{"Name": "someone"}}}
	if err := removeComposeNetworks(t.Context(), f.run, owner, "p"); err == nil || !strings.Contains(err.Error(), "endpoint") {
		t.Fatal("removed a network something is attached to", err)
	}
}
