package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lesomnus/cxz/internal/filemap"
	"github.com/lesomnus/xli"
	"github.com/lesomnus/xli/arg"
	"github.com/lesomnus/xli/flg"
)

func editShareCommand() *xli.Command {
	return &xli.Command{Name: "share", Brief: "Edit a file in ${CXZ_SHARE_DIR}; create parent directories as needed", Args: arg.Args{&arg.String{Name: "PATH", Brief: "Relative path inside the shared directory, e.g. CLAUDE.md or foo/bar.txt"}}, Handler: localEditHandler(func(ctx context.Context, c *xli.Command) error {
		rootCommand := c
		for rootCommand.HasParent() {
			rootCommand = rootCommand.Parent()
		}
		root, err := filepath.Abs(flg.MustGet[string](rootCommand, "state"))
		if err != nil {
			return err
		}
		path, err := editSharedFile(root, arg.MustGet[string](c, "PATH"), func(path string) error { return openSettingsEditor(ctx, c, path) })
		if err != nil {
			return err
		}
		fmt.Fprintln(c.Writer, "Saved", path)
		return finishSharedEdit(ctx, c, root, path)
	})}
}

// Shared files are ordinary source files: open the actual path in the editor,
// without putting temporary drafts or lock files into a mapped directory.
func editSharedFile(root, name string, open func(string) error) (string, error) {
	if !filepath.IsLocal(name) || filepath.Clean(name) == "." || strings.ContainsAny(name, "\x00\r\n") {
		return "", fmt.Errorf("share path must name a relative file inside ${CXZ_SHARE_DIR}")
	}
	name = filepath.Clean(name)
	share, err := filepath.Abs(filemap.ShareDir(root))
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(share, 0700); err != nil {
		return "", err
	}
	dir, err := os.OpenRoot(share)
	if err != nil {
		return "", err
	}
	defer dir.Close()
	// File mappings only accept regular sources. Reject symlink parents too so
	// editing and later copying refer to the same file within the share tree.
	for part := name; part != "."; part = filepath.Dir(part) {
		info, err := dir.Lstat(part)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return "", err
		}
		if info.Mode()&os.ModeSymlink != 0 || (part == name && !info.Mode().IsRegular()) || (part != name && !info.IsDir()) {
			return "", fmt.Errorf("share path requires regular files and directories: %s", part)
		}
	}
	if err := dir.MkdirAll(filepath.Dir(name), 0700); err != nil {
		return "", err
	}
	f, err := dir.OpenFile(name, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	path := filepath.Join(share, name)
	return path, open(path)
}

func sharedEditMappings(root, path string, mappings []filemap.Mapping) (*filemap.Bundle, error) {
	for _, mapping := range mappings {
		src, err := filemap.SourcePath(root, mapping.Src)
		if err != nil {
			return nil, err
		}
		rel, err := filepath.Rel(src, path)
		if err == nil && filepath.IsLocal(rel) {
			bundle, err := filemap.Snapshot(root, mappings)
			return &bundle, err
		}
	}
	return nil, nil
}
