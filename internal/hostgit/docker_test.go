package hostgit

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/dockerx"
)

func TestLiveGitConfigRefresh(t *testing.T) {
	if os.Getenv("CXZ_TEST_HOSTGIT_DOCKER") != "1" {
		t.Skip("opt-in disposable Docker test")
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	image := os.Getenv("CXZ_TEST_HOSTGIT_IMAGE")
	if image == "" {
		image = "golang:1.26"
	}
	name := "cxz-hostgit-test-" + core.ID()
	_, err := dockerx.Run(ctx, "run", "-d", "--rm", "--network", "none", "--name", name, "--label", "cxz.owner=hostgit-test", "--entrypoint", "sleep", image, "180")
	if err != nil {
		t.Fatal(err)
	}
	defer dockerx.Run(context.Background(), "rm", "-f", name)
	if _, err = dockerx.Run(ctx, "exec", name, "mkdir", "-p", "/cxz/state/data"); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", home)
	t.Setenv("GIT_CONFIG_GLOBAL", "")
	if _, err = dockerx.Run(ctx, "exec", name, "git", "config", "--system", "test.preserved", "yes"); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"First", "Changed", ""} {
		if err = os.WriteFile(filepath.Join(home, ".gitconfig"), []byte("[user]\nname = "+quote(value)+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
		b, err := Snapshot(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if err = Inject(ctx, name, "nobody", b); err != nil {
			t.Fatal(err)
		}
		if err = Inject(ctx, name, "nobody", b); err != nil {
			t.Fatal("idempotent injection", err)
		}
		for _, sessionHome := range []string{"/tmp/session-a", "/tmp/session-b"} {
			out, err := dockerx.Run(ctx, "exec", "--user", "nobody", "-e", "HOME="+sessionHome, name, "git", "config", "--get", "user.name")
			if err != nil || strings.TrimSpace(string(out)) != value {
				t.Fatalf("isolated HOME %s not refreshed: %v", sessionHome, err)
			}
		}
	}
	out, err := dockerx.Run(ctx, "exec", name, "git", "config", "--system", "--get-all", "include.path")
	if err != nil || strings.TrimSpace(string(out)) != Directory+"/active.config" {
		t.Fatal("duplicate system include", err)
	}
	out, err = dockerx.Run(ctx, "exec", name, "git", "config", "--get", "test.preserved")
	if err != nil || strings.TrimSpace(string(out)) != "yes" {
		t.Fatal("image configuration overwritten", err)
	}
}
