package main

import (
	"github.com/lesomnus/cxz/internal/settings"
	"github.com/lesomnus/xli/xlitest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileMappingsOfflineValidation(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(t.TempDir(), "CLAUDE.md")
	if err := os.WriteFile(src, []byte("instructions"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", src, "relative"}, {"add", "--agent", "unknown", src, "${AGENT_CONFIG_DIR}/CLAUDE.md"}, {"add", src + "-missing", "${AGENT_CONFIG_DIR}/CLAUDE.md"}} {
		got := xlitest.Run(t, newRoot(root), append([]string{"config", "files"}, args...)...)
		if got.Err == nil {
			t.Fatal("accepted", args)
		}
	}
	cfg, err := settings.Load(root)
	if err != nil || len(cfg.Files) != 0 {
		t.Fatal("invalid mapping changed settings", err)
	}
	got := xlitest.Run(t, newRoot(root), "config", "files", "ls")
	if got.Err != nil {
		t.Fatal(got.Err)
	}
	// Help documents the variables without opening a connection.
	got = xlitest.Run(t, newRoot(root), "config", "files", "add", "--help")
	if got.Err != nil || !strings.Contains(got.Stdout, "DST") {
		t.Fatal(got)
	}
}
