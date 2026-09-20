package settings

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestLegacySettingsMigrationAndPrecedence(t *testing.T) {
	for _, name := range []string{LegacyFilename, "settings.jsonm"} {
		t.Run(name, func(t *testing.T) { testLegacySettingsMigration(t, name) })
	}
}

func testLegacySettingsMigration(t *testing.T, name string) {
	t.Helper()
	root := t.TempDir()
	legacy := filepath.Join(root, name)
	original := []byte("{\n // My model.\n \"claude_model\":\"legacy\",\n}\n")
	if err := os.WriteFile(legacy, original, 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(root)
	if err != nil || cfg.ClaudeModel != "legacy" {
		t.Fatal("legacy settings not loaded", cfg, err)
	}
	if _, err := os.Stat(Path(root)); !os.IsNotExist(err) {
		t.Fatal("read-only load migrated settings", err)
	}
	cfg.ClaudeModel = "new"
	if err := Save(root, cfg); err != nil {
		t.Fatal(err)
	}
	cfg, err = Load(root)
	if err != nil || cfg.ClaudeModel != "new" || cfg.Schema != SchemaReference {
		t.Fatal("new settings not preferred", cfg, err)
	}
	old, err := os.ReadFile(legacy)
	if err != nil || !bytes.Equal(old, original) {
		t.Fatal("legacy backup modified", err)
	}
	current, err := os.ReadFile(Path(root))
	if err != nil || !bytes.Contains(current, []byte("// My model.")) {
		t.Fatal("migration lost comment", err)
	}
	if _, err := os.Stat(filepath.Join(root, cfg.Schema)); err != nil {
		t.Fatal("local schema reference does not resolve", err)
	}
	if err := os.WriteFile(Path(root), []byte(`{"typo":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(root); err == nil {
		t.Fatal("invalid primary silently fell back to old preferences")
	}
}

func TestSettingsFilenamePrecedence(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{LegacyFilename, "settings.jsonm", Filename} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(`{"claude_model":"`+name+`"}`), 0600); err != nil {
			t.Fatal(err)
		}
		cfg, err := Load(root)
		if err != nil || cfg.ClaudeModel != name {
			t.Fatal("newer filename not preferred", name, cfg, err)
		}
	}
}
