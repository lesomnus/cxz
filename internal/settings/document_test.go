package settings

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestCommentedSettingsAndDisabledTemplate(t *testing.T) {
	cfg, err := Parse(Template())
	if err != nil || !reflect.DeepEqual(cfg, Config{Schema: SchemaReference}) {
		t.Fatal("template changes defaults", cfg, err)
	}
	source := []byte(`// Leading comment
{
 /* block */ "connections": {
  "default": "work",
  "work": {"target":"ssh://work",}, // keep URL intact
 },
 "codex_model":"test-model",
} // after object
`)
	original := bytes.Clone(source)
	cfg, err = Parse(source)
	if err != nil || cfg.Connections.Entries["work"].Target != "ssh://work" || cfg.CodexModel != "test-model" {
		t.Fatal(cfg, err)
	}
	if !bytes.Equal(source, original) {
		t.Fatal("parsing modified the editor's original bytes")
	}
	for _, s := range []string{
		`{/*ok*/"typo":true,}`, `// only a comment`, `{"codex_model":"ok",}{}`, `[{},]`,
		`{"connections":{"work":{"target":"ssh://work","typo":true,},},}`, `{ /* unterminated`,
	} {
		if _, err := Parse([]byte(s)); err == nil {
			t.Fatal("invalid settings accepted", s)
		}
	}
}
func TestSaveSeedsAndRetainsExamples(t *testing.T) {
	root := t.TempDir()
	cfg := Config{Schema: SchemaReference, ClaudeModel: "first"}
	if err := Save(root, cfg); err != nil {
		t.Fatal(err)
	}
	cfg.ClaudeModel = "second"
	if err := Save(root, cfg); err != nil {
		t.Fatal(err)
	}
	cfg.ClaudeModel = ""
	if err := Save(root, cfg); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(root, "settings.jsonm"))
	if err != nil {
		t.Fatal(err)
	}
	for _, example := range []string{"ssh://work", "tcp://home-vpn:7349", "~/CLAUDE.md", `${STATE}`, "docker", "claude_model"} {
		if !strings.Contains(string(b), example) {
			t.Fatal("example lost", example, string(b))
		}
	}
	got, err := Load(root)
	if err != nil || !reflect.DeepEqual(got, cfg) {
		t.Fatal(got, err)
	}
}
func TestSavePreservesUnrelatedAndUnsetComments(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "settings.jsonm")
	original := []byte(`// header
{
 // model explanation
 "claude_model": "old", // model inline
 "connections": { // connection section
   "work": {"target": "ssh://work"}, /* work note */
 },
 // final note
}
`)
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ClaudeModel = "new"
	cfg.Schema = SchemaReference
	if err := Save(root, cfg); err != nil {
		t.Fatal(err)
	}
	cfg.ClaudeModel = ""
	if err := Save(root, cfg); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, comment := range []string{"header", "model explanation", "model inline", "connection section", "work note", "final note"} {
		if !strings.Contains(string(b), comment) {
			t.Fatal("comment lost", comment, string(b))
		}
	}
	got, err := Load(root)
	if err != nil || !reflect.DeepEqual(got, cfg) {
		t.Fatal(got, err)
	}
	// A no-op save does not reformat the document.
	if err := Save(root, cfg); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(b, after) {
		t.Fatal("no-op reformatted document")
	}
}

func TestUnsetRemovesCaseVariantsAndDuplicates(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "settings.jsonm"), []byte(`{"CLAUDE_MODEL":"first","claude_model":"last"}`), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ClaudeModel = ""
	if err = Save(root, cfg); err != nil {
		t.Fatal(err)
	}
	cfg, err = Load(root)
	if err != nil || cfg.ClaudeModel != "" {
		t.Fatal("unset restored an older duplicate", cfg, err)
	}
}
