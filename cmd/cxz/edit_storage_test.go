package main

import (
	"bytes"
	"encoding/json"
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
			path := filepath.Join(root, "settings.jsonm")
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
	current, err := os.ReadFile(filepath.Join(root, "settings.jsonm"))
	if err != nil || string(current) != string(cfg) {
		t.Fatal("settings overwritten", err)
	}
	if _, err = os.Stat(draft); err != nil {
		t.Fatal("conflicting draft removed", err)
	}
}

func TestFirstEditCreatesExamplesAndKeepsComments(t *testing.T) {
	root := t.TempDir()
	changed, bundle, err := editSettings(root, func(p string) error {
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if !strings.Contains(string(b), "//") || !strings.Contains(string(b), "ssh://work") {
			t.Fatal("first draft has no examples")
		}
		cfg, err := settings.Parse(b)
		if err != nil || cfg.Connections != nil || cfg.Schema != settings.SchemaReference {
			t.Fatal("examples became active", cfg, err)
		}
		if filepath.Ext(p) != ".jsonm" {
			t.Fatal("wrong editor draft extension", p)
		}
		schema, err := os.ReadFile(filepath.Join(filepath.Dir(p), cfg.Schema))
		if err != nil || !json.Valid(schema) {
			t.Fatal("editor schema unavailable before editor opened", err)
		}
		return nil
	})
	if err != nil || !changed || bundle != nil {
		t.Fatal(changed, bundle, err)
	}
	path := filepath.Join(root, "settings.jsonm")
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	changed, _, err = editSettings(root, func(string) error { return nil })
	if err != nil || changed {
		t.Fatal("unmodified existing file was rewritten", err)
	}
	changed, _, err = editSettings(root, func(p string) error {
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		b = bytes.Replace(b, []byte("// \"codex_model\": \"\""), []byte("\"codex_model\": \"test-model\""), 1)
		return os.WriteFile(p, b, 0600)
	})
	if err != nil || !changed {
		t.Fatal(changed, err)
	}
	cfg, err := settings.Load(root)
	if err != nil || cfg.CodexModel != "test-model" {
		t.Fatal(cfg, err)
	}
	after, _ := os.ReadFile(path)
	if !bytes.Contains(after, []byte("ssh://work")) || bytes.Equal(first, after) {
		t.Fatal("comments lost or edited setting discarded")
	}
}

func TestEditMigratesLegacySettingsAndDetectsBothWriters(t *testing.T) {
	for _, concurrent := range []string{"none", "legacy", "primary"} {
		t.Run(concurrent, func(t *testing.T) {
			root := t.TempDir()
			legacy := filepath.Join(root, settings.LegacyFilename)
			original := []byte("{\n // keep\n \"claude_model\":\"legacy\",\n}\n")
			if err := os.WriteFile(legacy, original, 0600); err != nil {
				t.Fatal(err)
			}
			var draft string
			changed, bundle, err := editSettings(root, func(p string) error {
				draft = p
				switch concurrent {
				case "legacy":
					return os.WriteFile(legacy, []byte(`{"codex_model":"concurrent"}`), 0600)
				case "primary":
					// Even equal bytes in a newly created primary represent a new writer.
					return os.WriteFile(settings.Path(root), original, 0600)
				}
				return nil
			})
			if concurrent != "none" {
				if err == nil || changed || !strings.Contains(err.Error(), "settings changed") {
					t.Fatal("migration overwrote a concurrent writer", changed, err)
				}
				if _, err := os.Stat(draft); err != nil {
					t.Fatal("conflicting draft removed", err)
				}
				return
			}
			if err != nil || !changed || bundle != nil {
				t.Fatal(changed, bundle, err)
			}
			cfg, err := settings.Load(root)
			if err != nil || cfg.ClaudeModel != "legacy" || cfg.Schema != settings.SchemaReference {
				t.Fatal(cfg, err)
			}
			old, err := os.ReadFile(legacy)
			if err != nil || !bytes.Equal(old, original) {
				t.Fatal("legacy backup modified", err)
			}
			current, err := os.ReadFile(settings.Path(root))
			if err != nil || !bytes.Contains(current, []byte("// keep")) {
				t.Fatal("migration lost comments", err)
			}
		})
	}
}

func TestEditRefreshesSchemaWithoutChangingSettings(t *testing.T) {
	root := t.TempDir()
	if _, _, err := editSettings(root, func(string) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, settings.SchemaFilename), []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	changed, _, err := editSettings(root, func(p string) error {
		b, err := os.ReadFile(filepath.Join(filepath.Dir(p), settings.SchemaFilename))
		if err != nil || !bytes.Contains(b, []byte(`"properties"`)) {
			t.Fatal("old schema not refreshed before editing", err)
		}
		return nil
	})
	if err != nil || changed {
		t.Fatal("schema refresh rewrote user preferences", changed, err)
	}
}
