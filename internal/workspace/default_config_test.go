package workspace

import (
	"encoding/json"
	"os"
	"path/filepath"
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
	var cfg map[string]string
	if json.Unmarshal(data, &cfg) != nil || cfg["image"] == "" || cfg["workspaceMount"] == "" {
		t.Fatal("incomplete default config", string(data))
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
