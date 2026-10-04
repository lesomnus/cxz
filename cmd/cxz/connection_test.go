package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lesomnus/cxz/internal/settings"
	"github.com/lesomnus/xli/xlitest"
)

func connectionTestState(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	cfg := `{"connections":{"default":"work","work":{"target":"ssh://alice@work:2222"},"local":{"target":"local://"},"tcp":{"target":"tcp://127.0.0.1:7349","token_file":"missing-secret-file"}}}`
	if err := os.WriteFile(settings.Path(root), []byte(cfg), 0600); err != nil {
		t.Fatal(err)
	}
	return root
}
func TestConnectionList(t *testing.T) {
	root := connectionTestState(t)
	got := xlitest.Run(t, newRoot(root), "connection", "ls", "--format", "json")
	if got.Err != nil {
		t.Fatal(got.Err)
	}
	var rows []struct {
		Name, Target    string
		Default, Tunnel bool
	}
	if err := json.Unmarshal([]byte(got.Stdout), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 || rows[0].Name != "local" || rows[2].Name != "work" || !rows[2].Default || !rows[2].Tunnel || rows[0].Tunnel {
		t.Fatalf("%+v", rows)
	}
	if strings.Contains(got.Stdout, "secret") {
		t.Fatal("list included credential details")
	}
	got = xlitest.Run(t, newRoot(root), "connection", "ls")
	if got.Err != nil || !strings.Contains(got.Stdout, "NAME") || !strings.Contains(got.Stdout, "work") {
		t.Fatal(got)
	}
	for _, args := range [][]string{{"connection", "--help"}, {"connection", "tunnel", "--help"}} {
		if got := xlitest.Run(t, newRoot(root), args...); got.Err != nil {
			t.Fatal(got)
		}
	}
}
func TestConnectionEmptyAndValidation(t *testing.T) {
	absent := filepath.Join(t.TempDir(), "absent")
	got := xlitest.Run(t, newRoot(absent), "connection", "ls", "--format", "json")
	if got.Err != nil || strings.TrimSpace(got.Stdout) != "[]" {
		t.Fatal(got)
	}
	if _, err := os.Stat(absent); !os.IsNotExist(err) {
		t.Fatal("listing created state")
	}
	root := connectionTestState(t)
	for _, args := range [][]string{
		{"connection", "ls", "--format", "bad"},
		{"connection", "tunnel"},
		{"connection", "tunnel", "missing"},
		{"connection", "tunnel", "local"},
		{"connection", "tunnel", "tcp"},
		{"connection", "tunnel", "--local-port", "0", "work"},
		{"connection", "tunnel", "--remote-port", "65536", "work"},
		{"connection", "tunnel", "--local-port", "abc", "work"},
	} {
		got := xlitest.Run(t, newRoot(root), args...)
		if got.Err == nil {
			t.Fatalf("accepted %v", args)
		}
		if strings.Contains(got.Stderr, "Opening SSH") {
			t.Fatalf("attempted connection before validation: %v", args)
		}
	}
}
