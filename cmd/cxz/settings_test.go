package main

import (
	"github.com/lesomnus/xli/xlitest"
	"strings"
	"testing"
)

func TestConfigOffline(t *testing.T) {
	root := t.TempDir()
	t.Setenv("DOCKER_HOST", "tcp://invalid.invalid:1")
	for _, args := range [][]string{{"config", "set", "agent", "codex"}, {"config", "set", "codex-model", "test-model"}, {"config"}, {"config", "unset", "agent"}} {
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
