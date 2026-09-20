package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/settings"
)

func TestEditSettingsPreservesInvalidAndConcurrentEdits(t *testing.T) {
	for _, kind := range []string{"invalid", "unknown", "cancel", "concurrent"} {
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

func TestEditClientSettingsOfflineAndLockConflict(t *testing.T) {
	root := t.TempDir()
	cfg := []byte(`{"connections":{"default":"work","work":{"target":"ssh://work"}},"claude_model":"test-model"}`)
	changed, bundle, err := editSettings(root, func(p string) error { return os.WriteFile(p, cfg, 0600) })
	if err != nil || !changed || bundle != nil {
		t.Fatal(changed, bundle, err)
	}
	saved, err := settings.Load(root)
	if err != nil || saved.Connections.DefaultName() != "work" || saved.ClaudeModel != "test-model" {
		t.Fatal(saved, err)
	}
	lock, err := core.Lock(filepath.Join(root, "settings.lock"))
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	var draft string
	changed, _, err = editSettings(root, func(p string) error { draft = p; return os.WriteFile(p, []byte(`{"claude_model":"overwrite"}`), 0600) })
	if err == nil || changed {
		t.Fatal("concurrent writer was allowed")
	}
	current, err := os.ReadFile(filepath.Join(root, "settings.json"))
	if err != nil || string(current) != string(cfg) {
		t.Fatal("settings overwritten", err)
	}
	if _, err = os.Stat(draft); err != nil {
		t.Fatal("conflicting draft removed", err)
	}
}
