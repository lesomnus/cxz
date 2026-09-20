//go:build !windows

package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lesomnus/cxz/internal/settings"
	"github.com/lesomnus/xli/xlitest"
)

func TestEditSettingsPreservesInvalidAndConcurrentEdits(t *testing.T) {
	for _, kind := range []string{"invalid", "unknown", "missing-source", "cancel", "concurrent"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			original := []byte("{\"claude_model\":\"old\"}\n")
			path := filepath.Join(root, "settings.json")
			if err := os.WriteFile(path, original, 0600); err != nil {
				t.Fatal(err)
			}
			var draft string
			changed, _, err := editSettings(root, func(p string) error {
				draft = p
				edit := `{"claude_model":"new"}`
				switch kind {
				case "invalid":
					edit = "{"
				case "unknown":
					edit = `{"typo":true}`
				case "missing-source":
					edit = `{"files":[{"src":"/nonexistent-cxz-edit-source","dst":"${AGENT_CONFIG_DIR}/CLAUDE.md"}]}`
				case "cancel":
					return errors.New("editor cancelled")
				case "concurrent":
					if err := os.WriteFile(path, []byte(`{"codex_model":"concurrent"}`), 0600); err != nil {
						return err
					}
				}
				return os.WriteFile(p, []byte(edit), 0600)
			})
			if err == nil || changed || !strings.Contains(err.Error(), draft) {
				t.Fatal("lost invalid draft or accepted edit", err)
			}
			if _, err := os.Stat(draft); err != nil {
				t.Fatal("draft removed", err)
			}
			b, _ := os.ReadFile(path)
			want := string(original)
			if kind == "concurrent" {
				want = `{"codex_model":"concurrent"}`
			}
			if string(b) != want {
				t.Fatal("overwrote saved settings")
			}
		})
	}
}
func TestEditSettingsSavesAndSnapshotsChangedMappings(t *testing.T) {
	root := t.TempDir()
	changed, bundle, err := editSettings(root, func(string) error { return nil })
	if err != nil || changed || bundle != nil {
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
