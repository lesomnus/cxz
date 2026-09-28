package containerterm

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestDownloadScriptPreservesBytesAndLiteralPaths(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("shell unavailable")
	}
	dir := t.TempDir()
	name := "한글 $(touch injected); report.bin"
	data := []byte{0, 255, 13, 10, 42}
	os.WriteFile(filepath.Join(dir, name), data, 0600)
	cmd := exec.Command("sh", "-c", downloadScript, "download", name)
	cmd.Dir = dir
	got, err := cmd.Output()
	if err != nil || !bytes.Equal(got, data) {
		t.Fatal(got, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "injected")); !os.IsNotExist(err) {
		t.Fatal("path executed")
	}
	for _, name := range []string{dir, filepath.Join(dir, "missing")} {
		if _, err := exec.Command("sh", "-c", downloadScript, "download", name).Output(); err == nil {
			t.Fatal("non-file accepted", name)
		}
	}
}
