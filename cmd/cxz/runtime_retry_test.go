package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/lesomnus/xli/xlitest"
)

func TestRuntimeRetryHelpAndRequiredTargetStayOffline(t *testing.T) {
	root := filepath.Join(t.TempDir(), "unused")
	got := xlitest.Run(t, newRoot(root), "self-update", "retry", "--help")
	if got.Err != nil || !strings.Contains(got.Stdout, "--project") || !strings.Contains(got.Stdout, "--server") {
		t.Fatal(got)
	}
	for _, args := range [][]string{{"self-update", "retry"}, {"self-update", "retry", "--server"}, {"self-update", "retry", "--project", "demo"}} {
		got := xlitest.Run(t, newRoot(root), args...)
		if got.Err == nil || !strings.Contains(got.Err.Error(), "requires --server --project") {
			t.Fatalf("invalid target reached the installer: %+v", got)
		}
	}
}
