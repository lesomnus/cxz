package settings

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestBundledSchemaAndOfflineRefresh(t *testing.T) {
	var schema struct {
		Dialect    string                     `json:"$schema"`
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(schemaDocument, &schema); err != nil {
		t.Fatal(err)
	}
	if schema.Dialect != "http://json-schema.org/draft-07/schema#" {
		t.Fatal("missing schema dialect")
	}
	// Every supported top-level setting must remain discoverable in the editor.
	typ := reflect.TypeOf(Config{})
	if len(schema.Properties) != typ.NumField() {
		t.Fatal("schema fields differ from supported settings")
	}
	for i := 0; i < typ.NumField(); i++ {
		name, _, _ := strings.Cut(typ.Field(i).Tag.Get("json"), ",")
		if schema.Properties[name] == nil {
			t.Fatal("missing editor schema for", name)
		}
	}
	root := filepath.Join(t.TempDir(), "state with spaces 한글")
	if err := EnsureSchema(root); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, SchemaFilename)
	original, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(original, schemaDocument) {
		t.Fatal("schema not extracted", err)
	}
	old := time.Unix(1_700_000_000, 0)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	if err := EnsureSchema(root); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(path)
	if err != nil || !st.ModTime().Equal(old) {
		t.Fatal("unchanged schema rewritten", err)
	}
	if err := os.WriteFile(path, []byte(`{"title":"older binary"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := EnsureSchema(root); err != nil {
		t.Fatal(err)
	}
	updated, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(updated, original) {
		t.Fatal("stale schema not refreshed", err)
	}
}

func TestSchemaMetadataAndCommentPreservation(t *testing.T) {
	original := []byte("{\n // Keep this connection.\n \"connections\": {\"work\":{\"target\":\"ssh://work\"}},\n}\n")
	unchanged := bytes.Clone(original)
	updated, err := WithSchema(original)
	if err != nil || !bytes.Contains(updated, []byte("// Keep this connection.")) {
		t.Fatal("lost comment", err)
	}
	if !bytes.Equal(original, unchanged) {
		t.Fatal("adding schema modified the original conflict-check bytes")
	}
	cfg, err := Parse(updated)
	if err != nil || cfg.Schema != SchemaReference || cfg.Connections.Entries["work"].Target != "ssh://work" {
		t.Fatal(cfg, err)
	}
	custom := []byte(`{"$schema":"./custom.schema.json",/* custom */"codex_model":"model"}`)
	updated, err = WithSchema(custom)
	if err != nil || !bytes.Equal(custom, updated) {
		t.Fatal("custom schema was replaced or reformatted", err)
	}
	for _, invalid := range []string{`{"$schema":42}`, `{"$scheme":"./settings.schema.json"}`} {
		if _, err := Parse([]byte(invalid)); err == nil {
			t.Fatal("invalid schema metadata accepted", invalid)
		}
	}
}
