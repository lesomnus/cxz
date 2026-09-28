package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/lesomnus/cxz/internal/projectconfig"
)

func projectTemplateName(workspace string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	// Manager runs as a different UID from the host checkout. These read-only
	// Git queries opt into reading its metadata without modifying Git settings.
	git := func(args ...string) string {
		cmd := exec.CommandContext(ctx, "git", append([]string{"-c", "safe.directory=*", "-C", workspace}, args...)...)
		b, err := cmd.Output()
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(b))
	}
	if root := git("rev-parse", "--show-toplevel"); root != "" {
		if origin := git("config", "--get", "remote.origin.url"); origin != "" {
			if strings.Contains(origin, "://") {
				if u, err := url.Parse(origin); err == nil {
					origin = u.Path
				} else {
					origin = ""
				}
			} else if i := strings.Index(origin, ":"); i >= 0 {
				origin = origin[i+1:]
			}
			name := strings.TrimSuffix(filepath.Base(strings.TrimRight(origin, "/")), ".git")
			if name != "" && name != "." && name != "/" {
				return name
			}
		}
		workspace = root
	}
	name := filepath.Base(filepath.Clean(workspace))
	if name == "" || name == "." || name == string(filepath.Separator) {
		return "workspace"
	}
	return name
}

func (m *Manager) materializeTemplate(p *Project, t *projectconfig.DefaultTemplate) (string, error) {
	files, err := t.Render(projectTemplateName(p.Workspace), p.Workspace)
	if err != nil {
		return "", err
	}
	b, err := json.Marshal(files)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	parent := filepath.Join(m.Root, "projects", p.ID, "templates")
	target := filepath.Join(parent, hex.EncodeToString(sum[:]))
	config := filepath.Join(target, "devcontainer.json")
	if _, err := os.Stat(config); err == nil {
		return config, nil
	}
	if err := os.MkdirAll(parent, 0700); err != nil {
		return "", err
	}
	temp, err := os.MkdirTemp(parent, ".prepare-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(temp)
	for name, f := range files {
		file := filepath.Join(temp, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
			return "", err
		}
		mode := os.FileMode(0600)
		if f.Executable {
			mode = 0700
		}
		if err := os.WriteFile(file, f.Data, mode); err != nil {
			return "", err
		}
	}
	if err := os.Rename(temp, target); err != nil {
		return "", fmt.Errorf("publish default devcontainer template: %w", err)
	}
	return config, nil
}
