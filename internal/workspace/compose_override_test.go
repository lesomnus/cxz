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
	write("compose.yaml", "services:\n  editor:\n    image: alpine\n    command: [sleep, infinity]\n    volumes:\n      - ..:/workspace\n      - /original:/workspaces\n      - cache:/cache\nvolumes:\n  cache: {}\n")
	m := &Manager{Root: filepath.Join(root, "manager"), Owner: "123456789012345678901234"}
	p := &Project{ID: "project", Workspace: filepath.Dir(configDir)}
	set := func(raw string) {
		t.Helper()
		if err := projectconfig.Save(m.Root, projectconfig.Spec{Compose: json.RawMessage(raw)}); err != nil {
			t.Fatal(err)
		}
	}
	set(`{"services":{"${DEVCONTAINER_SERVICE}":{"volumes":["/host/workspaces:/workspaces"],"environment":{"USER_OPTION":"kept"}}}}`)
	out, err := m.prepareComposeOverride(context.Background(), p)
	path := out.ComposeFile
	if err != nil || len(out.Digest) != 64 {
		t.Fatal(out, err)
	}
	data, err := dockerx.Run(context.Background(), "compose", "-f", filepath.Join(configDir, "compose.yaml"), "-f", path, "config", "--format", "json")
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Services map[string]struct {
			Volumes     []struct{ Source, Target string }
			Environment map[string]string
			Command     []string
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
	// Resolve local includes before transfer. The manager must be able to merge
	// the snapshot after those files are gone, without resetting base commands
	// or interpolating host values again in its own environment.
	host := t.TempDir()
	if err := os.WriteFile(filepath.Join(host, "mounts.yaml"), []byte("services:\n  ${DEVCONTAINER_SERVICE}:\n    volumes:\n      - ./src:/workspaces\n    environment:\n      VALUE: ${CXZ_TEST_COMPOSE_VALUE}\n      LITERAL: $${CXZ_TEST_COMPOSE_VALUE}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CXZ_TEST_COMPOSE_VALUE", "host-$value")
	snapshot, err := projectconfig.SnapshotFile(filepath.Join(host, projectconfig.Filename), []byte("include: [./mounts.yaml]\n"))
	if err != nil {
		t.Fatal(err)
	}
	if err = projectconfig.Save(m.Root, snapshot); err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(filepath.Join(host, "mounts.yaml")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CXZ_TEST_COMPOSE_VALUE", "manager-value")
	out, err = m.prepareComposeOverride(context.Background(), p)
	path = out.ComposeFile
	if err != nil {
		t.Fatal(err)
	}
	data, err = dockerx.Run(context.Background(), "compose", "-f", filepath.Join(configDir, "compose.yaml"), "-f", path, "config", "--format", "json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	dev = result.Services["editor"]
	for _, mount := range dev.Volumes {
		mounts[mount.Target] = mount.Source
	}
	// Recent Compose versions serialize dollars escaped for another Compose load;
	// older versions output the literal values instead.
	value := dev.Environment["VALUE"]
	literal := dev.Environment["LITERAL"]
	if mounts["/workspaces"] != filepath.Join(host, "src") || strings.Join(dev.Command, " ") != "sleep infinity" || (value != "host-$$value" && value != "host-$value") || (literal != "$${CXZ_TEST_COMPOSE_VALUE}" && literal != "${CXZ_TEST_COMPOSE_VALUE}") {
		t.Fatalf("include changed paths, commands or interpolation: %s", data)
	}
	set(`{"services":{"${DEVCONTAINER_SERVICE}":{"privileged":true}}}`)
	if _, err = m.prepareComposeOverride(context.Background(), p); err == nil || !strings.Contains(err.Error(), "requires explicit trust") {
		t.Fatal("override bypassed trust", err)
	}
	p.Trusted = true
	if _, err = m.prepareComposeOverride(context.Background(), p); err != nil {
		t.Fatal("trusted override rejected", err)
	}
	set(`{"services":{"${DEVCONTAINER_SERVICE}":{"not_a_compose_option":true}}}`)
	if _, err = m.prepareComposeOverride(context.Background(), p); err == nil {
		t.Fatal("invalid Compose accepted before recreate")
	}
	// An image devcontainer runs no Compose, so the same intent is translated
	// rather than refused: the override reaches every project or it is a lie.
	write("devcontainer.json", `{"image":"alpine"}`)
	set(`{"services":{"${DEVCONTAINER_SERVICE}":{"volumes":["/host/src:/workspaces:ro"],"environment":["TRANSLATED=yes"]}}}`)
	out, err = m.prepareComposeOverride(context.Background(), p)
	if err != nil || out.ComposeFile != "" || len(out.Digest) != 64 {
		t.Fatal("image devcontainer did not get a translated override", out, err)
	}
	if len(out.Image.Mounts) != 1 || out.Image.Mounts[0] != "type=bind,source=/host/src,target=/workspaces,readonly" {
		t.Fatal("volume not translated", out.Image.Mounts)
	}
	if out.Image.Env["TRANSLATED"] != "yes" {
		t.Fatal("environment not translated", out.Image.Env)
	}
	// What has no image equivalent is named, never dropped.
	set(`{"services":{"${DEVCONTAINER_SERVICE}":{"privileged":true}}}`)
	if _, err = m.prepareComposeOverride(context.Background(), p); err == nil || !strings.Contains(err.Error(), "only volumes and environment translate") {
		t.Fatal("unsupported key silently dropped", err)
	}
	set(`{"services":{"${DEVCONTAINER_SERVICE}":{"volumes":["/a:/b"]},"sidecar":{"image":"redis"}}}`)
	if _, err = m.prepareComposeOverride(context.Background(), p); err == nil || !strings.Contains(err.Error(), "sidecars need a Compose devcontainer") {
		t.Fatal("sidecar silently dropped", err)
	}
	set(`{"services":{"${DEVCONTAINER_SERVICE}":{"volumes":["/host/workspaces:/workspaces"]}}}`)
	if err := projectconfig.Save(m.Root, projectconfig.Spec{}); err != nil {
		t.Fatal(err)
	}
	out, err = m.prepareComposeOverride(context.Background(), p)
	if err != nil || out.ComposeFile != "" || out.Digest != "" || !out.Image.Empty() {
		t.Fatal("stale override still used", out, err)
	}
}
