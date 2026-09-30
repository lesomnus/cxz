package assets

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lesomnus/flob"
)

func TestSessionAssetsPreserveNamesAndShareContent(t *testing.T) {
	root := t.TempDir()
	ctx := t.Context()
	put := func(session, name, body string) string {
		t.Helper()
		path, err := Add(ctx, root, "project", session, name, int64(len(body)), strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		if filepath.Base(path) != name {
			t.Fatalf("filename changed: %q", path)
		}
		data, err := os.ReadFile(path)
		if err != nil || string(data) != body {
			t.Fatal("export unavailable", err)
		}
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != 0444 {
			t.Fatal("export not readable/immutable", err)
		}
		return path
	}
	a := put("one", "한글 report.txt", "same")
	b := put("one", "renamed.txt", "same")
	c := put("two", "other.txt", "same")
	d := put("one", "한글 report.txt", "different")
	e := put("one", "한글 report.txt", "same")
	ai, _ := os.Stat(a)
	for _, path := range []string{b, c, e} {
		info, _ := os.Stat(path)
		if a == path || !os.SameFile(ai, info) {
			t.Fatal("attachment identity or content dedup broken")
		}
	}
	di, _ := os.Stat(d)
	if d == a || os.SameFile(ai, di) {
		t.Fatal("same name overwrote different content")
	}
	if data, _ := os.ReadFile(a); string(data) != "same" {
		t.Fatal("previous attachment changed")
	}
	put("one", "empty.txt", "")
	// Reopening flob needs no cxz metadata and never stores a filename label.
	digest := flob.Digest(fmt.Sprintf("sha256:%x", sha256.Sum256([]byte("same"))))
	info, err := NewStores(root).Use("one").Stat(ctx, digest)
	if err != nil {
		t.Fatal(err)
	}
	labels, err := info.Labels(ctx)
	if err != nil || len(labels) != 0 {
		t.Fatal("unexpected filename metadata", labels, err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name() != "cas" && entry.Name() != "exports" {
			t.Fatal("unexpected sidecar state", entry.Name())
		}
	}
}

// Purging a session must reclaim the bytes only it held, and must not reclaim
// bytes a surviving session still points at -- content-addressed storage means
// two sessions that uploaded the same file share one blob.
func TestForgettingOneSessionKeepsWhatAnotherStillHolds(t *testing.T) {
	root := t.TempDir()
	ctx := t.Context()
	put := func(session, body string) string {
		t.Helper()
		path, err := Add(ctx, root, "project", session, "note.txt", int64(len(body)), strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		return path
	}
	shared := put("one", "shared")
	sharedAgain := put("two", "shared")
	onlyMine := put("one", "mine alone")
	// Read the store the way an operator would: is this content still on disk
	// anywhere under cas, whatever namespace or shard holds it?
	stored := func(body string) bool {
		t.Helper()
		found := false
		err := filepath.WalkDir(filepath.Join(root, "cas"), func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if string(data) == body {
				found = true
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		return found
	}
	if err := Forget(ctx, root, "project", "one"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(shared); !os.IsNotExist(err) {
		t.Fatal("purged session kept its export", err)
	}
	if _, err := os.Stat(onlyMine); !os.IsNotExist(err) {
		t.Fatal("purged session kept its export", err)
	}
	if _, err := os.Stat(sharedAgain); err != nil {
		t.Fatal("purging one session broke another's export", err)
	}
	if !stored("shared") {
		t.Fatal("reclaimed a blob another session still references")
	}
	if stored("mine alone") {
		t.Fatal("left the purged session's only blob behind")
	}
	// Purge is re-runnable, and a session that never uploaded has nothing to lose.
	for _, session := range []string{"one", "three"} {
		if err := Forget(ctx, root, "project", session); err != nil {
			t.Fatal(session, err)
		}
	}
	if _, err := os.Stat(sharedAgain); err != nil {
		t.Fatal("a re-run reached into a surviving session", err)
	}
}

type failedReader struct{}

func (failedReader) Read([]byte) (int, error) { return 0, fmt.Errorf("connection lost") }

func TestInvalidOrInterruptedStreamsAreNotPublished(t *testing.T) {
	cases := []struct {
		name string
		size int64
		src  io.Reader
	}{
		{"short", 4, strings.NewReader("abc")},
		{"long", 2, strings.NewReader("abc")},
		{"interrupted", 6, io.MultiReader(strings.NewReader("abc"), failedReader{})},
		{"oversized", MaxSize + 1, strings.NewReader("")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			if _, err := Add(t.Context(), root, "p", "s", "report.txt", tc.size, tc.src); err == nil {
				t.Fatal("invalid upload accepted")
			}
			if _, err := os.Stat(ExportRoot(root, "p")); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("failed upload published files", err)
			}
			for _, body := range []string{"abc", "ab"} {
				digest := flob.Digest(fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(body))))
				if _, err := NewStores(root).Use("s").Stat(t.Context(), digest); !errors.Is(err, flob.ErrNotExist) {
					t.Fatal("partial blob committed", err)
				}
			}
		})
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := Add(ctx, t.TempDir(), "p", "s", "empty.txt", 0, strings.NewReader("")); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled upload accepted", err)
	}
}

func TestInvalidAttachmentPaths(t *testing.T) {
	for _, name := range []string{"", "../x", "/x", "..", `a\b`, "a\x00b", "a\nb"} {
		if _, err := Add(t.Context(), t.TempDir(), "p", "s", name, 0, strings.NewReader("")); err == nil {
			t.Fatal("accepted filename", name)
		}
	}
	if _, err := Add(t.Context(), t.TempDir(), "..", "s", "x", 0, strings.NewReader("")); err == nil {
		t.Fatal("accepted scope traversal")
	}
}

func TestRemoveSessionPreservesOtherAttachmentLinks(t *testing.T) {
	root := t.TempDir()
	ctx := t.Context()
	for _, id := range []string{"one", "two"} {
		if _, err := Add(ctx, root, "project", id, "shared.txt", 6, strings.NewReader("shared")); err != nil {
			t.Fatal(err)
		}
	}
	if err := RemoveSession(ctx, root, "project", "one"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(ExportRoot(root, "project"), "one")); !os.IsNotExist(err) {
		t.Fatal(err)
	}
	var count int
	for _, err := range NewStores(root).Use("one").(flob.OsStore).Walk(ctx) {
		if err != nil {
			t.Fatal(err)
		}
		count++
	}
	if count != 0 {
		t.Fatal("removed attachment retained in CAS")
	}
	files, err := filepath.Glob(filepath.Join(ExportRoot(root, "project"), "two", "*", "shared.txt"))
	if err != nil || len(files) != 1 {
		t.Fatal(files, err)
	}
	if body, err := os.ReadFile(files[0]); err != nil || string(body) != "shared" {
		t.Fatal(string(body), err)
	}
	if err := RemoveSession(ctx, root, "project", "one"); err != nil {
		t.Fatal(err)
	}
}
