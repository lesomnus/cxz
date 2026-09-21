package projectconfig

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultComposeAndEmptyTemplate(t *testing.T) {
	root := t.TempDir()
	cfg := Config{}
	if spec, err := cfg.Snapshot(root); err != nil || len(spec.Compose) != 0 {
		t.Fatal(spec, err)
	}
	path, err := cfg.Path(root)
	if err != nil || path != filepath.Join(root, Filename) {
		t.Fatal(path, err)
	}
	for _, content := range [][]byte{Template(), []byte("# disabled\n"), []byte("{}\n"), {}} {
		if err := os.WriteFile(path, content, 0600); err != nil {
			t.Fatal(err)
		}
		if spec, err := cfg.Snapshot(root); err != nil || len(spec.Compose) != 0 {
			t.Fatal("empty file enabled overrides", spec, err)
		}
	}
	if err := os.WriteFile(path, []byte("services:\n  dev:\n    environment:\n      EXAMPLE: active\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if spec, err := cfg.Snapshot(root); err != nil || !strings.Contains(string(spec.Compose), "active") {
		t.Fatal("default file not discovered", spec, err)
	}
}

func TestIncludeSnapshotsRelativePathsAndMerging(t *testing.T) {
	root := t.TempDir()
	subdir := filepath.Join(root, "shared")
	if err := os.Mkdir(subdir, 0700); err != nil {
		t.Fatal(err)
	}
	write := func(path, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, path), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(Filename, "include:\n  - path: [./shared/base.yaml, ./shared/local.yaml]\n")
	write("shared/base.yaml", "services:\n  ${DEVCONTAINER_SERVICE}:\n    volumes:\n      - ./src:/extra\n    environment:\n      KEEP: original\n      CHANGE: original\n")
	write("shared/local.yaml", "services:\n  ${DEVCONTAINER_SERVICE}:\n    environment:\n      CHANGE: edited\n")
	spec, err := (Config{}).Snapshot(root)
	if err != nil {
		t.Fatal(err)
	}
	b, err := spec.Render("editor")
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Services map[string]struct {
			Volumes     []struct{ Source, Target string }
			Environment []string
		}
	}
	if err = json.Unmarshal(b, &cfg); err != nil {
		t.Fatal(err)
	}
	dev := cfg.Services["editor"]
	env := make(map[string]string)
	for _, item := range dev.Environment {
		key, value, _ := strings.Cut(item, "=")
		env[key] = value
	}
	if len(dev.Volumes) != 1 || dev.Volumes[0].Source != filepath.Join(subdir, "src") || env["KEEP"] != "original" || env["CHANGE"] != "edited" {
		t.Fatalf("include semantics lost: %s", b)
	}
	if strings.Contains(string(b), `"include"`) || strings.Contains(string(b), `"command"`) || strings.Contains(string(b), `"entrypoint"`) {
		t.Fatalf("snapshot still references files or changes unspecified commands: %s", b)
	}
	write("shared/local.yaml", "services:\n  ${DEVCONTAINER_SERVICE}:\n    environment:\n      CHANGE: refreshed\n")
	next, err := (Config{}).Snapshot(root)
	if err != nil || !strings.Contains(string(next.Compose), "refreshed") {
		t.Fatal("included edits not refreshed", next, err)
	}
	if err := Save(root, next); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(subdir); err != nil {
		t.Fatal(err)
	}
	saved, err := Load(root)
	if err != nil || string(saved.Compose) != string(next.Compose) {
		t.Fatal("snapshot depends on source files", saved, err)
	}
	write(Filename, "include: [./missing.yaml]\n")
	if _, err := (Config{}).Snapshot(root); err == nil {
		t.Fatal("missing include ignored")
	}
	write(Filename, "include: [./docker-compose.yaml]\n")
	if _, err := (Config{}).Snapshot(root); err == nil {
		t.Fatal("include cycle ignored")
	}
}

func TestLegacyInlineIsPreservedForMigration(t *testing.T) {
	var cfg Config
	old := []byte(`{"compose":{"services":{"dev":{"volumes":["${HOME}/workspaces:/workspaces"]}}}}`)
	if err := json.Unmarshal(old, &cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := cfg.Snapshot(t.TempDir()); err == nil || !strings.Contains(err.Error(), "cxz edit docker-compose") {
		t.Fatal("legacy inline still active or lacks migration guidance", err)
	}
	got, err := json.Marshal(cfg)
	if err != nil || string(got) != string(old) {
		t.Fatal("legacy data lost", string(got), err)
	}
}
