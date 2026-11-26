package main

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/lesomnus/xli/xlitest"
)

func TestInstallationCommandsRefuseOnKubernetes(t *testing.T) {
	if runtime.GOOS == "windows" {
		// A Kubernetes manager is a Linux pod; the Windows frontend has its
		// own command tree and never runs as one.
		t.Skip("Kubernetes managers run on Linux")
	}
	t.Setenv("CXZ_BACKEND", "kubernetes")
	root := filepath.Join(t.TempDir(), "state")
	for _, args := range [][]string{{"install"}, {"uninstall"}, {"use", "edge"}, {"self-update"}, {"self-update", "status", "--server"}, {"purge", "--dry-run"}} {
		got := xlitest.Run(t, newRoot(root), args...)
		if got.Err == nil || !strings.Contains(got.Err.Error(), "image tag of its Deployment") {
			t.Errorf("%v: %v", args, got.Err)
		}
	}
	// Help still answers.
	if got := xlitest.Run(t, newRoot(root), "self-update", "--help"); got.Err != nil {
		t.Fatal(got)
	}
}
