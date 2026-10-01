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

// Override is one prepared shared override. Exactly one field is set: a Compose
// devcontainer gets another Compose file to merge, while an image or Dockerfile
// devcontainer gets the same intent translated, since it never runs Compose.
type Override struct {
	ComposeFile string
	Image       projectconfig.ImageOverride
	Digest      string
}

// Prepare one immutable snapshot for preflight and provision. This runs before
// recreation can remove containers; only manager-owned configuration is written.
func (m *Manager) prepareComposeOverride(ctx context.Context, p *Project) (Override, error) {
	var out Override
	spec, err := projectconfig.Load(m.Root)
	if err != nil || len(spec.Compose) == 0 {
		return out, err
	}
	file := p.Config
	if file == "" {
		files := Discover(p.Workspace)
		if len(files) != 1 {
			return out, fmt.Errorf("devcontainer.compose requires an existing devcontainer; select one with --config")
		}
		file = files[0]
	}
	if !filepath.IsAbs(file) {
		file = filepath.Join(p.Workspace, file)
	}
	b, err := os.ReadFile(file)
	if err != nil {
		return out, err
	}
	b, err = hujson.Standardize(b)
	if err != nil {
		return out, err
	}
	var cfg map[string]any
	if err = json.Unmarshal(b, &cfg); err != nil {
		return out, err
	}
	files, err := composeFiles(cfg, filepath.Dir(file))
	if err != nil {
		return out, err
	}
	if len(files) == 0 {
		// No Compose to merge into, so translate instead of refusing. The
		// override exists to reach every project, and a project with no
		// devcontainer of its own is exactly the case it was written for.
		image, err := spec.RenderImage()
		if err != nil {
			return out, err
		}
		if err = CheckTrust(map[string]any{"mounts": image.Mounts}, p.Trusted); err != nil {
			return out, fmt.Errorf("configuration cxz devcontainer.compose: %w", err)
		}
		digest, err := json.Marshal(image)
		if err != nil {
			return out, err
		}
		hash := sha256.Sum256(digest)
		return Override{Image: image, Digest: hex.EncodeToString(hash[:])}, nil
	}
	service, _ := cfg["service"].(string)
	b, err = spec.Render(service)
	if err != nil {
		return out, err
	}
	var override map[string]any
	if err = json.Unmarshal(b, &override); err != nil {
		return out, err
	}
	if err = CheckTrust(override, p.Trusted); err != nil {
		return out, fmt.Errorf("configuration cxz devcontainer.compose: %w", err)
	}
	dir := filepath.Join(m.Root, "projects", p.ID)
	if err = os.MkdirAll(dir, 0700); err != nil {
		return out, err
	}
	path := filepath.Join(dir, "compose.user.json")
	if err = core.WriteFile(path, b); err != nil {
		return out, err
	}
	if len(m.Owner) < 12 {
		return out, fmt.Errorf("invalid manager owner")
	}
	args := []string{"compose", "--project-name", "cxz-" + m.Owner[:12] + "-" + p.ID}
	for _, f := range files {
		args = append(args, "-f", f.(string))
	}
	args = append(args, "-f", path, "config", "--format", "json")
	merged, err := dockerx.Run(ctx, args...)
	if err != nil {
		return out, fmt.Errorf("devcontainer.compose preflight: %w", err)
	}
	var resolved map[string]any
	if err = json.Unmarshal(merged, &resolved); err != nil {
		return out, fmt.Errorf("devcontainer.compose preflight: %w", err)
	}
	if err = CheckTrust(resolved, p.Trusted); err != nil {
		return out, fmt.Errorf("configuration merged devcontainer.compose: %w", err)
	}
	hash := sha256.Sum256(b)
	return Override{ComposeFile: path, Digest: hex.EncodeToString(hash[:])}, nil
}
