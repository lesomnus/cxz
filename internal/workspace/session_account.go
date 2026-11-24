package workspace

import (
	"context"

	"github.com/lesomnus/cxz/api"
)

func (m *Manager) SwitchSessionAccount(ctx context.Context, r *api.SessionAccountInput) (*api.Session, error) {
	p, err := m.projectOf(ctx, r.SessionId)
	if err != nil {
		return nil, err
	}
	lock := m.projectLock(p.ID)
	lock.Lock()
	defer lock.Unlock()
	p, err = m.resolve(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	conn, client, err := m.client(ctx, p)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	out, err := client.SwitchSessionAccount(ctx, r)
	if err != nil {
		return nil, err
	}
	list, err := client.List(ctx, &api.Empty{})
	if err != nil {
		return nil, err
	}
	p.Sessions = list.Sessions
	for _, item := range p.Sessions {
		item.ProjectId = p.ID
	}
	out.ProjectId = p.ID
	return out, m.save(ctx, p)
}
