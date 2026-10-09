package webui

import (
	"errors"
	"io/fs"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiskAssetsReadFilesWithoutRebuildingGo(t *testing.T) {
	dir := t.TempDir()
	index := filepath.Join(dir, "index.html")
	if err := os.WriteFile(index, []byte("first build"), 0644); err != nil {
		t.Fatal(err)
	}
	assets, err := Assets(dir)
	if err != nil {
		t.Fatal(err)
	}
	handler := spaFiles(assets)
	read := func(want string) {
		t.Helper()
		r := httptest.NewRequest("GET", "/sessions/demo", nil)
		r.Header.Set("Accept", "text/html")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != 200 || w.Body.String() != want {
			t.Fatalf("%d %q", w.Code, w.Body.String())
		}
	}
	read("first build")
	if err := os.WriteFile(index, []byte("second build"), 0644); err != nil {
		t.Fatal(err)
	}
	read("second build")
}

func TestDiskAssetsRequireReadableIndex(t *testing.T) {
	dir := t.TempDir()
	if _, err := Assets(dir); !errors.Is(err, fs.ErrNotExist) || !strings.Contains(err.Error(), "assets-dir") {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "index.html"), 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := Assets(dir); err == nil {
		t.Fatal("directory accepted as app shell")
	}
}

func TestExplicitAssetsDirectoryNeverFallsBack(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "missing")
	t.Setenv("CXZ_WEB_ASSETS_DIR", dir)
	if got := AssetsDirectory(); got != dir {
		t.Fatalf("%q", got)
	}
	if _, err := Assets(AssetsDirectory()); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
}
