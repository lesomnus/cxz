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
