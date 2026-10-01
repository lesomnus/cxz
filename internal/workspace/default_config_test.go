package workspace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMissingDevcontainerUsesManagedDefault(t *testing.T) {
	workspace := t.TempDir()
	m := &Manager{Root: t.TempDir()}
	p := &Project{ID: "test", Name: "test", Workspace: workspace}
	if err := preflight(p); err != nil {
		t.Fatal(err)
	}
	file, err := m.provisionConfiguration(p)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]any
	if json.Unmarshal(data, &cfg) != nil || cfg["image"] == "" || cfg["workspaceMount"] == "" {
		t.Fatal("incomplete default config", string(data))
	}
	// The built-in default is a template, so it is rendered like one: the name
	// resolves and cxz binds the workspace the template deliberately omits.
	if cfg["name"] != filepath.Base(workspace) {
		t.Fatal("project name not substituted", string(data))
	}
	if cfg["workspaceFolder"] != "/workspaces/"+filepath.Base(workspace) {
		t.Fatal("workspace folder not filled in", string(data))
	}
	if mount, _ := cfg["workspaceMount"].(string); !strings.Contains(mount, "source="+workspace+",") {
		t.Fatal("workspace not bound", string(data))
	}
	entries, _ := os.ReadDir(workspace)
	if len(entries) != 0 || p.Config != "" {
		t.Fatal("default modified the workspace or pinned a generated config")
	}
	real := filepath.Join(workspace, ".devcontainer.json")
	if err := os.WriteFile(real, []byte(`{"image":"alpine"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if got, err := m.provisionConfiguration(p); err != nil || got != real {
		t.Fatal("new real configuration not discovered", got, err)
	}
	p.Config = "missing-custom.json"
	if err := preflight(p); err == nil {
		t.Fatal("explicit missing config silently ignored")
	}
}
