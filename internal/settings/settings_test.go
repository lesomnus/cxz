package settings

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

func TestSettings(t *testing.T) {
	root := t.TempDir()
	c, err := Load(root)
	if err != nil || c.Agent != "" {
		t.Fatal(c, err)
	}
	c = Config{Agent: "codex", CodexModel: "test-model"}
	if err = Save(root, c); err != nil {
		t.Fatal(err)
	}
	c.Schema = SchemaReference
	got, err := Load(root)
	if err != nil || !reflect.DeepEqual(got, c) {
		t.Fatal(got, err)
	}
	st, err := os.Stat(filepath.Join(root, "settings.jsonm"))
	if err != nil || (runtime.GOOS != "windows" && st.Mode().Perm() != 0600) {
		t.Fatal("unsafe settings mode", err)
	}
	for _, value := range []string{"bad\nmodel", "--injected", "model name"} {
		if ValidateModel(value) == nil {
			t.Fatal("accepted", value)
		}
	}
	for _, raw := range []string{`{"agent":"other"}`, `{"token":"secret"}`, `{} {}`} {
		if err = os.WriteFile(filepath.Join(root, "settings.jsonm"), []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err = Load(root); err == nil {
			t.Fatal("accepted", raw)
		}
	}
}
