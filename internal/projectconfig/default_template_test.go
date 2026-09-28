package projectconfig

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultTemplateSnapshotRenderAndClear(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, TemplateDirectory)
	if err := os.MkdirAll(filepath.Join(dir, "scripts"), 0700); err != nil {
		t.Fatal(err)
	}
	config := `{// JSONC is supported
 "name":"${cxz:projectName}", "build":{"dockerfile":"Dockerfile"},
 "containerEnv":{"PROJECT":"${cxz:projectName}","NATIVE":"${localWorkspaceFolderBasename}"},
 "postCreateCommand":["echo", "${cxz:projectName}"],
 }`
	for name, b := range map[string]string{"devcontainer.json": config, "Dockerfile": "FROM alpine\n", "scripts/setup.sh": "#!/bin/sh\necho ok\n"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(b), 0700); err != nil {
			t.Fatal(err)
		}
	}
	spec, err := (Config{}).Snapshot(root)
	if err != nil || spec.Template == nil {
		t.Fatal(spec, err)
	}
	name := "repo\"name"
	files, err := spec.Template.Render(name, "/source/checkout")
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(files["devcontainer.json"].Data, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg["name"] != name || cfg["workspaceMount"] != "source=/source/checkout,target=/workspaces/checkout,type=bind" || cfg["containerEnv"].(map[string]any)["NATIVE"] != "${localWorkspaceFolderBasename}" {
		t.Fatal(cfg)
	}
	if !files["scripts/setup.sh"].Executable || string(spec.Template.Files["devcontainer.json"].Data) != config {
		t.Fatal("lost executable mode or mutated original")
	}
	if err := Save(root, spec); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	saved, err := Load(root)
	if err != nil || saved.Template == nil {
		t.Fatal("snapshot depends on source", err)
	}
	empty, err := (Config{}).Snapshot(root)
	if err != nil || empty.Template != nil {
		t.Fatal("removed template retained", err)
	}
}

func TestDefaultTemplateRejectsInvalidSnapshots(t *testing.T) {
	for _, name := range []string{"../escape", "/absolute", `a\b`, "a:b", "scripts/run/child"} {
		files := map[string]TemplateFile{"devcontainer.json": {Data: []byte(`{"image":"alpine"}`)}, "scripts/run": {Data: []byte("x")}, name: {Data: []byte("x")}}
		if err := (&DefaultTemplate{Files: files}).Validate(); err == nil {
			t.Fatal("unsafe path accepted", name)
		}
	}
	for _, config := range []string{`null`, `{`, `{}`, `{"image":42}`, `{"build":{"dockerfile":"missing"}}`, `{"dockerComposeFile":[],"service":"dev","workspaceFolder":"/workspace"}`} {
		template := &DefaultTemplate{Files: map[string]TemplateFile{"devcontainer.json": {Data: []byte(config)}}}
		if err := template.Validate(); err == nil {
			t.Fatal("invalid config accepted", config)
		}
	}
	root := t.TempDir()
	dir := filepath.Join(root, TemplateDirectory)
	os.MkdirAll(dir, 0700)
	if _, err := (Config{}).Snapshot(root); err == nil {
		t.Fatal("empty directory silently ignored")
	}
	os.WriteFile(filepath.Join(dir, "devcontainer.json"), []byte(`{"image":"alpine"}`), 0600)
	os.WriteFile(filepath.Join(dir, "large"), []byte(strings.Repeat("x", MaxTemplateBytes)), 0600)
	if _, err := (Config{}).Snapshot(root); err == nil {
		t.Fatal("oversized template accepted")
	}
	os.Remove(filepath.Join(dir, "large"))
	if err := os.Symlink("devcontainer.json", filepath.Join(dir, "link")); err == nil {
		if _, err := (Config{}).Snapshot(root); err == nil {
			t.Fatal("symlink accepted")
		}
	}
}
