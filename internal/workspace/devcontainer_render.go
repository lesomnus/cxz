package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/devcontainerrender"
	"github.com/lesomnus/cxz/internal/dockerx"
)

// Rendering reports the devcontainer a project actually runs under: the
// configuration cxz handed the devcontainer CLI, and the Compose files it was
// told to merge, in the order they are merged. It reads back what provisioning
// wrote instead of rendering a second time, because a second renderer is a
// second answer, and the one that is easy to look at is the one that would
// quietly drift from the one that runs.

func (m *Manager) renderDevcontainer(ctx context.Context, spec []byte) (*api.Receipt, error) {
	var r devcontainerrender.Request
	if err := json.Unmarshal(spec, &r); err != nil {
		return nil, err
	}
	p, err := m.resolveNearest(ctx, r.Project)
	if err != nil {
		return nil, err
	}
	reply, err := m.renderProjectDevcontainer(ctx, p)
	if err != nil {
		return nil, err
	}
	b, err := json.Marshal(reply)
	if err != nil {
		return nil, err
	}
	return &api.Receipt{Status: string(b)}, nil
}

// resolveNearest accepts the directory the question was asked from, which is
// usually inside a workspace rather than at its root -- a .devcontainer
// directory is exactly where someone wonders which devcontainer is in effect.
// Only a registered workspace matches, so walking up cannot widen the answer,
// and the reply names the project it settled on.
func (m *Manager) resolveNearest(ctx context.Context, handle string) (*Project, error) {
	p, err := m.resolve(ctx, handle)
	if err == nil || !filepath.IsAbs(handle) {
		return p, err
	}
	for dir := filepath.Dir(handle); ; dir = filepath.Dir(dir) {
		if found, e := m.resolve(ctx, dir); e == nil {
			return found, nil
		}
		if parent := filepath.Dir(dir); parent == dir {
			return nil, err // The original error names what was actually asked for.
		}
	}
}

func (m *Manager) renderProjectDevcontainer(ctx context.Context, p *Project) (devcontainerrender.Reply, error) {
	out := devcontainerrender.Reply{Project: p.ID, Name: p.Name, Workspace: p.Workspace}
	dir := filepath.Join(m.Root, "projects", p.ID)
	config := filepath.Join(dir, "devcontainer.json")
	b, err := os.ReadFile(config)
	if os.IsNotExist(err) {
		return out, fmt.Errorf("project %s has no provisioned devcontainer yet; run cxz up first", p.Workspace)
	}
	if err != nil {
		return out, err
	}
	total := 0
	add := func(name, source, role string, data []byte) error {
		total += len(data)
		if total > devcontainerrender.MaxBytes {
			return fmt.Errorf("devcontainer configuration exceeds %d bytes", devcontainerrender.MaxBytes)
		}
		out.Files = append(out.Files, devcontainerrender.File{Name: name, Source: source, Role: role, Data: data})
		return nil
	}
	if err = add("devcontainer.json", config, "what cxz passed to the devcontainer CLI: the project's own configuration plus cxz's environment, mounts and startup hook", b); err != nil {
		return out, err
	}
	var cfg map[string]any
	if err = json.Unmarshal(b, &cfg); err != nil {
		return out, err
	}
	// The project's own file is read during provisioning and never written, so
	// there is nothing on disk that records which one was read. Discovery is
	// repeated here, and labelled as a repeat rather than as a record.
	if source, err := configurationPath(p); err == nil && source != "" {
		if b, err := os.ReadFile(source); err == nil {
			if err = add("source/"+filepath.Base(source), source, "the project's own configuration, as discovery finds it now", b); err != nil {
				return out, err
			}
		}
	}
	var files []string
	switch v := cfg["dockerComposeFile"].(type) {
	case string:
		files = []string{v}
	case []any:
		for _, f := range v {
			if s, ok := f.(string); ok {
				files = append(files, s)
			}
		}
	}
	if len(files) == 0 {
		out.Note = "This devcontainer is image or Dockerfile based, so there are no Compose files to merge. A shared Compose override, if any, was translated into the mounts and environment above."
		return out, nil
	}
	for i, file := range files {
		role := "a Compose file from the project's devcontainer"
		switch file {
		case filepath.Join(dir, "compose.user.json"):
			role = "the installation's shared Compose override (cxz edit docker-compose)"
		case filepath.Join(dir, "compose.json"):
			role = "cxz's own Compose override: the project's labels, network and state volumes. Merged last, so it wins"
		}
		b, err := os.ReadFile(file)
		if err != nil {
			return out, err
		}
		if err = add(fmt.Sprintf("compose/%02d-%s", i+1, filepath.Base(file)), file, role, b); err != nil {
			return out, err
		}
	}
	// The resolved merge is the answer to most questions worth asking -- a
	// volume's final name, an overridden image -- but it needs the Compose CLI.
	// A manager without one should still report the files it has.
	resolved, err := m.resolvedCompose(ctx, p, files)
	if err != nil {
		out.Note = "The merged result is unavailable: " + err.Error()
		return out, nil
	}
	return out, add(devcontainerrender.ResolvedCompose, "docker compose config", "the merge of the files above, as Compose resolves it: the values that actually take effect", resolved)
}

// resolvedCompose merges exactly the file list the devcontainer CLI is given,
// under the project name cxz pins, so a volume's prefix in the result is the
// prefix the project really gets.
func (m *Manager) resolvedCompose(ctx context.Context, p *Project, files []string) ([]byte, error) {
	if len(m.Owner) < 12 {
		return nil, fmt.Errorf("invalid manager owner")
	}
	args := []string{"compose", "--project-name", "cxz-" + m.Owner[:12] + "-" + p.ID}
	for _, file := range files {
		args = append(args, "-f", file)
	}
	return dockerx.Run(ctx, append(args, "config")...)
}
