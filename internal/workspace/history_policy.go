package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/dockerx"
	"github.com/lesomnus/cxz/internal/historypolicy"
)

func (m *Manager) syncHistoryPolicy(ctx context.Context, client api.SessionsClient) error {
	if _, err := os.Stat(filepath.Join(m.Root, "history-policy.json")); os.IsNotExist(err) {
		return nil
	}
	p, err := historypolicy.Load(m.Root)
	if err != nil {
		return err
	}
	b, _ := json.Marshal(p)
	_, err = client.Docker(ctx, &api.DockerInput{Action: "history-policy", Spec: b})
	return err
}
func (m *Manager) historyPolicy(ctx context.Context, spec []byte) (*api.Receipt, error) {
	if len(spec) > 0 {
		var p historypolicy.Policy
		if err := json.Unmarshal(spec, &p); err != nil {
			return nil, err
		}
		if err := historypolicy.Save(m.Root, p); err != nil {
			return nil, err
		}
		projects, err := m.all(ctx)
		if err != nil {
			return nil, err
		}
		var failures []error
		for _, p := range projects {
			if p.ContainerID == "" {
				continue
			}
			v, err := dockerx.Owned(ctx, p.ContainerID, m.Owner, p.ID)
			if err == nil && !v.State.Running {
				continue
			}
			if err == nil {
				conn, client, e := m.client(ctx, p)
				err = e
				if err == nil {
					err = m.syncHistoryPolicy(ctx, client)
					conn.Close()
				}
			}
			if err != nil {
				failures = append(failures, fmt.Errorf("%s: %w", p.Name, err))
			}
		}
		if err := errors.Join(failures...); err != nil {
			return nil, fmt.Errorf("history policy saved; some projects require a retry: %w", err)
		}
	}
	p, err := historypolicy.Load(m.Root)
	if err != nil {
		return nil, err
	}
	b, err := json.Marshal(p)
	return &api.Receipt{Status: string(b)}, err
}

func (m *Manager) syncRuntimePreferences(ctx context.Context, client api.SessionsClient, project string) error {
	if err := m.syncFileMappings(ctx, client); err != nil {
		return err
	}
	if err := m.syncHistoryPolicy(ctx, client); err != nil {
		return err
	}
	return m.syncMCP(ctx, client, project)
}
