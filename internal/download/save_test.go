package download

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestSaveBinaryCollisionAndFailure(t *testing.T) {
	dir := t.TempDir()
	data := bytes.Repeat([]byte{0, 255, 13, 10, 42}, 20000)
	for i := 0; i < 2; i++ {
		file, n, err := Save(context.Background(), dir, "/container/한글 report.bin", func(w io.Writer) error { _, err := w.Write(data); return err })
		if err != nil || n != int64(len(data)) {
			t.Fatal(n, err)
		}
		want := "한글 report.bin"
		if i == 1 {
			want = "한글 report (1).bin"
		}
		got, _ := os.ReadFile(file)
		if filepath.Base(file) != want || !bytes.Equal(got, data) {
			t.Fatal(file, "corrupted download")
		}
	}
	if _, _, err := Save(context.Background(), dir, "broken", func(w io.Writer) error { w.Write([]byte("partial")); return errors.New("connection lost") }); err == nil {
		t.Fatal("failed transfer accepted")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 2 {
		t.Fatal("partial files remain", entries)
	}
}
func TestSaveEmptyAndCancelled(t *testing.T) {
	dir := t.TempDir()
	if _, n, err := Save(context.Background(), dir, "empty", func(io.Writer) error { return nil }); err != nil || n != 0 {
		t.Fatal(n, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	if _, _, err := Save(ctx, dir, "cancelled", func(w io.Writer) error { cancel(); _, err := w.Write([]byte("x")); return err }); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatal(entries)
	}
}
func TestSafeFilename(t *testing.T) {
	for source, want := range map[string]string{"../../etc/passwd": "passwd", "/a/CON": "_CON", "/a/a:b?.txt": "a_b_.txt", "/a/..": "download", "/a/report.txt": "report.txt"} {
		if got := filename(source); got != want {
			t.Fatal(source, got, want)
		}
	}
}
