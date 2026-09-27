package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/lesomnus/xli/xlitest"
)

func TestRemoteCommandHelpAndValidation(t *testing.T) {
	t.Setenv("CXZ_ENDPOINT", "")
	root := func() string { return filepath.Join(t.TempDir(), "client") }
	help := xlitest.Run(t, newRoot(root()), "--help")
	if help.Err != nil || !strings.Contains(help.Stdout, "endpoint") {
		t.Fatal(help)
	}
	for _, args := range [][]string{{"--endpoint", "https://host"}, {"--endpoint", "tcp://host:7349"}, {"--endpoint", "ssh://user:secret@host"}} {
		if got := xlitest.Run(t, newRoot(root()), args...); got.Err == nil {
			t.Fatal("invalid connection accepted", args)
		}
	}
}
