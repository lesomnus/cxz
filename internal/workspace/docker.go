package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/engine"
)

func (m *Manager) dockerEngine() engine.Engine { return engine.Engine{Root: m.Root, Owner: m.Owner} }
func (m *Manager) Docker(ctx context.Context, r *api.DockerInput) (*api.Receipt, error) {
	m.dockerMu.Lock()
	defer m.dockerMu.Unlock()
	e := m.dockerEngine()
	switch r.Action {
	case "save", "up":
		var spec engine.Spec
		if err := json.Unmarshal(r.Spec, &spec); err != nil {
			return nil, err
		}
		if spec.Mode == "dind" {
			if _, _, err := e.Render(ctx, spec); err != nil {
				return nil, err
			}
		}
		if err := e.Save(spec); err != nil {
			return nil, err
		}
		if r.Action == "save" {
			return &api.Receipt{Status: "Docker settings saved; project recreation applies connection settings; cxz docker up applies engine changes"}, nil
		}
		if spec.Mode != "dind" {
			return nil, fmt.Errorf("set docker.mode to dind in cxz edit first")
		}
		if err := e.Ensure(ctx, spec, true); err != nil {
			return nil, err
		}
		projects, err := m.all(ctx)
		if err != nil {
			return nil, err
		}
		for _, p := range projects {
			if p.ContainerID != "" {
				if err = e.Connect(ctx, p.Network); err != nil {
					return nil, err
				}
			}
		}
		return &api.Receipt{Status: e.Endpoint()}, nil
	case "down":
		if err := e.Down(ctx); err != nil {
			return nil, err
		}
		return &api.Receipt{Status: "Docker engine removed; cache volume retained"}, nil
	case "status":
		status, err := e.Status(ctx)
		return &api.Receipt{Status: status}, err
	default:
		return nil, fmt.Errorf("unknown Docker action")
	}
}
func (m *Manager) ensureDocker(ctx context.Context, p *Project) (string, error) {
	m.dockerMu.Lock()
	defer m.dockerMu.Unlock()
	e := m.dockerEngine()
	spec, err := e.Load()
	if err != nil {
		return "", err
	}
	if spec.Mode != "dind" {
		return "", nil
	}
	if err = e.Ensure(ctx, spec, false); err != nil {
		return "", err
	}
	if err = e.Connect(ctx, p.Network); err != nil {
		return "", err
	}
	return e.Endpoint(), nil
}
