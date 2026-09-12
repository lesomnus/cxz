package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lesomnus/xli/xlitest"
)

func TestInvalidInputsHaveNoSideEffects(t *testing.T) {
	t.Setenv("DOCKER_HOST", "tcp://invalid.invalid:1")
	cases := [][]string{
		{"account", "add"}, {"account", "add", "codex"},
		{"account", "add", "other", "work"}, {"account", "add", "", "work"},
		{"account", "add", "codex", "bad/account"},
		{"account", "add", "--agent", "codex", "work"},
		{"account", "add", "--auth-backend", "brokered-access-token", "claude", "work"},
		{"account", "add", "--auth-backend", "", "codex", "work"},
		{"account", "get", ""}, {"account", "login", "../work"},
		{"update"}, {"update", "--image", "cxz:edge"}, {"update", "cxz:latest"},
		{"update", "cxz:"}, {"update", "cxz@"}, {"update", "cxz"}, {"update", ""},
		{"_new-local", "."}, {"_new-local", "--account", "work", "."},
		{"project", "set", "project"}, {"project", "set", "--name", "", "project"},
		{"project", "set", "--alias", "bad/alias", "project"},
		{"project", "add", "--name", "bad\nname", "."},
		{"config", "set", "unknown", "value"}, {"config", "unset", "unknown"},
		{"config", "set", "agent", "codex"}, {"config", "set", "codex-model", "bad model"},
		{"new", "."}, {"new", "--account", "", "."},
		{"up", "--model", "bad model"}, {"up", "--agent", ""},
		{"send", "session", ""}, {"stop", " "},
		{"reply", "session", "request", "allow", "null"},
		{"reply", "session", "request", "allow", "[]"},
		{"reply", "session", "request", "allow", `{"q":1}`},
		{"reply", "session", "request", "allow", "invalid"},
		{"--state", "", "version"}, {"serve", "--agent", ""},
		{"logs", "--tail", "0"}, {"logs", "--tail", "10001"},
	}
	for _, args := range cases {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			state := filepath.Join(t.TempDir(), "untouched")
			got := xlitest.Run(t, newRoot(state), args...)
			if got.Err == nil {
				t.Fatal("accepted invalid invocation")
			}
			if strings.Contains(got.Err.Error(), "not installed") || strings.Contains(got.Err.Error(), "dial") || strings.Contains(got.Stderr, "preparing workspace") {
				t.Fatalf("validation occurred too late: %+v", got)
			}
			if _, err := os.Stat(state); !os.IsNotExist(err) {
				t.Fatalf("touched state: %v", err)
			}
		})
	}
}
