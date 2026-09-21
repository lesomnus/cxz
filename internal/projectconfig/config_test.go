package projectconfig

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestInlineAndFileSnapshots(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home with spaces")
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	inline := `{"services":{"${DEVCONTAINER_SERVICE}":{"volumes":["${HOME}/workspaces:/workspaces"],"environment":{"LITERAL":"$${HOME}","OTHER":"${UNRELATED:-default}"}}}}`
	yaml := "services:\n  ${DEVCONTAINER_SERVICE}:\n    volumes:\n      - ${HOME}/workspaces:/workspaces\n    environment:\n      LITERAL: $${HOME}\n      OTHER: ${UNRELATED:-default}\n"
	path := filepath.Join(root, "override.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0600); err != nil {
		t.Fatal(err)
	}
	var first Spec
	for _, raw := range []string{inline, `"override.yaml"`, string(mustJSON(t, path))} {
		spec, err := (Config{Compose: json.RawMessage(raw)}).Snapshot(root)
		if err != nil {
			t.Fatal(err)
		}
		if first.Compose != nil && !reflect.DeepEqual(spec, first) {
			t.Fatal("file and inline contents differ")
		}
		first = spec
		b, err := spec.Render("actual-dev")
		if err != nil {
			t.Fatal(err)
		}
		var v struct {
			Services map[string]struct {
				Volumes     []string
				Environment map[string]string
			}
		}
		if err = json.Unmarshal(b, &v); err != nil {
			t.Fatal(err)
		}
		dev := v.Services["actual-dev"]
		if len(v.Services) != 1 || dev.Volumes[0] != home+"/workspaces:/workspaces" || dev.Environment["LITERAL"] != "$${HOME}" || dev.Environment["OTHER"] != "${UNRELATED:-default}" {
			t.Fatalf("incorrect expansion: %s", b)
		}
		if !strings.Contains(string(spec.Compose), ServiceVariable) {
			t.Fatal("render mutated saved snapshot")
		}
	}
	manager := t.TempDir()
	if err := Save(manager, first); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(manager)
	originalRender, _ := first.Render("dev")
	loadedRender, renderErr := loaded.Render("dev")
	if err != nil || renderErr != nil || string(originalRender) != string(loadedRender) {
		t.Fatal("snapshot depends on source file", loaded, err)
	}
	if err = Save(manager, Spec{}); err != nil {
		t.Fatal(err)
	}
	loaded, err = Load(manager)
	if err != nil || len(loaded.Compose) != 0 {
		t.Fatal("removing override did not clear saved settings", loaded, err)
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	b, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestInvalidOverridesAndServiceCollision(t *testing.T) {
	for _, raw := range []string{`""`, `42`, `[]`, `{"services":[]}`, `{"services":{"dev":null}}`, `{"services":{"dev":"wrong"}}`} {
		if err := (Config{Compose: json.RawMessage(raw)}).Validate(); err == nil {
			t.Fatal("accepted invalid override", raw)
		}
	}
	root := t.TempDir()
	for _, content := range []string{
		"services: [broken",
		"services:\n  dev: {}\n---\nservices:\n  other: {}\n",
		"services:\n  dev: {}\n  dev: {}\n",
		strings.Repeat(" ", MaxBytes+1),
	} {
		if err := os.WriteFile(filepath.Join(root, "bad.yaml"), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := (Config{Compose: json.RawMessage(`"bad.yaml"`)}).Snapshot(root); err == nil {
			t.Fatal("accepted invalid file", content[:min(len(content), 70)])
		}
	}
	spec := Spec{Compose: json.RawMessage(`{"services":{"${DEVCONTAINER_SERVICE}":{},"dev":{}}}`)}
	if _, err := spec.Render("dev"); err == nil {
		t.Fatal("service collision accepted")
	}
	if _, err := spec.Render(""); err == nil {
		t.Fatal("missing service accepted")
	}
	if _, err := (Config{Compose: json.RawMessage(`"missing.yaml"`)}).Snapshot(root); err == nil {
		t.Fatal("missing file ignored")
	}
}

func TestResourceOnlyOverride(t *testing.T) {
	raw := `{"volumes":{"go.cache.mod":{"external":true,"name":"go.cache.mod"}}}`
	spec, err := (Config{Compose: json.RawMessage(raw)}).Snapshot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	b, err := spec.Render("dev")
	if err != nil || string(b) != raw {
		t.Fatal("resource-only override lost", string(b), err)
	}
}

func TestHomePathAndFileRefresh(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	for _, path := range []string{"~/override.yaml", "${HOME}/override.yaml"} {
		cfg := Config{Compose: mustJSON(t, path)}
		for _, image := range []string{"first", "second"} {
			if err := os.WriteFile(filepath.Join(home, "override.yaml"), []byte("services:\n  dev:\n    image: "+image+"\n"), 0600); err != nil {
				t.Fatal(err)
			}
			spec, err := cfg.Snapshot(t.TempDir())
			if err != nil || !strings.Contains(string(spec.Compose), image) {
				t.Fatal("stale or wrong home snapshot", spec, err)
			}
		}
	}
}
