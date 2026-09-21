package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lesomnus/cxz/internal/filemap"
)

func TestEditShareCreatesNestedFilesAndPreservesExistingContent(t *testing.T) {
	root := filepath.Join(t.TempDir(), "state with spaces 한글")
	for _, name := range []string{"CLAUDE.md", "foo/bar/baz.txt"} {
		path, err := editSharedFile(root, name, func(path string) error {
			if filepath.Ext(path) != filepath.Ext(name) {
				t.Fatal("editor lost extension", path)
			}
			return os.WriteFile(path, []byte("original"), 0600)
		})
		if err != nil || path != filepath.Join(root, "share", filepath.FromSlash(name)) {
			t.Fatal(path, err)
		}
		_, err = editSharedFile(root, name, func(path string) error {
			b, err := os.ReadFile(path)
			if err != nil || string(b) != "original" {
				t.Fatal("existing file was truncated", string(b), err)
			}
			return os.WriteFile(path, []byte("updated"), 0600)
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	bundle, err := filemap.Snapshot(root, []filemap.Mapping{{Src: "${CXZ_SHARE_DIR}", Dst: "${AGENT_CONFIG_DIR}"}})
	if err != nil || len(bundle.Files) != 2 {
		t.Fatal("editor metadata leaked into share directory", bundle, err)
	}
}

func TestEditShareRejectsPathsOutsideShareAndDirectories(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"", ".", "..", "../outside", "foo/../../outside", filepath.Join(root, "outside"), "bad\x00name"} {
		if _, err := editSharedFile(root, name, func(string) error { t.Fatal("opened invalid path", name); return nil }); err == nil {
			t.Fatal("accepted invalid share path", name)
		}
	}
	if err := os.MkdirAll(filepath.Join(root, "share", "directory"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := editSharedFile(root, "directory", func(string) error { t.Fatal("opened directory"); return nil }); err == nil {
		t.Fatal("directory accepted as file")
	}
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.Symlink(filepath.Dir(outside), filepath.Join(root, "share", "link")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := editSharedFile(root, "link/outside", func(string) error { t.Fatal("opened symlink"); return nil }); err == nil {
		t.Fatal("symlink accepted")
	}
	if _, err := os.Stat(outside); !os.IsNotExist(err) {
		t.Fatal("created file outside share", err)
	}
}

func TestSharedEditSnapshotsOnlyWhenMapped(t *testing.T) {
	root := t.TempDir()
	path, err := editSharedFile(root, "foo/bar.txt", func(path string) error { return os.WriteFile(path, []byte("new instructions"), 0600) })
	if err != nil {
		t.Fatal(err)
	}
	for _, src := range []string{"${CXZ_SHARE_DIR}/foo/bar.txt", "${CXZ_SHARE_DIR}/foo", "${CXZ_SHARE_DIR}"} {
		bundle, err := sharedEditMappings(root, path, []filemap.Mapping{{Src: src, Dst: "${AGENT_CONFIG_DIR}/shared", Agent: "claude"}})
		if err != nil || bundle == nil || len(bundle.Files) != 1 || string(bundle.Files[0].Content) != "new instructions" {
			t.Fatal(src, bundle, err)
		}
	}
	bundle, err := sharedEditMappings(root, path, []filemap.Mapping{{Src: "${CXZ_SHARE_DIR}/other", Dst: "${WORKSPACE}/other"}})
	if err != nil || bundle != nil {
		t.Fatal("unmapped edit should remain offline", bundle, err)
	}
}
