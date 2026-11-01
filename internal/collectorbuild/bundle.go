// Package collectorbuild ships a reproducible, minimal OCB build context.
package collectorbuild

import (
	"archive/tar"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/lesomnus/cxz/internal/resourceid"
	"github.com/lesomnus/cxz/internal/telemetryauth"
)

//go:embed assets
var assets embed.FS

func Files() (map[string][]byte, error) {
	out := map[string][]byte{}
	err := fs.WalkDir(assets, "assets", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		b, err := assets.ReadFile(path)
		if err != nil {
			return err
		}
		out[strings.TrimSuffix(path[len("assets/"):], ".txt")] = b
		return nil
	})
	out["identitylib/go.mod"] = []byte("module github.com/lesomnus/cxz\n\ngo 1.27.0\n")
	out["identitylib/internal/telemetryauth/identity.go"] = telemetryauth.Source
	out["identitylib/internal/resourceid/id.go"] = resourceid.Source
	return out, err
}
func Write(directory string) error {
	files, err := Files()
	if err != nil {
		return err
	}
	for path, b := range files {
		target := filepath.Join(directory, filepath.FromSlash(path))
		if err = os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		if err = os.WriteFile(target, b, 0644); err != nil {
			return err
		}
	}
	return nil
}
func Archive(w *tar.Writer, prefix string) error {
	files, err := Files()
	if err != nil {
		return err
	}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		b := files[name]
		if err = w.WriteHeader(&tar.Header{Name: prefix + name, Mode: 0644, Size: int64(len(b))}); err != nil {
			return err
		}
		if _, err = w.Write(b); err != nil {
			return err
		}
	}
	return nil
}
func ValidateDirectory(directory string) error {
	if directory == "" {
		return fmt.Errorf("collector build directory is required")
	}
	return nil
}
