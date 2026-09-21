package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/dockerx"
	"github.com/lesomnus/cxz/internal/projectconfig"
	"github.com/tailscale/hujson"
)

func composeFiles(cfg map[string]any, base string) ([]any, error) {
	var files []any
	switch v := cfg["dockerComposeFile"].(type) {
	case nil:
		return nil, nil
	case string:
		files = []any{v}
	case []any:
		files = append(files, v...)
	default:
		return nil, fmt.Errorf("dockerComposeFile must be a path or list of paths")
	}
	for i, f := range files {
		name, ok := f.(string)
		if !ok || name == "" {
			return nil, fmt.Errorf("invalid Compose filename")
		}
		if !filepath.IsAbs(name) {
			name = filepath.Join(base, name)
		}
		files[i] = name
	}
	return files, nil
}

// Prepare one immutable snapshot for preflight and provision. This runs before
// recreation can remove containers; only manager-owned configuration is written.
func (m *Manager) prepareComposeOverride(ctx context.Context, p *Project) (string, string, error) {
	spec, err := projectconfig.Load(m.Root)
	if err != nil || len(spec.Compose) == 0 {
		return "", "", err
	}
	file := p.Config
	if file == "" {
		files := Discover(p.Workspace)
		if len(files) != 1 {
			return "", "", fmt.Errorf("devcontainer.compose requires an existing Compose-based devcontainer; select one with --config")
		}
		file = files[0]
	}
	if !filepath.IsAbs(file) {
		file = filepath.Join(p.Workspace, file)
	}
	b, err := os.ReadFile(file)
	if err != nil {
		return "", "", err
	}
	b, err = hujson.Standardize(b)
	if err != nil {
		return "", "", err
	}
	var cfg map[string]any
	if err = json.Unmarshal(b, &cfg); err != nil {
		return "", "", err
	}
	files, err := composeFiles(cfg, filepath.Dir(file))
	if err != nil {
		return "", "", err
	}
	if len(files) == 0 {
		return "", "", fmt.Errorf("devcontainer.compose requires dockerComposeFile; image/Dockerfile-only devcontainers do not use Compose")
	}
	service, _ := cfg["service"].(string)
	b, err = spec.Render(service)
	if err != nil {
		return "", "", err
	}
	var override map[string]any
	if err = json.Unmarshal(b, &override); err != nil {
		return "", "", err
	}
	if err = CheckTrust(override, p.Trusted); err != nil {
		return "", "", fmt.Errorf("configuration cxz devcontainer.compose: %w", err)
	}
	dir := filepath.Join(m.Root, "projects", p.ID)
	if err = os.MkdirAll(dir, 0700); err != nil {
		return "", "", err
	}
	path := filepath.Join(dir, "compose.user.json")
	if err = core.WriteFile(path, b); err != nil {
		return "", "", err
	}
	if len(m.Owner) < 12 {
		return "", "", fmt.Errorf("invalid manager owner")
	}
	args := []string{"compose", "--project-name", "cxz-" + m.Owner[:12] + "-" + p.ID}
	for _, f := range files {
		args = append(args, "-f", f.(string))
	}
	args = append(args, "-f", path, "config", "--format", "json")
	merged, err := dockerx.Run(ctx, args...)
	if err != nil {
		return "", "", fmt.Errorf("devcontainer.compose preflight: %w", err)
	}
	var resolved map[string]any
	if err = json.Unmarshal(merged, &resolved); err != nil {
		return "", "", fmt.Errorf("devcontainer.compose preflight: %w", err)
	}
	if err = CheckTrust(resolved, p.Trusted); err != nil {
		return "", "", fmt.Errorf("configuration merged devcontainer.compose: %w", err)
	}
	hash := sha256.Sum256(b)
	return path, hex.EncodeToString(hash[:]), nil
}
