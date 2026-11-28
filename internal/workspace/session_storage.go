package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/accounts"
	"github.com/lesomnus/cxz/internal/assets"
	"github.com/lesomnus/cxz/internal/core"
)

// Project state lives in the Manager's own volume, so the sizes can be read
// here without opening the project. Where it does not -- a pod, or an
// installation that still keeps state in project volumes -- the runtime is
// asked instead, since it is the one that can see the files.
func (m *Manager) SessionStorage(ctx context.Context, r *api.SessionRef) (*api.StorageUsage, error) {
	if m.kube != nil {
		conn, c, err := m.kubeSession(ctx, r.Id)
		if err != nil {
			return nil, err
		}
		defer conn.Close()
		return c.SessionStorage(ctx, r)
	}
	p, err := m.sessionProject(ctx, r.Id)
	if err != nil {
		return nil, err
	}
	if m.StateVolume == "" {
		conn, c, err := m.client(ctx, p)
		if err != nil {
			return nil, err
		}
		defer conn.Close()
		return c.SessionStorage(ctx, r)
	}
	dir, _ := m.projectState(p.storageID())
	return sessionStorage(ctx, filepath.Join(dir, "data"), r.Id)
}

// The manifest is read from the session's own directory rather than from the
// registry: the profile is keyed by the creation id, and a session the registry
// knows about may never have written one.
func sessionStorage(ctx context.Context, root, id string) (*api.StorageUsage, error) {
	if !assets.ValidID(id) {
		return nil, fmt.Errorf("invalid session id")
	}
	var manifest core.Session
	if raw, err := os.ReadFile(filepath.Join(core.Dir(root, id), "session.json")); err == nil {
		if err = json.Unmarshal(raw, &manifest); err != nil {
			return nil, err
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	manifest.ID = id
	return MeasureSessionStorage(ctx, root, manifest)
}

// MeasureSessionStorage is also how a project runtime answers for itself, where
// the Manager cannot see the files.
func MeasureSessionStorage(ctx context.Context, root string, session core.Session) (*api.StorageUsage, error) {
	out := &api.StorageUsage{}
	var err error
	if out.JournalBytes, err = core.TreeBytes(ctx, core.Dir(root, session.ID)); err != nil {
		return nil, err
	}
	if session.CreateID != "" {
		profile := accounts.SessionRoot(root, session.CreateID)
		total, err := core.TreeBytes(ctx, profile)
		if err != nil {
			return nil, err
		}
		if session.Account != "" {
			folder := "projects"
			if session.Kind == "codex" {
				folder = "sessions"
			}
			if out.ProviderBytes, err = core.TreeBytes(ctx, filepath.Join(accounts.Config(profile, session.Account), folder)); err != nil {
				return nil, err
			}
		}
		out.ProfileBytes = max(0, total-out.ProviderBytes)
	}
	out.TotalBytes = out.JournalBytes + out.ProviderBytes + out.ProfileBytes
	return out, nil
}
