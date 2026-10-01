package workspace

import (
	"fmt"
	"path/filepath"
	"strings"

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
	// The installation's template and the built-in one materialize through the
	// same path, so the fallback is a devcontainer anyone can read rather than a
	// literal, and both obey the same rules. p.Config stays empty either way, so
	// adding a real devcontainer later is still discovered.
	template := spec.Template
	if template == nil {
		if template, err = projectconfig.Builtin(); err != nil {
			return "", err
		}
	}
	return m.materializeTemplate(p, template)
}
