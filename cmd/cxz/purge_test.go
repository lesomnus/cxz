package main

import (
	"github.com/lesomnus/xli/xlitest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPurgeDryRunAndTTYRequirement(t *testing.T) {
	t.Setenv("CXZ_OWNER", "")
	t.Setenv("CXZ_PROJECT_ID", "")
	t.Setenv("DOCKER_HOST", "tcp://invalid.invalid:1")
	root := t.TempDir()
	filename := filepath.Join(root, "settings.json")
	os.WriteFile(filename, []byte("{}"), 0600)
	got := xlitest.Run(t, newRoot(root), "purge", "--dry-run")
	if got.Err != nil || !strings.Contains(got.Stdout, filename) {
		t.Fatal(got)
	}
	if _, err := os.Stat(filename); err != nil {
		t.Fatal("dry-run deleted state")
	}
	got = xlitest.Run(t, newRoot(root), "purge")
	if got.Err == nil || !strings.Contains(got.Err.Error(), "interactive terminal") {
		t.Fatal("noninteractive purge allowed", got.Err)
	}
	if _, err := os.Stat(filename); err != nil {
		t.Fatal("noninteractive invocation deleted state")
	}
	entries, _ := os.ReadDir(root)
	if len(entries) != 1 {
		t.Fatal("preview wrote files")
	}
}
