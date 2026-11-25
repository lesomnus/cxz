package workspace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/lesomnus/cxz/internal/projectconfig"
	"github.com/lesomnus/cxz/internal/projectspec"
)

func specManager(t *testing.T) *Manager {
	return &Manager{Root: t.TempDir(), Owner: strings.Repeat("o", 24), ToolsVolume: "tools", StateVolume: "statevol"}
}

func TestRuntimeSpecStateMount(t *testing.T) {
	m := specManager(t)
	own := &Project{ID: "pid", StorageID: "sid", Workspace: "/host/proj", Network: "net", Volume: "projvol"}
	spec, err := m.runtimeSpec(own, map[string]any{"image": "img", "workspaceFolder": "/w/${localWorkspaceFolderBasename}"}, "", "/s", "/a")
	if err != nil {
		t.Fatal(err)
	}
	if spec.Mounts[0] != (projectspec.Mount{Type: projectspec.Volume, Source: "projvol", Target: "/cxz/state"}) {
		t.Fatal("a project with its own volume mounts it", spec.Mounts[0])
	}
	if spec.Image != "img" || spec.Workspace != "/w/proj" || spec.HostWorkspace != "/host/proj" || spec.Network != "net" {
		t.Fatalf("%+v", spec)
	}
	shared := &Project{ID: "pid", StorageID: "sid", Workspace: "/host/proj", Network: "net"}
	spec, err = m.runtimeSpec(shared, map[string]any{}, "tcp://d:2375", "/s", "/a")
	if err != nil {
		t.Fatal(err)
	}
	if spec.Mounts[0] != (projectspec.Mount{Type: projectspec.Volume, Source: "statevol", Subpath: "projects/sid/state", Target: "/cxz/state"}) {
		t.Fatal("a project without a volume mounts its subdirectory of the manager's", spec.Mounts[0])
	}
	if spec.Workspace != "/workspaces/proj" || spec.Env["DOCKER_HOST"] != "tcp://d:2375" {
		t.Fatalf("%+v", spec)
	}
	m.StateVolume = ""
	if _, err = m.runtimeSpec(shared, map[string]any{}, "", "/s", "/a"); err == nil {
		t.Fatal("a project with nowhere to keep its state was accepted")
	}
}

// The renderers see only the Spec and the project's own configuration: a Spec
// built by hand, with no Project behind it, is enough to start a container.
func handSpec() projectspec.Spec {
	return projectspec.Spec{
		Owner: strings.Repeat("o", 24), Project: "p",
		Env: map[string]string{"CXZ_STATE": projectspec.StateDir, "CXZ_PROJECT_ID": "id"},
		Mounts: []projectspec.Mount{
			{Type: projectspec.Volume, Source: "state", Subpath: "projects/p/state", Target: "/cxz/state"},
			{Type: projectspec.Volume, Source: "tools", Target: "/cxz/tools", ReadOnly: true},
			{Type: projectspec.Bind, Source: "/secrets", Target: "/cxz/secrets"},
		},
		Workspace: "/workspace", Start: []string{"/cxz/tools/cxz", "_boot"}, Network: "net",
	}
}

func TestApplySpecToImageConfiguration(t *testing.T) {
	m := specManager(t)
	cfg := map[string]any{"image": "img", "containerEnv": map[string]any{"A": "1", "CXZ_STATE": "mine"}, "mounts": []any{"type=bind,source=/x,target=/y"}, "runArgs": []any{"--init"}, "postStartCommand": "echo hi"}
	shared := projectconfig.ImageOverride{Mounts: []string{"type=volume,source=s,target=/s"}, Env: map[string]string{"S": "1", "CXZ_PROJECT_ID": "theirs"}}
	compose, err := m.applySpec(cfg, handSpec(), shared, nil)
	if err != nil || compose != nil {
		t.Fatal(compose, err)
	}
	want := map[string]any{
		"image":            "img",
		"containerEnv":     map[string]any{"A": "1", "S": "1", "CXZ_STATE": projectspec.StateDir, "CXZ_PROJECT_ID": "id"},
		"mounts":           []any{"type=bind,source=/x,target=/y", "type=volume,source=s,target=/s", "type=volume,source=state,target=/cxz/state,volume-subpath=projects/p/state", "type=volume,source=tools,target=/cxz/tools,readonly"},
		"runArgs":          []any{"--init", "--network", "net", "--mount", "type=bind,source=/secrets,target=/cxz/secrets"},
		"postStartCommand": map[string]any{"project": "echo hi", "cxz-runtime": "/cxz/tools/cxz _boot"},
		"workspaceFolder":  "/workspace",
	}
	if !reflect.DeepEqual(cfg, want) {
		got, _ := json.MarshalIndent(cfg, "", " ")
		t.Fatalf("%s", got)
	}
	if _, err = m.applySpec(map[string]any{"postStartCommand": map[string]any{"cxz-runtime": "x"}}, handSpec(), projectconfig.ImageOverride{}, nil); err == nil {
		t.Fatal("a project took over the runtime hook")
	}
}

func TestApplySpecToComposeConfiguration(t *testing.T) {
	m := specManager(t)
	file := filepath.Join(t.TempDir(), "compose.yaml")
	if err := os.WriteFile(file, []byte("services:\n  dev:\n    image: x\n    networks: [front]\n  db:\n    image: y\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := map[string]any{"dockerComposeFile": "compose.yaml", "service": "dev", "mounts": []any{"type=bind,source=/x,target=/y"}}
	compose, err := m.applySpec(cfg, handSpec(), projectconfig.ImageOverride{}, []any{file})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cfg["mounts"], []any{"type=bind,source=/x,target=/y"}) {
		t.Fatal("cxz's mounts belong in the Compose file, not the configuration", cfg["mounts"])
	}
	b, _ := json.Marshal(compose)
	var got map[string]any
	json.Unmarshal(b, &got)
	labels := map[string]any{"cxz.owner": strings.Repeat("o", 24), "cxz.project": "p"}
	want := map[string]any{
		"name": "cxz-oooooooooooo-p",
		"services": map[string]any{
			"db": map[string]any{"labels": labels},
			"dev": map[string]any{
				"labels":      labels,
				"networks":    map[string]any{"front": nil, "cxz": nil},
				"environment": map[string]any{"CXZ_STATE": projectspec.StateDir, "CXZ_PROJECT_ID": "id"},
				"volumes": []any{
					map[string]any{"type": "volume", "source": "state", "target": "/cxz/state", "volume": map[string]any{"subpath": "projects/p/state"}},
					map[string]any{"type": "volume", "source": "tools", "target": "/cxz/tools", "read_only": true},
					map[string]any{"type": "bind", "source": "/secrets", "target": "/cxz/secrets"},
				},
			},
		},
		"networks": map[string]any{"cxz": map[string]any{"external": true, "name": "net"}},
		"volumes":  map[string]any{"state": map[string]any{"external": true, "name": "state"}, "tools": map[string]any{"external": true, "name": "tools"}},
	}
	if !reflect.DeepEqual(got, want) {
		out, _ := json.MarshalIndent(got, "", " ")
		t.Fatalf("%s", out)
	}
	if _, err = m.applySpec(map[string]any{"dockerComposeFile": "compose.yaml"}, handSpec(), projectconfig.ImageOverride{}, []any{file}); err == nil {
		t.Fatal("a Compose devcontainer without a service was accepted")
	}
}
