package purge

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/transport"
)

const testOwner = "1234567890abcdef12345678"

type fakeDocker struct {
	items      map[string]map[string]any
	removed    []string
	failRemove string
}

func (f *fakeDocker) run(_ context.Context, args ...string) ([]byte, error) {
	if args[0] == "ps" || (len(args) > 1 && args[1] == "ls") {
		kind := args[0]
		if kind == "ps" {
			kind = "container"
		}
		var ids []string
		for key := range f.items {
			if strings.HasPrefix(key, kind+":") {
				ids = append(ids, strings.TrimPrefix(key, kind+":"))
			}
		}
		return []byte(strings.Join(ids, "\n")), nil
	}
	if len(args) > 1 && args[1] == "inspect" {
		v, ok := f.items[args[0]+":"+args[2]]
		if !ok {
			return nil, fmt.Errorf("missing")
		}
		return json.Marshal([]any{v})
	}
	kind, id := args[0], args[len(args)-1]
	if kind == "rm" {
		kind = "container"
	}
	if args[0] == "rm" || args[1] == "rm" {
		if kind+":"+id == f.failRemove {
			return nil, fmt.Errorf("resource in use")
		}
		delete(f.items, kind+":"+id)
		f.removed = append(f.removed, kind+":"+id)
		return nil, nil
	}
	return nil, fmt.Errorf("unexpected Docker operation: %v", args)
}
func setup(t *testing.T) (string, *fakeDocker) {
	t.Helper()
	root := t.TempDir()
	if err := core.WriteJSON(filepath.Join(root, "installation.json"), transport.Installation{Owner: testOwner, Container: "manager", StateVolume: "state", ToolsVolume: "tools"}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"settings.json", "settings.jsonm", "settings.jsonc", "settings.schema.json"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("{}"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	f := &fakeDocker{items: map[string]map[string]any{
		"container:manager-id": {"Id": "manager-id", "Name": "/manager", "Config": map[string]any{"Labels": map[string]string{"cxz.owner": testOwner}}},
		"container:agent-id":   {"Id": "agent-id", "Name": "/agent", "Config": map[string]any{"Labels": map[string]string{"cxz.owner": testOwner, "cxz.project": "p"}}},
		"volume:state":         {"Name": "state", "CreatedAt": "today", "Labels": map[string]string{"cxz.owner": testOwner}},
		"volume:project":       {"Name": "project", "CreatedAt": "today", "Labels": map[string]string{"cxz.owner": testOwner, "cxz.project": "p"}},
		"network:network-id":   {"Id": "network-id", "Name": "project-network", "Labels": map[string]string{"cxz.owner": testOwner}},
	}}
	return root, f
}
func all() map[string]bool {
	s := map[string]bool{}
	for _, g := range Groups {
		s[g.ID] = true
	}
	return s
}
func TestOwnedCleanupAndUnknownPreservation(t *testing.T) {
	root, f := setup(t)
	ctx := context.Background()
	unknown := filepath.Join(root, "my-source.txt")
	os.WriteFile(unknown, []byte("keep"), 0600)
	p, err := Discover(ctx, root, f.run)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.removed) != 0 {
		t.Fatal("discovery mutated")
	}
	if err = Execute(ctx, p, all(), f.run, io.Discard); err != nil {
		t.Fatal(err)
	}
	if len(f.items) != 0 || f.removed[0] != "container:manager-id" {
		t.Fatal("wrong resource order", f.removed)
	}
	if _, err = os.Stat(unknown); err != nil {
		t.Fatal("unknown user file removed")
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 1 {
		t.Fatal("state leftovers", entries, err)
	}
}
func TestCleanupRefusesChangesAndUnsafeSelection(t *testing.T) {
	root, f := setup(t)
	ctx := context.Background()
	p, err := Discover(ctx, root, f.run)
	if err != nil {
		t.Fatal(err)
	}
	selected := all()
	selected["projects"] = false
	if p.Validate(selected) == nil {
		t.Fatal("locator orphan allowed")
	}
	selected = all()
	selected["containers"] = false
	if p.Validate(selected) == nil {
		t.Fatal("live data deletion allowed")
	}
	f.items["volume:project"]["Labels"] = map[string]string{"cxz.owner": "foreign"}
	if Execute(ctx, p, all(), f.run, io.Discard) == nil || len(f.removed) != 0 {
		t.Fatal("ownership change not refused")
	}
	if _, err = os.Stat(filepath.Join(root, "installation.json")); err != nil {
		t.Fatal("locator lost")
	}
}
func TestLocalActiveProcessAndSymlink(t *testing.T) {
	root, f := setup(t)
	ctx := context.Background()
	lock, err := core.Lock(filepath.Join(root, "daemon.lock"))
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	p, err := Discover(ctx, root, f.run)
	if err != nil {
		t.Fatal(err)
	}
	if Execute(ctx, p, all(), f.run, io.Discard) == nil || len(f.removed) != 0 {
		t.Fatal("active native daemon ignored")
	}
	outside := t.TempDir()
	os.Symlink(outside, filepath.Join(root, "sessions"))
	if _, err = Discover(ctx, root, f.run); err == nil {
		t.Fatal("symlink accepted")
	}
	if _, err = Discover(ctx, "/", f.run); err == nil {
		t.Fatal("broad root accepted")
	}
}
func TestContainersOnlyRetainsState(t *testing.T) {
	root, f := setup(t)
	ctx := context.Background()
	p, err := Discover(ctx, root, f.run)
	if err != nil {
		t.Fatal(err)
	}
	if err = Execute(ctx, p, map[string]bool{"containers": true}, f.run, io.Discard); err != nil {
		t.Fatal(err)
	}
	if len(f.items) != 3 {
		t.Fatal("unselected resources removed")
	}
	if _, err = os.Stat(filepath.Join(root, "installation.json")); err != nil {
		t.Fatal("locator lost")
	}
}

func TestPartialFailureRetainsLocator(t *testing.T) {
	root, f := setup(t)
	ctx := context.Background()
	p, err := Discover(ctx, root, f.run)
	if err != nil {
		t.Fatal(err)
	}
	f.failRemove = "network:network-id"
	if err = Execute(ctx, p, all(), f.run, io.Discard); err == nil {
		t.Fatal("failure ignored")
	}
	if len(f.removed) != 2 {
		t.Fatal("unexpected partial removal", f.removed)
	}
	if _, err = os.Stat(filepath.Join(root, "installation.json")); err != nil {
		t.Fatal("locator removed on partial failure")
	}
}

func TestBusySupervisorPreventsAllDeletion(t *testing.T) {
	root, f := setup(t)
	ctx := context.Background()
	dir := filepath.Join(root, "sessions", "fixture")
	os.MkdirAll(dir, 0700)
	lock, err := core.Lock(filepath.Join(dir, "supervisor.lock"))
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	p, err := Discover(ctx, root, f.run)
	if err != nil {
		t.Fatal(err)
	}
	if Execute(ctx, p, all(), f.run, io.Discard) == nil || len(f.removed) > 0 {
		t.Fatal("active supervisor data was not protected")
	}
}
