package tui

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/lesomnus/cxz/internal/containerterm"
)

func hostHintPath(text string) (parent, query, quote string, ok bool) {
	if strings.ContainsAny(text, "\r\n\x00") {
		return
	}
	if strings.HasPrefix(text, `"`) || strings.HasPrefix(text, "'") {
		quote = text[:1]
		text = strings.TrimSuffix(text[1:], quote)
		if strings.Contains(text, quote) {
			return "", "", "", false
		}
	}
	if text == "" || text == "~" {
		return "~/", "", quote, true
	}
	if !filepath.IsAbs(text) && !strings.HasPrefix(text, "~/") && !(runtime.GOOS == "windows" && strings.HasPrefix(text, `~\`)) {
		return "", "", "", false
	}
	separators := "/"
	if runtime.GOOS == "windows" {
		separators += `\`
	}
	slash := strings.LastIndexAny(text, separators)
	if slash < 0 {
		return "", "", "", false
	}
	return text[:slash+1], text[slash+1:], quote, true
}

func localHostPath(path string) (string, error) {
	if strings.HasPrefix(path, "~/") || runtime.GOOS == "windows" && strings.HasPrefix(path, `~\`) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		path = filepath.Join(home, path[2:])
	}
	return path, nil
}

func readHostPaths(ctx context.Context, parent string) (containerterm.PathListing, error) {
	var listing containerterm.PathListing
	path, err := localHostPath(parent)
	if err != nil {
		return listing, err
	}
	if err := ctx.Err(); err != nil {
		return listing, err
	}
	dir, err := os.Open(path)
	if err != nil {
		return listing, err
	}
	defer dir.Close()
	entries, err := dir.ReadDir(2049)
	if err != nil && err != io.EOF {
		return listing, err
	}
	if len(entries) > 2048 {
		listing.Truncated = true
		entries = entries[:2048]
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return listing, err
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		item := containerterm.PathEntry{Name: entry.Name(), Directory: info.IsDir(), Executable: info.Mode()&0111 != 0, Symlink: info.Mode()&os.ModeSymlink != 0}
		if item.Symlink {
			target := filepath.Join(path, entry.Name())
			item.LinkTarget, _ = os.Readlink(target)
			if info, err := os.Stat(target); err == nil {
				item.Directory = info.IsDir()
			}
		}
		listing.Entries = append(listing.Entries, item)
	}
	return listing, nil
}
