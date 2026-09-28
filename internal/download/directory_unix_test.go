//go:build !windows

package download

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDirectoryUsesClientXDGSetting(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("XDG on Linux")
	}
	home, config := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", config)
	os.WriteFile(filepath.Join(config, "user-dirs.dirs"), []byte("XDG_DOWNLOAD_DIR=\"$HOME/받은 파일\"\n"), 0600)
	if got, err := Directory(); err != nil || got != filepath.Join(home, "받은 파일") {
		t.Fatal(got, err)
	}
	os.Remove(filepath.Join(config, "user-dirs.dirs"))
	if got, err := Directory(); err != nil || got != filepath.Join(home, "Downloads") {
		t.Fatal(got, err)
	}
}
