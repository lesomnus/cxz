package workspace

import (
	"context"
	"testing"
)

func TestAutomaticUpdatesOptOut(t *testing.T) {
	for _, value := range []string{"false", "0", "FALSE"} {
		t.Setenv("CXZ_AUTO_UPDATE", value)
		// A disabled worker must not even access the registry or network.
		var m *Manager
		m.RunUpdates(context.Background())
	}
}

func TestUpdateVersionOrdering(t *testing.T) {
	for _, v := range [][2]string{{"2.10.0", "2.9.9"}, {"1.0.0", "0.154.0"}, {"2.1.300", "2.1.267"}} {
		if !newerVersion(v[0], v[1]) {
			t.Fatal(v)
		}
	}
	for _, v := range [][2]string{{"2.9.9", "2.10.0"}, {"1.0.0", "1.0.0"}, {"1.0.0-beta", "0.0.0"}, {"1.0.0", ""}} {
		if newerVersion(v[0], v[1]) {
			t.Fatal(v)
		}
	}
	if binaryVersion("codex", "/cxz/tools/codex/0.154.0/x86_64-unknown-linux-musl/bin/codex") != "0.154.0" {
		t.Fatal("version missing")
	}
	if binaryVersion("codex", "/tmp/codex") != "" {
		t.Fatal("adopted unmanaged executable")
	}
}
