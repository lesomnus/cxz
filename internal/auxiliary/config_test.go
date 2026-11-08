package auxiliary

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/lesomnus/cxz/internal/core"
)

func TestLegacyAccountsMigrateToOnePersistedIdentity(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "auxiliary")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	chosen := profile()
	other := profile()
	other.Account = "other"
	other.Enabled = false
	if err := core.WriteJSON(filepath.Join(dir, "config.json"), Config{Revision: "old", Summary: other, Suggestion: chosen, Title: chosen}); err != nil {
		t.Fatal(err)
	}
	c, err := New(root, func(context.Context, Input) (Output, error) {
		t.Fatal("migration ran a provider")
		return Output{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	cfg := c.Config()
	if cfg.Account != chosen.Account || cfg.Revision == "old" || cfg.Summary.Enabled || cfg.Summary.Model != "" || !cfg.Suggestion.Enabled || cfg.Title != chosen {
		t.Fatalf("unexpected migration: %+v", cfg)
	}
	for _, p := range []Profile{cfg.Summary, cfg.Suggestion, cfg.Title} {
		if p.Account != chosen.Account || p.Agent != chosen.Agent || p.Backend != chosen.Backend {
			t.Fatalf("mixed identity: %+v", cfg)
		}
	}
	c.Close()
	reopened, err := New(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if reopened.Config() != cfg {
		t.Fatal("migration was not stable across restart")
	}
}

func TestSharedAccountChangeResetsOtherTaskModels(t *testing.T) {
	c, err := New(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	p := profile()
	if _, err = c.SaveProfiles(map[string]Profile{"summary": p, "suggestion": p, "title": p}); err != nil {
		t.Fatal(err)
	}
	p.Model = "another-model"
	if _, err = c.Save("title", p); err != nil {
		t.Fatal(err)
	}
	if c.Config().Summary.Model != "test" {
		t.Fatal("same-account model change reset another task")
	}
	p.Account = "other"
	if _, err = c.Save("title", p); err != nil {
		t.Fatal(err)
	}
	cfg := c.Config()
	if cfg.Account != "other" || cfg.Title != p {
		t.Fatal(cfg)
	}
	for _, p := range []Profile{cfg.Summary, cfg.Suggestion} {
		if p.Account != "other" || p.Enabled || p.Model != "" || p.Effort != "" {
			t.Fatal("kept an old-account model", p)
		}
	}
}

func TestSharedAccountSaveIsAtomic(t *testing.T) {
	c, err := New(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	p := profile()
	if _, err = c.Save("summary", p); err != nil {
		t.Fatal(err)
	}
	before := c.Config()
	other := p
	other.Account = "other"
	invalid := p
	invalid.Model = ""
	for _, updates := range []map[string]Profile{
		{"summary": p, "suggestion": other},
		{"summary": other, "suggestion": invalid},
		{"unknown": p},
	} {
		if _, err = c.SaveProfiles(updates); err == nil {
			t.Fatal("accepted invalid update", updates)
		}
		if c.Config() != before {
			t.Fatal("partially saved failed request")
		}
	}
}

func TestErasingSharedAccountClearsEveryTask(t *testing.T) {
	c, err := New(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	p := profile()
	if _, err = c.SaveProfiles(map[string]Profile{"summary": p, "suggestion": p, "title": p}); err != nil {
		t.Fatal(err)
	}
	before := c.Config()
	if err = c.ForgetAccount("other"); err != nil || c.Config() != before {
		t.Fatal("unrelated account changed settings", err)
	}
	if err = c.ForgetAccount("work"); err != nil {
		t.Fatal(err)
	}
	cfg := c.Config()
	if cfg.Configured() || cfg.Account != "" || cfg.Summary.Enabled || cfg.Suggestion.Enabled || cfg.Title.Enabled {
		t.Fatal("kept erased credentials", cfg)
	}
}
