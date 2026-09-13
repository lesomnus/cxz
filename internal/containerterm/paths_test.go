package containerterm

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
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

func TestPathMetadata(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "run"), nil, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("missing target", filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	b, err := exec.Command("sh", "-c", listPathsScript, "paths", root).Output()
	if err != nil {
		t.Fatal(err)
	}
	out, err := parsePaths(b)
	if err != nil || len(out.Entries) != 2 {
		t.Fatal(out, err)
	}
	if !out.Entries[0].Symlink || out.Entries[0].LinkTarget != "missing target" || !out.Entries[1].Executable {
		t.Fatal(out)
	}
}

func TestPathStreamingBeforeEOF(t *testing.T) {
	r, w := io.Pipe()
	defer r.Close()
	defer w.Close()
	first := make(chan PathListing, 1)
	done := make(chan error, 1)
	go func() { _, err := readPaths(r, func(p PathListing) { first <- p }); done <- err }()
	if _, err := w.Write([]byte("f\x00first\x00\x00")); err != nil {
		t.Fatal(err)
	}
	select {
	case p := <-first:
		if len(p.Entries) != 1 || p.Entries[0].Name != "first" {
			t.Fatal(p)
		}
	case <-time.After(time.Second):
		t.Fatal("waited for EOF before emitting first entry")
	}
	w.Close()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestPathListingLimit(t *testing.T) {
	listing, err := parsePaths([]byte("d\x00folder\x00\x00limit\x00\x00\x00"))
	if err != nil || !listing.Truncated || len(listing.Entries) != 1 {
		t.Fatal(listing, err)
	}
	for _, raw := range []string{"d\x00foo", "d\x00a/b\x00", "x\x00foo\x00"} {
		if _, err := parsePaths([]byte(raw)); err == nil {
			t.Fatal("invalid response accepted", raw)
		}
	}
}
