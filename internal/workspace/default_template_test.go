package workspace

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lesomnus/cxz/internal/projectconfig"
)

func TestTemplateProjectName(t *testing.T) {
	root := filepath.Join(t.TempDir(), "checkout")
	os.MkdirAll(root, 0700)
	if got := projectTemplateName(root); got != "checkout" {
		t.Fatal(got)
	}
	if got := projectTemplateName("/"); got != "workspace" {
		t.Fatal(got)
	}
	git := func(args ...string) {
		t.Helper()
		if b, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput(); err != nil {
			t.Fatal(string(b), err)
		}
	}
	git("init")
	child := filepath.Join(root, "subdirectory")
	os.MkdirAll(child, 0700)
	if got := projectTemplateName(child); got != "checkout" {
		t.Fatal(got)
	}
	for _, origin := range []string{"git@github.com:org/my-app.git", "https://github.com/org/my-app.git", "ssh://git@example.com/org/my-app.git", "/repos/my-app.git"} {
		git("config", "remote.origin.url", origin)
		if got := projectTemplateName(child); got != "my-app" {
			t.Fatal(origin, got)
		}
	}
	git("-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "--allow-empty", "-m", "init")
	worktree := filepath.Join(t.TempDir(), "temporary-worktree")
	git("worktree", "add", "--detach", worktree)
	if got := projectTemplateName(worktree); got != "my-app" {
		t.Fatal("worktree name", got)
	}
}

func TestManagedTemplatePrecedenceAndImmutableSnapshots(t *testing.T) {
	root, workspace := t.TempDir(), filepath.Join(t.TempDir(), "my-project")
	os.MkdirAll(workspace, 0700)
	m := &Manager{Root: root}
	p := &Project{ID: "id", Workspace: workspace}
	template := &projectconfig.DefaultTemplate{Files: map[string]projectconfig.TemplateFile{
		"devcontainer.json": {Data: []byte(`{"name":"${cxz:projectName}","build":{"dockerfile":"Dockerfile"}}`)},
		"Dockerfile":        {Data: []byte("FROM alpine\n")},
		"setup.sh":          {Data: []byte("#!/bin/sh\n"), Executable: true},
	}}
	save := func() {
		t.Helper()
		if err := projectconfig.Save(root, projectconfig.Spec{Template: template}); err != nil {
			t.Fatal(err)
		}
	}
	save()
	file, err := m.provisionConfiguration(p)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(file)
	var cfg map[string]any
	json.Unmarshal(b, &cfg)
	if cfg["name"] != "my-project" || !strings.HasPrefix(file, root) || p.Config != "" {
		t.Fatal(cfg, file)
	}
	if entries, _ := os.ReadDir(workspace); len(entries) != 0 {
		t.Fatal("workspace modified")
	}
	if info, err := os.Stat(filepath.Join(filepath.Dir(file), "setup.sh")); err != nil || info.Mode()&0100 == 0 {
		t.Fatal("script mode lost", err)
	}
	if again, err := m.provisionConfiguration(p); err != nil || again != file {
		t.Fatal("snapshot unstable", again, err)
	}
	template.Files["Dockerfile"] = projectconfig.TemplateFile{Data: []byte("FROM debian\n")}
	save()
	updated, err := m.provisionConfiguration(p)
	if err != nil || updated == file {
		t.Fatal("edit reused old snapshot", err)
	}
	if b, _ := os.ReadFile(filepath.Join(filepath.Dir(file), "Dockerfile")); string(b) != "FROM alpine\n" {
		t.Fatal("existing snapshot overwritten")
	}
	projectFile := filepath.Join(workspace, ".devcontainer.json")
	os.WriteFile(projectFile, []byte(`{"image":"alpine"}`), 0600)
	if got, err := m.provisionConfiguration(p); err != nil || got != projectFile {
		t.Fatal("project config not preferred", got, err)
	}
	p.Config = "explicit.json"
	if got, err := m.provisionConfiguration(p); err != nil || got != filepath.Join(workspace, p.Config) {
		t.Fatal("explicit config not preferred", got, err)
	}
	p.Config = updated
	if err := preflight(p); err != nil {
		t.Fatal(err)
	}
	template.Files["devcontainer.json"] = projectconfig.TemplateFile{Data: []byte(`{"image":"alpine","privileged":true}`)}
	save()
	p.Config = ""
	os.Remove(projectFile)
	dangerous, err := m.provisionConfiguration(p)
	if err != nil {
		t.Fatal(err)
	}
	configured := *p
	configured.Config = dangerous
	if err := preflight(&configured); err == nil {
		t.Fatal("template bypassed trust preflight")
	}
}

func TestDefaultComposeTemplateWithHostOverride(t *testing.T) {
	if _, err := exec.Command("docker", "compose", "version").Output(); err != nil {
		t.Skip("Docker Compose CLI required")
	}
	m := &Manager{Root: t.TempDir(), Owner: "123456789012345678901234"}
	p := &Project{ID: "template", Workspace: t.TempDir()}
	spec := projectconfig.Spec{
		Template: &projectconfig.DefaultTemplate{Files: map[string]projectconfig.TemplateFile{
			"devcontainer.json": {Data: []byte(`{"name":"${cxz:projectName}","dockerComposeFile":"compose.yaml","service":"dev","workspaceFolder":"/workspace"}`)},
			"compose.yaml":      {Data: []byte("services:\n  dev:\n    image: alpine\n    command: [sleep, infinity]\n    volumes:\n      - ${cxz:workspace}:/workspace\n")},
		}},
		Compose: json.RawMessage(`{"services":{"${DEVCONTAINER_SERVICE}":{"environment":{"FROM_OVERRIDE":"yes"}}}}`),
	}
	if err := projectconfig.Save(m.Root, spec); err != nil {
		t.Fatal(err)
	}
	file, err := m.provisionConfiguration(p)
	if err != nil {
		t.Fatal(err)
	}
	configured := *p
	configured.Config = file
	if err := preflight(&configured); err != nil {
		t.Fatal(err)
	}
	// A Compose template mounts the workspace itself, so the materialized file
	// has to carry the real host path rather than a relative bind into the
	// snapshot it was written beside.
	compose, err := os.ReadFile(filepath.Join(filepath.Dir(file), "compose.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(compose), p.Workspace+":/workspace") {
		t.Fatal("workspace variable not substituted into the Compose file", string(compose))
	}
	override, err := m.prepareComposeOverride(context.Background(), &configured)
	if err != nil || override.ComposeFile == "" || len(override.Digest) != 64 {
		t.Fatal(override, err)
	}
	if p.Config != "" {
		t.Fatal("fallback pinned into project")
	}
}
