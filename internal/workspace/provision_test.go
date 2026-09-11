package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTrustBoundary(t *testing.T) {
	for _, cfg := range []map[string]any{
		{"privileged": true}, {"initializeCommand": "echo host"},
		{"runArgs": []any{"--network", "host"}}, {"runArgs": []any{"--pid=host"}},
		{"mounts": []any{"source=/var/run/docker.sock,target=/var/run/docker.sock,type=bind"}},
		{"services": map[string]any{"dev": map[string]any{"cap_add": []any{"SYS_ADMIN"}}}},
	} {
		if CheckTrust(cfg, false) == nil {
			t.Fatalf("accepted privileged config: %v", cfg)
		}
		if e := CheckTrust(cfg, true); e != nil {
			t.Fatal(e)
		}
	}
	if e := CheckTrust(map[string]any{"postCreateCommand": "go test ./...", "remoteUser": "vscode"}, false); e != nil {
		t.Fatal(e)
	}
}

func TestPreflightBeforeRecreate(t *testing.T) {
	root := t.TempDir()
	put := func(name, text string) {
		t.Helper()
		path := filepath.Join(root, name)
		if e := os.MkdirAll(filepath.Dir(path), 0755); e != nil {
			t.Fatal(e)
		}
		if e := os.WriteFile(path, []byte(text), 0600); e != nil {
			t.Fatal(e)
		}
	}
	put(".devcontainer/devcontainer.json", `{"dockerComposeFile":"compose.yaml","service":"dev", /* jsonc */}`)
	put(".devcontainer/compose.yaml", "services:\n  dev:\n    image: alpine\n    privileged: true\n")
	p := &Project{Workspace: root}
	if preflight(p) == nil {
		t.Fatal("compose privilege accepted before destructive recreation")
	}
	p.Trusted = true
	if e := preflight(p); e != nil {
		t.Fatal(e)
	}
	put(".devcontainer/other/devcontainer.json", `{"image":"alpine"}`)
	if preflight(p) == nil {
		t.Fatal("ambiguous config accepted")
	}
	p.Config = filepath.Join(root, ".devcontainer/devcontainer.json")
	if e := preflight(p); e != nil {
		t.Fatal(e)
	}
}
