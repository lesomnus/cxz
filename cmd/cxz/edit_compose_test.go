package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lesomnus/cxz/internal/projectconfig"
	"github.com/lesomnus/cxz/internal/settings"
)

func TestComposeEditorDefaultCustomAndIncludes(t *testing.T) {
	root := t.TempDir()
	path, changed, err := editDockerCompose(root, func(draft string) error {
		b, err := os.ReadFile(draft)
		if err != nil || !strings.Contains(string(b), "# include:") {
			t.Fatal("template missing", err)
		}
		return nil
	})
	if err != nil || !changed || path != filepath.Join(root, projectconfig.Filename) {
		t.Fatal(path, changed, err)
	}
	if _, err := os.Stat(settings.Path(root)); !os.IsNotExist(err) {
		t.Fatal("default editor changed settings", err)
	}
	if spec, err := (projectconfig.Config{}).Snapshot(root); err != nil || len(spec.Compose) != 0 {
		t.Fatal("default template activates override", spec, err)
	}
	path, changed, err = editDockerCompose(root, func(string) error { return nil })
	if err != nil || changed {
		t.Fatal(path, changed, err)
	}
	if err := settings.Save(root, settings.Config{Devcontainer: projectconfig.Config{Compose: "custom/compose.yaml"}}); err != nil {
		t.Fatal(err)
	}
	path, changed, err = editDockerCompose(root, func(draft string) error {
		if err := os.WriteFile(filepath.Join(filepath.Dir(draft), "extra.yaml"), []byte("services:\n  dev:\n    environment:\n      SAMPLE: included\n"), 0600); err != nil {
			return err
		}
		return os.WriteFile(draft, []byte("include: [./extra.yaml]\n"), 0600)
	})
	if err != nil || !changed || path != filepath.Join(root, "custom", "compose.yaml") {
		t.Fatal(path, changed, err)
	}
}

func TestComposeEditorRetainsInvalidAndConflictingDrafts(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, projectconfig.Filename)
	if err := os.WriteFile(path, []byte("services: {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	_, changed, err := editDockerCompose(root, func(draft string) error { return os.WriteFile(draft, []byte("include: [./missing.yaml]\n"), 0600) })
	if err == nil || changed || !strings.Contains(err.Error(), "draft kept") {
		t.Fatal(changed, err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "services: {}\n" {
		t.Fatal("invalid draft overwrote original")
	}
	_, changed, err = editDockerCompose(root, func(draft string) error {
		if err := os.WriteFile(path, []byte("# concurrent edit\nservices: {}\n"), 0600); err != nil {
			return err
		}
		return os.WriteFile(draft, []byte("services: {}\n"), 0600)
	})
	if err == nil || changed || !strings.Contains(err.Error(), "changed while editing") {
		t.Fatal(changed, err)
	}
	got, _ = os.ReadFile(path)
	if !strings.Contains(string(got), "concurrent edit") {
		t.Fatal("concurrent edit lost")
	}
}

func TestComposeEditorMigratesInlineWithoutLosingSettings(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(settings.Path(root), []byte(`{
// Keep the model preference.
"claude_model":"keep",
"devcontainer":{"compose":{"services":{"${DEVCONTAINER_SERVICE}":{"environment":{"PRESERVED":"yes"}}}}}
}`), 0600); err != nil {
		t.Fatal(err)
	}
	path, changed, err := editDockerCompose(root, func(draft string) error {
		b, err := os.ReadFile(draft)
		if err != nil || !strings.Contains(string(b), "PRESERVED") {
			t.Fatal("inline content missing in editor", err)
		}
		return nil
	})
	if err != nil || !changed || path != filepath.Join(root, projectconfig.Filename) {
		t.Fatal(path, changed, err)
	}
	cfg, err := settings.Load(root)
	if err != nil || cfg.ClaudeModel != "keep" || cfg.Devcontainer.Compose != projectconfig.Filename || len(cfg.Devcontainer.LegacyInline) != 0 {
		t.Fatal(cfg, err)
	}
	b, _ := os.ReadFile(settings.Path(root))
	if !strings.Contains(string(b), "Keep the model") {
		t.Fatal("unrelated comment lost")
	}
	if spec, err := cfg.Devcontainer.Snapshot(root); err != nil || !strings.Contains(string(spec.Compose), "PRESERVED") {
		t.Fatal("migrated settings do not work", spec, err)
	}
}

func TestComposeEditorRejectsSettingsChange(t *testing.T) {
	root := t.TempDir()
	_, changed, err := editDockerCompose(root, func(string) error {
		return settings.Save(root, settings.Config{Devcontainer: projectconfig.Config{Compose: "other.yaml"}})
	})
	if err == nil || changed || !strings.Contains(err.Error(), "settings changed") {
		t.Fatal(changed, err)
	}
	if _, err := os.Stat(filepath.Join(root, projectconfig.Filename)); !os.IsNotExist(err) {
		t.Fatal("saved to stale selection", err)
	}
}
