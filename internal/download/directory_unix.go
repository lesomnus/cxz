//go:build !windows

package download

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

func Directory() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if runtime.GOOS == "linux" {
		config := os.Getenv("XDG_CONFIG_HOME")
		if config == "" {
			config = filepath.Join(home, ".config")
		}
		if b, err := os.ReadFile(filepath.Join(config, "user-dirs.dirs")); err == nil {
			for _, line := range strings.Split(string(b), "\n") {
				key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
				if !ok || strings.TrimSpace(key) != "XDG_DOWNLOAD_DIR" {
					continue
				}
				dir, err := strconv.Unquote(strings.TrimSpace(value))
				if err != nil {
					continue
				}
				dir = strings.ReplaceAll(strings.ReplaceAll(dir, "${HOME}", home), "$HOME", home)
				if filepath.IsAbs(dir) && !strings.ContainsAny(dir, "\x00\r\n") {
					return dir, nil
				}
			}
		}
	}
	return filepath.Join(home, "Downloads"), nil
}
