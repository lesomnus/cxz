package projectconfig

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
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
	// The default is Compose, like this repository's own devcontainer, so the
	// workspace is the Compose file's business and cxz must not fill in a mount.
	if cfg["dockerComposeFile"] == nil || cfg["service"] == "" || cfg["workspaceFolder"] == "" {
		t.Fatal("Compose default needs a file, a service and a workspace folder", cfg)
	}
	if cfg["workspaceMount"] != nil {
		t.Fatal("cxz overrode a Compose template's own mount", cfg["workspaceMount"])
	}
	// The one thing a Compose template cannot do without substitution: name the
	// workspace. It is materialized outside it, so `..` would be the snapshot.
	compose := string(files["docker-compose.yaml"].Data)
	if strings.Contains(compose, WorkspaceVariable) {
		t.Fatal("workspace variable survived rendering", compose)
	}
	if !strings.Contains(compose, "/host/my-project:/"+strings.TrimPrefix(cfg["workspaceFolder"].(string), "/")) {
		t.Fatal("workspace not bound into the Compose file", compose)
	}
	// Volume scope is deliberate and opposite for the two kinds. Compose prefixes
	// a bare declaration with the project, so shell history stays per project; a
	// pinned name escapes that prefix, so a content-addressed cache is one cache
	// for the machine. Getting either backwards is silent until it bites.
	var declared struct {
		Volumes map[string]struct {
			Name     string `yaml:"name"`
			External bool   `yaml:"external"`
		} `yaml:"volumes"`
	}
	if err = yaml.Unmarshal([]byte(compose), &declared); err != nil {
		t.Fatal(err)
	}
	for volume, want := range map[string]string{
		"command.history": "",
		"go.cache.mod":    "cxz-go-cache-mod",
		"go.cache.bin":    "cxz-go-cache-build",
	} {
		got, exists := declared.Volumes[volume]
		if !exists {
			t.Fatal("default lost its", volume, "volume", declared.Volumes)
		}
		if got.Name != want {
			t.Fatal(volume, "pinned name is", got.Name, "want", want)
		}
		// Compose creates a pinned volume but refuses a missing external one, so
		// external would make a fresh machine fail to start a project.
		if got.External {
			t.Fatal(volume, "is external; a fresh machine would have no such volume")
		}
	}
	// It is still a devcontainer anyone can read, comments and all.
	if !strings.Contains(string(builtin.Files["devcontainer.json"].Data), "//") {
		t.Fatal("embedded default lost its comments")
	}
	if !strings.Contains(string(builtin.Files["docker-compose.yaml"].Data), "#") {
		t.Fatal("embedded Compose file lost its comments")
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
