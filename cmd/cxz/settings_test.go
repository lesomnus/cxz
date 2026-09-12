package main

import (
	"errors"
	"github.com/lesomnus/xli"
	"github.com/lesomnus/xli/xlitest"
	"strings"
	"testing"
)

func TestConfigOffline(t *testing.T) {
	root := t.TempDir()
	t.Setenv("DOCKER_HOST", "tcp://invalid.invalid:1")
	for _, args := range [][]string{{"config", "set", "claude-model", "claude-test"}, {"config", "set", "codex-model", "test-model"}, {"config"}, {"config", "unset", "agent"}} {
		result := xlitest.Run(t, newRoot(root), args...)
		if result.Err != nil {
			t.Fatalf("%v: %v", args, result.Err)
		}
	}
	got := xlitest.Run(t, newRoot(root), "config")
	if !strings.Contains(got.Stdout, "test-model") || strings.Contains(got.Stdout, `"agent"`) {
		t.Fatal(got)
	}
	for _, args := range [][]string{{"config", "set", "token", "secret"}, {"config", "set", "agent", "other"}, {"config", "set", "claude-model", "bad model"}} {
		if xlitest.Run(t, newRoot(root), args...).Err == nil {
			t.Fatal("accepted", args)
		}
	}
}

func TestReleaseCommandsOffline(t *testing.T) {
	root := t.TempDir()
	got := xlitest.Run(t, newRoot(root), "version")
	if got.Err != nil || !strings.Contains(got.Stdout, `"version"`) {
		t.Fatal(got)
	}
	for _, image := range []string{"", "ghcr.io/lesomnus/cxz", "ghcr.io/lesomnus/cxz:latest"} {
		args := []string{"update"}
		if image != "" {
			args = append(args, image)
		}
		got = xlitest.Run(t, newRoot(root), args...)
		if got.Err == nil || (image == "" && !errors.Is(got.Err, xli.ErrNeedArgs)) || (image != "" && !strings.Contains(got.Err.Error(), "explicit version")) {
			t.Fatal(got)
		}
	}
}
