package distribution

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/lesomnus/cxz/internal/core"
)

func TestAgentPathsMatchContainerPlatforms(t *testing.T) {
	for _, tc := range []struct {
		kind, arch string
		musl       bool
		path       string
	}{
		{"claude", "amd64", false, "claude/1.2.3/linux-x64/claude"},
		{"claude", "x86_64", true, "claude/1.2.3/linux-x64-musl/claude"},
		{"claude", "aarch64", false, "claude/1.2.3/linux-arm64/claude"},
		{"claude", "arm64", true, "claude/1.2.3/linux-arm64-musl/claude"},
		{"codex", "amd64", false, "codex/1.2.3/x86_64-unknown-linux-musl/bin/codex"},
		{"codex", "aarch64", true, "codex/1.2.3/aarch64-unknown-linux-musl/bin/codex"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			got, err := AgentPath("/tools", tc.kind, tc.arch, tc.musl, "1.2.3")
			if err != nil || got != filepath.Join("/tools", tc.path) {
				t.Fatal(got, err)
			}
		})
	}
	for _, tc := range []struct{ kind, arch, version string }{
		{"unknown", "amd64", "1.2.3"}, {"codex", "riscv64", "1.2.3"}, {"claude", "amd64", "../bad"},
	} {
		if _, err := AgentPath("/tools", tc.kind, tc.arch, false, tc.version); err == nil {
			t.Fatal("accepted unsupported release", tc)
		}
	}
}

func TestEnsureAgentReusesTheSelectedCachedRelease(t *testing.T) {
	root := t.TempDir()
	if err := core.WriteJSON(filepath.Join(root, "releases.json"), map[string]string{"codex": "1.2.3"}); err != nil {
		t.Fatal(err)
	}
	for _, version := range []string{"1.2.3", "9.9.9"} {
		bin, err := AgentPath(root, "codex", "amd64", true, version)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.MkdirAll(filepath.Dir(bin), 0755); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(bin, []byte("cached verified release"), 0755); err != nil {
			t.Fatal(err)
		}
	}
	want, _ := AgentPath(root, "codex", "amd64", true, "1.2.3")
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			got, err := Ensure(t.Context(), root, "codex", "amd64", true)
			if err != nil || got != want {
				t.Errorf("selected cache not reused: %s, %v", got, err)
			}
		})
	}
	wg.Wait()
	if err := os.Chmod(want, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := Ensure(t.Context(), root, "codex", "amd64", true); err == nil {
		t.Fatal("accepted a non-executable cache entry")
	}
	for _, tc := range []struct{ kind, version string }{{"claude", ClaudeVersion}, {"codex", CodexVersion}} {
		got, err := SelectedAgentVersion(t.TempDir(), tc.kind)
		if err != nil || got != tc.version {
			t.Fatal("bundled fallback lost", got, err)
		}
	}
	if _, err := Ensure(t.Context(), root, "unknown", "amd64", false); err == nil {
		t.Fatal("accepted an unknown provider")
	}
}
