package installer

import (
	"archive/tar"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"testing"
	"time"
)

func TestUISourceRevisionRequiresExactRevision(t *testing.T) {
	revision := strings.Repeat("a", 40)
	for _, tc := range []struct {
		name     string
		settings []debug.BuildSetting
		want     string
	}{
		{"release", []debug.BuildSetting{{Key: "-ldflags", Value: "-s -w -X main.buildRevision=" + revision}}, revision},
		{"clean checkout", []debug.BuildSetting{{Key: "vcs.revision", Value: revision}, {Key: "vcs.modified", Value: "false"}}, revision},
		{"dirty checkout", []debug.BuildSetting{{Key: "vcs.revision", Value: revision}, {Key: "vcs.modified", Value: "true"}}, ""},
		{"missing metadata", nil, ""},
		{"moving branch", []debug.BuildSetting{{Key: "-ldflags", Value: "-X main.buildRevision=main"}}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := uiSourceRevision(tc.settings); got != tc.want {
				t.Fatalf("%q", got)
			}
		})
	}
}

func TestUIArchiveIncludesFilesAndIgnoresTimestamps(t *testing.T) {
	dir := t.TempDir()
	index := filepath.Join(dir, "index.html")
	if err := os.WriteFile(index, []byte("app"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "assets"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "assets", "app.js"), []byte("script"), 0644); err != nil {
		t.Fatal(err)
	}
	archive := func() []byte {
		t.Helper()
		var buf bytes.Buffer
		tw := tar.NewWriter(&buf)
		if err := writeUIArchive(tw, dir); err != nil {
			t.Fatal(err)
		}
		if err := tw.Close(); err != nil {
			t.Fatal(err)
		}
		return buf.Bytes()
	}
	first := archive()
	if err := os.Chtimes(index, time.Unix(123, 0), time.Unix(456, 0)); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, archive()) {
		t.Fatal("timestamps change image identity")
	}
	found := map[string]string{}
	tr := tar.NewReader(bytes.NewReader(first))
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			t.Fatal(err)
		}
		found[hdr.Name] = string(data)
	}
	if found["webui/index.html"] != "app" || found["webui/assets/app.js"] != "script" {
		t.Fatal(found)
	}
	if err := os.WriteFile(index, []byte("changed"), 0644); err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(first, archive()) {
		t.Fatal("UI content does not change image identity")
	}
}

func TestUIArchiveRejectsSymlinks(t *testing.T) {
	dir := t.TempDir()
	if err := os.Symlink("/outside", filepath.Join(dir, "asset")); err != nil {
		t.Skip(err)
	}
	if err := writeUIArchive(tar.NewWriter(io.Discard), dir); err == nil {
		t.Fatal("UI archive followed an external symlink")
	}
}

func TestManagerUIBuildUsesExplicitBundle(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CXZ_WEB_ASSETS_DIR", dir)
	if _, _, _, err := managerUIBuild(); err == nil {
		t.Fatal("missing explicit UI build accepted")
	}
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("app"), 0644); err != nil {
		t.Fatal(err)
	}
	recipe, got, revision, err := managerUIBuild()
	if err != nil || got != dir || revision != "" || !bytes.Equal(recipe, dockerfile) {
		t.Fatalf("%q %q %v", got, revision, err)
	}
}
