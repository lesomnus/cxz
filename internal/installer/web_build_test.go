//go:build !windows

package installer

import (
	"archive/tar"
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestLocalImageContextIncludesUIAndChangesWithUI(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("local manager builds require Linux")
	}
	dir := t.TempDir()
	ui := filepath.Join(dir, "webui")
	if err := os.Mkdir(ui, 0755); err != nil {
		t.Fatal(err)
	}
	index := filepath.Join(ui, "index.html")
	if err := os.WriteFile(index, []byte("first UI"), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CXZ_WEB_ASSETS_DIR", ui)
	t.Setenv("CXZ_TEST_CONTEXT", filepath.Join(dir, "context.tar"))
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	script := "#!/bin/sh\ncase \"$1\" in\ninfo) printf 'linux/" + runtime.GOARCH + "\n';;\nimage) exit 1;;\nbuild) cat > \"$CXZ_TEST_CONTEXT\";;\n*) exit 1;;\nesac\n"
	if err := os.WriteFile(filepath.Join(dir, "docker"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	first, err := Build(context.Background(), io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(os.Getenv("CXZ_TEST_CONTEXT"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	tr := tar.NewReader(f)
	foundUI, foundBinary := false, false
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if h.Name == "webui/index.html" {
			data, err := io.ReadAll(tr)
			if err != nil || string(data) != "first UI" {
				t.Fatalf("%q %v", data, err)
			}
			foundUI = true
		}
		if h.Name == "linux-"+runtime.GOARCH+"/cxz" {
			foundBinary = true
		}
	}
	if !foundUI || !foundBinary {
		t.Fatalf("UI %v, executable %v", foundUI, foundBinary)
	}
	if err := os.WriteFile(index, []byte("second UI"), 0644); err != nil {
		t.Fatal(err)
	}
	second, err := Build(context.Background(), io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("image cache reused after a UI-only change")
	}
}
