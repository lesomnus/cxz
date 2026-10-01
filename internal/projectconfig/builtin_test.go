package projectconfig

import (
	"encoding/json"
	"strings"
	"testing"
)

// The built-in default now goes through the same validation and rendering as a
// user's template, so a broken edit to the embedded directory is a test failure
// rather than a project that fails to come up.
func TestBuiltinDevcontainerIsAValidTemplate(t *testing.T) {
	builtin, err := Builtin()
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := builtin.Files["devcontainer.json"]; !exists {
		t.Fatal("embedded default has no devcontainer.json", builtin.Files)
	}
	files, err := builtin.Render("my-project", "/host/my-project")
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]any
	if err = json.Unmarshal(files["devcontainer.json"].Data, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg["name"] != "my-project" {
		t.Fatal("project name not substituted", cfg["name"])
	}
	// Left out of the file on purpose so cxz binds the real workspace; a default
	// that hard-coded them would mount the wrong directory.
	if cfg["workspaceFolder"] != "/workspaces/my-project" {
		t.Fatal("workspace folder not filled in", cfg["workspaceFolder"])
	}
	if mount, _ := cfg["workspaceMount"].(string); !strings.Contains(mount, "source=/host/my-project,") {
		t.Fatal("workspace not bound", cfg["workspaceMount"])
	}
	if cfg["image"] == nil && cfg["build"] == nil && cfg["dockerFile"] == nil {
		t.Fatal("default must build or name an image", cfg)
	}
	// The remote user has to be able to write the workspace, so the default
	// naming one is the whole reason it works unprivileged.
	if remote, _ := cfg["remoteUser"].(string); remote == "" {
		t.Fatal("default must name a remote user", cfg)
	}
	// It is still a devcontainer anyone can read, comments and all.
	if !strings.Contains(string(builtin.Files["devcontainer.json"].Data), "//") {
		t.Fatal("embedded default lost its comments")
	}
}

// The built-in default is the one template a user never edits, so a reader has
// to be able to copy it verbatim into their own directory and have it work.
func TestBuiltinDevcontainerSurvivesACopy(t *testing.T) {
	builtin, err := Builtin()
	if err != nil {
		t.Fatal(err)
	}
	copied := &DefaultTemplate{Files: map[string]TemplateFile{}}
	for name, file := range builtin.Files {
		copied.Files[name] = file
	}
	if err = copied.Validate(); err != nil {
		t.Fatal("a verbatim copy is not a valid template:", err)
	}
}
