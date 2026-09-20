package core

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLockRejectsSecondWriterAndReleasesOnClose(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.lock")
	first, err := Lock(path)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	if second, err := Lock(path); err == nil {
		second.Close()
		t.Fatal("two writers acquired the same lock")
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := Lock(path)
	if err != nil {
		t.Fatal("lock not released", err)
	}
	second.Close()
}
func TestWriteJSONReplacesSavedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	for _, s := range []string{"first", "second"} {
		if err := WriteJSON(path, s); err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(path)
		if err != nil || string(b) != `"`+s+`"` {
			t.Fatal(string(b), err)
		}
	}
}
