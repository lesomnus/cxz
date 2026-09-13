package server

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAttachmentStorage(t *testing.T) {
	dir := t.TempDir()
	body := []byte("first\n한글\n")
	path, err := storeAttachment(dir, body)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(path) != filepath.Join(dir, "attachments") {
		t.Fatal(path)
	}
	b, err := os.ReadFile(path)
	if err != nil || string(b) != string(body) {
		t.Fatal("changed content", err)
	}
	st, _ := os.Stat(path)
	if st.Mode().Perm() != 0600 {
		t.Fatal(st.Mode())
	}
	again, err := storeAttachment(dir, body)
	if err != nil || again != path {
		t.Fatal("retry not idempotent", err)
	}
	other, err := storeAttachment(t.TempDir(), body)
	if err != nil || other == path {
		t.Fatal("session path not isolated")
	}
}

func TestAttachmentCannotFollowOutsideSymlink(t *testing.T) {
	dir, outside := t.TempDir(), t.TempDir()
	if err := os.Symlink(outside, filepath.Join(dir, "attachments")); err != nil {
		t.Skip(err)
	}
	if _, err := storeAttachment(dir, []byte("private")); err == nil {
		t.Fatal("escaped session root")
	}
	files, _ := os.ReadDir(outside)
	if len(files) != 0 {
		t.Fatal("wrote outside session")
	}
}
