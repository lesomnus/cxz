package projectconfig

import (
	"embed"
	"fmt"
	"io/fs"
	"strings"
)

// The built-in default is a directory on disk rather than a literal in Go, so
// it can be read, diffed and copied as the devcontainer it actually is. A real
// devcontainer rarely is one file, and the day this one needs a Dockerfile
// beside it, the shape already allows for it.
//
//go:embed devcontainer
var builtin embed.FS

// Builtin is the devcontainer cxz gives a project that brings none of its own.
// It is a DefaultTemplate like any user's, so one code path materializes both
// and the fallback obeys the same rules the documentation states for templates.
func Builtin() (*DefaultTemplate, error) {
	t := &DefaultTemplate{Files: map[string]TemplateFile{}}
	prefix := TemplateDirectory + "/"
	err := fs.WalkDir(builtin, TemplateDirectory, func(file string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		b, err := builtin.ReadFile(file)
		if err != nil {
			return err
		}
		// embed.FS always uses forward slashes, so this is a path operation.
		name, ok := strings.CutPrefix(file, prefix)
		if !ok || name == "" {
			return fmt.Errorf("built-in devcontainer file outside %s: %s", TemplateDirectory, file)
		}
		t.Files[name] = TemplateFile{Data: b}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if err := t.Validate(); err != nil {
		return nil, fmt.Errorf("built-in devcontainer: %w", err)
	}
	return t, nil
}
