package distribution

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

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

func TestRuntimeResolvesAgentsWithoutWritingTheSharedCache(t *testing.T) {
	root := t.TempDir()
	if err := core.WriteJSON(filepath.Join(root, "releases.json"), map[string]string{"claude": "3.0.0"}); err != nil {
		t.Fatal(err)
	}
	selected, _ := AgentPath(root, "claude", "arm64", true, "3.0.0")
	if err := os.MkdirAll(filepath.Dir(selected), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(selected, []byte("cached"), 0555); err != nil {
		t.Fatal(err)
	}
	registered := filepath.Join(t.TempDir(), "custom-claude")
	if err := os.WriteFile(registered, []byte("custom"), 0755); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"", "/removed/claude", registered} {
		got, err := ResolveAgent(root, "claude", "arm64", true, path)
		want := selected
		if path == registered {
			want = registered
		}
		if err != nil || got != want {
			t.Fatal("registered choice or cached fallback lost", got, err)
		}
	}
	wrong, _ := AgentPath(root, "claude", "amd64", false, "3.0.0")
	if err := os.MkdirAll(filepath.Dir(wrong), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(wrong, []byte("wrong platform"), 0755); err != nil {
		t.Fatal(err)
	}
	if got, err := ResolveAgent(root, "claude", "arm64", true, wrong); err != nil || got != selected {
		t.Fatal("a registration from the previous container platform was reused", got, err)
	}
	if _, err := ResolveAgent(root, "claude", "amd64", true, ""); err == nil {
		t.Fatal("used a cached binary for the wrong architecture")
	}
	if _, err := ResolveAgent(root, "claude", "arm64", false, ""); err == nil {
		t.Fatal("used a cached binary for the wrong libc")
	}
	if _, err := ResolveAgent(root, "codex", "amd64", false, ""); err == nil {
		t.Fatal("missing cache did not report that the Manager must prepare it")
	}
	if _, err := os.Stat(filepath.Join(root, "codex")); !os.IsNotExist(err) {
		t.Fatal("the read-only resolver attempted an installation", err)
	}
}

func TestConcurrentAgentInstallWaitsForThePublishedRelease(t *testing.T) {
	root := t.TempDir()
	binary, _ := AgentPath(root, "codex", "amd64", true, "1.2.3")
	dir := filepath.Dir(filepath.Dir(binary))
	if err := os.MkdirAll(filepath.Dir(dir), 0755); err != nil {
		t.Fatal(err)
	}
	lock, err := core.Lock(dir + ".lock")
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		got, err := EnsureVersion(ctx, root, "codex", "amd64", true, "1.2.3")
		if err == nil && got != binary {
			err = errors.New("the published cache entry was not reused")
		}
		done <- err
	}()
	select {
	case err := <-done:
		t.Fatal("a competing installer did not wait", err)
	case <-time.After(30 * time.Millisecond):
	}
	// The lock holder completes a verified, immutable publication.
	if err = os.MkdirAll(filepath.Dir(binary), 0755); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(binary, []byte("published release"), 0755); err != nil {
		t.Fatal(err)
	}
	if err = lock.Close(); err != nil {
		t.Fatal(err)
	}
	if err = <-done; err != nil {
		t.Fatal("a completed installation was not reused", err)
	}

	missing, _ := AgentPath(root, "codex", "amd64", true, "2.0.0")
	if err = os.MkdirAll(filepath.Dir(filepath.Dir(filepath.Dir(missing))), 0755); err != nil {
		t.Fatal(err)
	}
	lock, err = core.Lock(filepath.Dir(filepath.Dir(missing)) + ".lock")
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	ctx, cancel = context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	if _, err = EnsureVersion(ctx, root, "codex", "amd64", true, "2.0.0"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("waiting for another installer did not honor cancellation", err)
	}
}
