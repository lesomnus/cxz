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
	if json.Unmarshal(data, &cfg) != nil || cfg["dockerComposeFile"] == nil {
		t.Fatal("incomplete default config", string(data))
	}
	// The built-in default is a template, so it is rendered like one.
	if cfg["name"] != filepath.Base(workspace) {
		t.Fatal("project name not substituted", string(data))
	}
	// The Compose file travels beside the materialized devcontainer.json and
	// carries the real workspace path, since it sits outside the workspace.
	compose, err := os.ReadFile(filepath.Join(filepath.Dir(file), cfg["dockerComposeFile"].(string)))
	if err != nil {
		t.Fatal("Compose file not materialized beside the config", err)
	}
	if !strings.Contains(string(compose), workspace+":") {
		t.Fatal("workspace not bound into the Compose file", string(compose))
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
