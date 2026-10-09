package workspace

import (
	"context"
	"fmt"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/cxzupdate"
	"github.com/lesomnus/cxz/internal/engine"
	"github.com/lesomnus/cxz/internal/versionpin"
)

func (m *Manager) dockerEngine() engine.Engine { return engine.Engine{Root: m.Root, Owner: m.Owner} }

// SaveEngine writes the configuration without applying it. Connection settings
// reach a project when it is recreated; engine changes need StartEngine.
func (m *Manager) SaveEngine(ctx context.Context, r *api.SaveEngineInput) (*api.EngineReply, error) {
	m.dockerMu.Lock()
	defer m.dockerMu.Unlock()
	e := m.dockerEngine()
	spec := engineSpec(r.Spec)
	if err := m.renderIfManaged(ctx, e, spec); err != nil {
		return nil, err
	}
	if err := e.Save(spec); err != nil {
		return nil, err
	}
	return &api.EngineReply{Status: "Docker settings saved; project recreation applies connection settings; cxz docker up applies engine changes"}, nil
}

// StartEngine saves and applies. An absent spec applies the one already saved,
// which is how `cxz docker up` with nothing to publish works.
func (m *Manager) StartEngine(ctx context.Context, r *api.StartEngineInput) (*api.EngineReply, error) {
	m.dockerMu.Lock()
	defer m.dockerMu.Unlock()
	e := m.dockerEngine()
	spec := engineSpec(r.Spec)
	if r.Spec == nil {
		var err error
		if spec, err = e.Load(); err != nil {
			return nil, err
		}
	}
	if err := m.renderIfManaged(ctx, e, spec); err != nil {
		return nil, err
	}
	if err := e.Save(spec); err != nil {
		return nil, err
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
	return &api.EngineReply{Status: e.Endpoint()}, nil
}

func (m *Manager) StopEngine(ctx context.Context, _ *api.Empty) (*api.EngineReply, error) {
	m.dockerMu.Lock()
	defer m.dockerMu.Unlock()
	if err := m.dockerEngine().Down(ctx); err != nil {
		return nil, err
	}
	return &api.EngineReply{Status: "Docker engine removed; cache volume retained"}, nil
}

func (m *Manager) PruneEngine(ctx context.Context, _ *api.Empty) (*api.EngineReply, error) {
	m.dockerMu.Lock()
	defer m.dockerMu.Unlock()
	text, err := m.dockerEngine().PruneBuildCache(ctx)
	return &api.EngineReply{Status: text}, err
}

func (m *Manager) EngineStatus(ctx context.Context, _ *api.Empty) (*api.EngineReply, error) {
	m.dockerMu.Lock()
	defer m.dockerMu.Unlock()
	status, err := m.dockerEngine().Status(ctx)
	return &api.EngineReply{Status: status}, err
}

func (m *Manager) GetEngineInfo(ctx context.Context, _ *api.Empty) (*api.EngineInfo, error) {
	m.dockerMu.Lock()
	defer m.dockerMu.Unlock()
	info, err := m.dockerEngine().Info(ctx)
	if err != nil {
		return nil, err
	}
	return &api.EngineInfo{
		Mode: info.Mode, State: info.State, Health: info.Health,
		Image: info.Image, ConfiguredImage: info.ConfiguredImage, Endpoint: info.Endpoint,
		BuildCache: info.BuildCache, Reclaimable: info.Reclaimable, UsageError: info.UsageError,
	}, nil
}

// GetInstallationVersion takes no engine lock: what cxz is does not wait behind
// a rebuild. It used to, because it was answered inside the engine's info.
func (m *Manager) GetInstallationVersion(context.Context, *api.Empty) (*api.InstallationVersion, error) {
	build := cxzupdate.Current()
	out := &api.InstallationVersion{Version: build.Version, Revision: build.Revision}
	channel, err := versionpin.Channel(m.Root)
	out.Channel = channel
	if err != nil {
		out.Error = err.Error()
	}
	pin, err := versionpin.Load(m.Root)
	if err != nil {
		out.Error = err.Error()
	} else if pin.Pinned() {
		out.Pin = pin.Version
	}
	return out, nil
}

// A managed engine's configuration is rendered before it is saved, so a
// Compose override that cannot be merged is refused rather than stored and
// discovered at the next start.
func (m *Manager) renderIfManaged(ctx context.Context, e engine.Engine, spec engine.Spec) error {
	if spec.Mode != "dind" {
		return nil
	}
	_, _, err := e.Render(ctx, spec)
	return err
}

func engineSpec(v *api.EngineSpec) engine.Spec {
	if v == nil {
		return engine.Spec{}
	}
	return engine.Spec{Mode: v.Mode, Image: v.Image, Override: v.Override}
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
