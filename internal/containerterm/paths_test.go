package containerterm

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestPathListingSafeNames(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"한글 파일", ".hidden", "$(touch injected)", "semi;colon", "line\nbreak"} {
		if err := os.WriteFile(filepath.Join(root, name), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(root, "a directory"), 0700); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sh", "-c", listPathsScript, "paths", root)
	b, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	listing, err := parsePaths(b)
	if err != nil {
		t.Fatal(err)
	}
	if len(listing.Entries) != 5 || listing.Entries[0].Name != "a directory" || !listing.Entries[0].Directory {
		t.Fatal(listing)
	}
	found := map[string]bool{}
	for _, e := range listing.Entries {
		found[e.Name] = true
	}
	for _, name := range []string{"한글 파일", ".hidden", "$(touch injected)", "semi;colon"} {
		if !found[name] {
			t.Fatal("name lost", name)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "injected")); !os.IsNotExist(err) {
		t.Fatal("filename executed")
	}
}

func TestPathListingLimit(t *testing.T) {
	listing, err := parsePaths([]byte("d\x00folder\x00limit\x00\x00"))
	if err != nil || !listing.Truncated || len(listing.Entries) != 1 {
		t.Fatal(listing, err)
	}
	for _, raw := range []string{"d\x00foo", "d\x00a/b\x00", "x\x00foo\x00"} {
		if _, err := parsePaths([]byte(raw)); err == nil {
			t.Fatal("invalid response accepted", raw)
		}
	}
}
