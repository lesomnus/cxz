//go:build !windows

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lesomnus/cxz/internal/settings"
	"github.com/lesomnus/xli/xlitest"
)

func TestEditSettingsSavesAndSnapshotsChangedMappings(t *testing.T) {
	root := t.TempDir()
	changed, bundle, err := editSettings(root, func(string) error { return nil })
	if err != nil || !changed || bundle != nil {
		t.Fatal(changed, bundle, err)
	}
	// Model-only editing stays offline, even with the default empty files array.
	changed, bundle, err = editSettings(root, func(p string) error {
		return os.WriteFile(p, []byte("{\n  \"files\": [],\n  \"claude_model\": \"new\"\n}\n"), 0600)
	})
	if err != nil || !changed || bundle != nil {
		t.Fatal(changed, bundle, err)
	}
	cfg, err := settings.Load(root)
	if err != nil || cfg.ClaudeModel != "new" {
		t.Fatal(cfg, err)
	}
	t.Setenv("HOME", root)
	if err := os.WriteFile(filepath.Join(root, "CLAUDE.md"), []byte("instructions"), 0600); err != nil {
		t.Fatal(err)
	}
	changed, bundle, err = editSettings(root, func(p string) error {
		return os.WriteFile(p, []byte(`{"files":[{"src":"~/CLAUDE.md","dst":"${AGENT_CONFIG_DIR}/CLAUDE.md","agent":"claude"}]}`), 0600)
	})
	if err != nil || !changed || bundle == nil || len(bundle.Files) != 1 || string(bundle.Files[0].Content) != "instructions" {
		t.Fatal(changed, bundle, err)
	}
	changed, bundle, err = editSettings(root, func(p string) error { return os.WriteFile(p, []byte(`{"files":[]}`), 0600) })
	if err != nil || !changed || bundle == nil || len(bundle.Files) != 0 {
		t.Fatal("removing last mapping must publish empty bundle", err)
	}
}
func TestEditCommandUsesEditorWithLiteralFilename(t *testing.T) {
	root := filepath.Join(t.TempDir(), "state with spaces $(false)")
	script := filepath.Join(t.TempDir(), "editor with spaces")
	if err := os.WriteFile(script, []byte("#!/bin/sh\n[ \"$1\" = --wait ] || exit 2\nprintf '%s' '{\"claude_model\":\"editor-model\"}' > \"$2\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("VISUAL", "'"+script+"' --wait")
	t.Setenv("EDITOR", "false")
	got := xlitest.Run(t, newRoot(root), "edit")
	if got.Err != nil {
		t.Fatal(got)
	}
	cfg, err := settings.Load(root)
	if err != nil || cfg.ClaudeModel != "editor-model" {
		t.Fatal(cfg, err)
	}
}
func TestEditMissingSourceRetainsDraft(t *testing.T) {
	root := t.TempDir()
	changed, _, err := editSettings(root, func(p string) error {
		return os.WriteFile(p, []byte(`{"files":[{"src":"/nonexistent-cxz-edit-source","dst":"${AGENT_CONFIG_DIR}/CLAUDE.md"}]}`), 0600)
	})
	if changed || err == nil {
		t.Fatal("missing source accepted")
	}
}
