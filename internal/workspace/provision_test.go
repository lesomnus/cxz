package workspace

import (
	"os"
	"path/filepath"
	"strings"
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
	if err := preflight(p); err == nil || !strings.Contains(err.Error(), "compose.yaml") || !strings.Contains(err.Error(), "/services/dev/privileged") {
		t.Fatal("missing Compose trust diagnostic before destructive recreation", err)
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

func TestTrustReasons(t *testing.T) {
	cases := []struct {
		cfg          map[string]any
		path, reason string
	}{
		{map[string]any{"privileged": true}, "/privileged", "privileged container"},
		{map[string]any{"initializeCommand": "echo PRIVATE_VALUE"}, "/initializeCommand", "host-side initialization"},
		{map[string]any{"network_mode": "host"}, "/network_mode", "host network"},
		{map[string]any{"pid": "host"}, "/pid", "host PID"},
		{map[string]any{"runArgs": []any{"--net", "host"}}, "/runArgs/0", "host network"},
		{map[string]any{"runArgs": []any{"--network", "host"}}, "/runArgs/0", "host network"},
		{map[string]any{"runArgs": []any{"--pid", "host"}}, "/runArgs/0", "host PID"},
		{map[string]any{"runArgs": []any{"--privileged"}}, "/runArgs/0", "privileged container"},
		{map[string]any{"runArgs": []any{"--network=host"}}, "/runArgs/0", "host network"},
		{map[string]any{"runArgs": []any{"--pid=host"}}, "/runArgs/0", "host PID"},
		{map[string]any{"mounts": []any{"source=/PRIVATE_VALUE/docker.sock,target=/var/run/docker.sock"}}, "/mounts/0", "Docker socket"},
		{map[string]any{"services": map[string]any{"dev": map[string]any{"cap_add": []any{"SYS_ADMIN"}}}}, "/services/dev/cap_add/0", "SYS_ADMIN"},
	}
	for _, c := range cases {
		t.Run(c.path+c.reason, func(t *testing.T) {
			err := CheckTrust(c.cfg, false)
			if err == nil {
				t.Fatal("trust check bypassed")
			}
			for _, want := range []string{c.path, c.reason, "--trust-config"} {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("missing %q in %s", want, err)
				}
			}
			if strings.Contains(err.Error(), "PRIVATE_VALUE") {
				t.Fatal("configuration value leaked")
			}
			if err := CheckTrust(c.cfg, true); err != nil {
				t.Fatal("explicit trust rejected", err)
			}
		})
	}
}

func TestTrustReportsAllDeterministically(t *testing.T) {
	cfg := map[string]any{"privileged": true, "initializeCommand": "echo SECRET", "runArgs": []any{"--pid=host", "--privileged"}, "mounts": []any{"/SECRET/docker.sock"}}
	first := CheckTrust(cfg, false).Error()
	if strings.Count(first, "  - ") != 5 || strings.Contains(first, "SECRET") {
		t.Fatal("incomplete or unsafe diagnostics", first)
	}
	for range 30 {
		if CheckTrust(cfg, false).Error() != first {
			t.Fatal("unstable order")
		}
	}
	err := CheckTrust(map[string]any{"a/b~c\n": map[string]any{"privileged": true}}, false)
	if !strings.Contains(err.Error(), `/a~1b~0c\n/privileged`) {
		t.Fatal("path not escaped", err)
	}
}

func TestPreflightDevcontainerTrustLocation(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "devcontainer.json")
	if err := os.WriteFile(file, []byte(`{"initializeCommand":"echo PRIVATE_VALUE"}`), 0600); err != nil {
		t.Fatal(err)
	}
	err := preflight(&Project{Workspace: root, Config: file})
	if err == nil || !strings.Contains(err.Error(), file) || !strings.Contains(err.Error(), "/initializeCommand") || strings.Contains(err.Error(), "PRIVATE_VALUE") {
		t.Fatal("missing or unsafe source diagnostic", err)
	}
}
