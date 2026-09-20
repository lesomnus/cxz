package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/dockerx"
	"github.com/lesomnus/cxz/internal/filemap"
)

func (m *Manager) syncFileMappings(ctx context.Context, client api.SessionsClient) error {
	m.filesMu.Lock()
	defer m.filesMu.Unlock()
	b, exists, err := filemap.Load(m.Root)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	data, err := json.Marshal(b)
	if err != nil {
		return err
	}
	_, err = client.FileMappings(ctx, &api.FileMappingsInput{Bundle: data})
	return err
}
func (m *Manager) FileMappings(ctx context.Context, b filemap.Bundle) (*api.Receipt, error) {
	m.filesMu.Lock()
	err := filemap.Save(m.Root, b)
	m.filesMu.Unlock()
	if err != nil {
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
		if err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", p.Name, err))
			continue
		}
		if !v.State.Running {
			continue
		}
		conn, client, err := m.client(ctx, p)
		if err == nil {
			err = m.syncFileMappings(ctx, client)
			conn.Close()
		}
		if err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", p.Name, err))
		}
	}
	if err := errors.Join(failures...); err != nil {
		return nil, fmt.Errorf("saved on manager; some projects could not receive files (retry config files sync): %w", err)
	}
	return &api.Receipt{Status: "saved; applies on next agent start; stopped projects receive files when opened"}, nil
}
