package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/projectconfig"
)

func configurationPath(p *Project) (string, error) {
	if p.Config != "" {
		file := p.Config
		if !filepath.IsAbs(file) {
			file = filepath.Join(p.Workspace, file)
		}
		return file, nil
	}
	files := Discover(p.Workspace)
	switch len(files) {
	case 0:
		return "", nil
	case 1:
		return files[0], nil
	default:
		return "", fmt.Errorf("multiple devcontainer configurations: specify --config (%s)", strings.Join(files, ", "))
	}
}

func (m *Manager) provisionConfiguration(p *Project) (string, error) {
	file, err := configurationPath(p)
	if err != nil || file != "" {
		return file, err
	}
	spec, err := projectconfig.Load(m.Root)
	if err != nil {
		return "", err
	}
	if spec.Template != nil {
		return m.materializeTemplate(p, spec.Template)
	}
	file = filepath.Join(m.Root, "projects", p.ID, "default-devcontainer.json")
	if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
		return "", err
	}
	// Leave Config empty so adding a real devcontainer later is discovered.
	err = core.WriteJSON(file, map[string]any{
		"name": p.Name, "image": "mcr.microsoft.com/devcontainers/base:bookworm", "remoteUser": "vscode",
		"workspaceFolder": "/workspaces/" + filepath.Base(p.Workspace),
		"workspaceMount":  "source=" + p.Workspace + ",target=/workspaces/" + filepath.Base(p.Workspace) + ",type=bind",
	})
	return file, err
}
