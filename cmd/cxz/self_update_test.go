package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/transport"
	"github.com/lesomnus/xli/xlitest"
)

func TestSelfUpdateHelpAndInvalidRefStayOffline(t *testing.T) {
	root := filepath.Join(t.TempDir(), "unused-state")
	got := xlitest.Run(t, newRoot(root), "self-update", "--help")
	if got.Err != nil || !strings.Contains(got.Stdout, "client-only") || !strings.Contains(got.Stdout, "--ref") {
		t.Fatal(got)
	}
	got = xlitest.Run(t, newRoot(root), "self-update", "--ref", "bad ref")
	if got.Err == nil || !strings.Contains(got.Err.Error(), "ref must") {
		t.Fatal(got)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatal("help/invalid flags created updater state", err)
	}
}

func TestSelfUpdateRefreshesOnlyInstalledLocalManager(t *testing.T) {
	root := t.TempDir()
	if refresh, err := shouldRefreshManager(root, false); err != nil || refresh {
		t.Fatal("missing installation triggers manager create", refresh, err)
	}
	if err := core.WriteJSON(filepath.Join(root, "installation.json"), transport.Installation{Owner: "owner", Container: "local-manager"}); err != nil {
		t.Fatal(err)
	}
	if refresh, err := shouldRefreshManager(root, false); err != nil || refresh != (runtime.GOOS == "linux") {
		t.Fatal("wrong platform manager update decision", refresh, err)
	}
	if refresh, err := shouldRefreshManager(root, true); err != nil || refresh {
		t.Fatal("client-only refreshed manager", refresh, err)
	}
}
