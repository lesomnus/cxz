package workspace

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lesomnus/cxz/internal/dockerx"
	"github.com/lesomnus/cxz/internal/projectconfig"
)

// Compose config performs no Docker engine operations. Exercise the actual merge
// rules, file ordering and relative-path base used by the devcontainer CLI.
func TestComposeOverridePreflightAndMerge(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("Docker Compose CLI required")
	}
	if _, err := dockerx.Run(context.Background(), "compose", "version"); err != nil {
		t.Skip("Docker Compose CLI required")
	}
	root := t.TempDir()
	configDir := filepath.Join(root, "workspace", ".devcontainer")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatal(err)
	}
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(configDir, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("devcontainer.json", `{"dockerComposeFile":"compose.yaml","service":"editor"}`)
	write("compose.yaml", "services:\n  editor:\n    image: alpine\n    volumes:\n      - ..:/workspace\n      - /original:/workspaces\n      - cache:/cache\nvolumes:\n  cache: {}\n")
	m := &Manager{Root: filepath.Join(root, "manager"), Owner: "123456789012345678901234"}
	p := &Project{ID: "project", Workspace: filepath.Dir(configDir)}
	set := func(raw string) {
		t.Helper()
		if err := projectconfig.Save(m.Root, projectconfig.Spec{Compose: json.RawMessage(raw)}); err != nil {
			t.Fatal(err)
		}
	}
	set(`{"services":{"${DEVCONTAINER_SERVICE}":{"volumes":["/host/workspaces:/workspaces"],"environment":{"USER_OPTION":"kept"}}}}`)
	path, digest, err := m.prepareComposeOverride(context.Background(), p)
	if err != nil || len(digest) != 64 {
		t.Fatal(path, digest, err)
	}
	data, err := dockerx.Run(context.Background(), "compose", "-f", filepath.Join(configDir, "compose.yaml"), "-f", path, "config", "--format", "json")
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Services map[string]struct {
			Volumes     []struct{ Source, Target string }
			Environment map[string]string
		}
	}
	if err = json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	dev := result.Services["editor"]
	mounts := map[string]string{}
	for _, mount := range dev.Volumes {
		mounts[mount.Target] = mount.Source
	}
	if len(mounts) != 3 || mounts["/workspace"] != p.Workspace || mounts["/workspaces"] != "/host/workspaces" || mounts["/cache"] != "cache" || dev.Environment["USER_OPTION"] != "kept" {
		t.Fatalf("wrong merge: %s", data)
	}
	set(`{"services":{"${DEVCONTAINER_SERVICE}":{"privileged":true}}}`)
	if _, _, err = m.prepareComposeOverride(context.Background(), p); err == nil || !strings.Contains(err.Error(), "requires explicit trust") {
		t.Fatal("override bypassed trust", err)
	}
	p.Trusted = true
	if _, _, err = m.prepareComposeOverride(context.Background(), p); err != nil {
		t.Fatal("trusted override rejected", err)
	}
	set(`{"services":{"${DEVCONTAINER_SERVICE}":{"not_a_compose_option":true}}}`)
	if _, _, err = m.prepareComposeOverride(context.Background(), p); err == nil {
		t.Fatal("invalid Compose accepted before recreate")
	}
	write("devcontainer.json", `{"image":"alpine"}`)
	if _, _, err = m.prepareComposeOverride(context.Background(), p); err == nil || !strings.Contains(err.Error(), "requires dockerComposeFile") {
		t.Fatal("override silently ignored", err)
	}
	if err := projectconfig.Save(m.Root, projectconfig.Spec{}); err != nil {
		t.Fatal(err)
	}
	path, digest, err = m.prepareComposeOverride(context.Background(), p)
	if err != nil || path != "" || digest != "" {
		t.Fatal("stale override still used", path, digest, err)
	}
}
