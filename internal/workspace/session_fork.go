package workspace

import (
	"context"

	"github.com/lesomnus/cxz/api"
)

func (m *Manager) ForkSession(ctx context.Context, r *api.SessionForkInput) (*api.Session, error) {
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
	source, err := client.Get(ctx, &api.SessionRef{Id: r.SessionId})
	if err != nil {
		return nil, err
	}
	if err = m.connectAccount(ctx, p, source.Account, source.AuthBackend); err != nil {
		return nil, err
	}
	if err = m.syncRuntimePreferences(ctx, client, p.ID); err != nil {
		return nil, err
	}
	child, err := client.ForkSession(ctx, r)
	if err != nil {
		return nil, err
	}
	auxiliary, err := m.auxiliaryController()
	if err != nil {
		return nil, err
	}
	if err = auxiliary.InheritSession(source.Id, child.Id); err != nil {
		return nil, err
	}
	if title, err := auxiliary.Title(child.Id); err != nil {
		return nil, err
	} else if title.Text != "" {
		child.Title = title.Text
	} else if child.Title != "" {
		if err = auxiliary.SetTitle(child.Id, child.Title); err != nil {
			return nil, err
		}
	}
	list, err := client.List(ctx, &api.Empty{})
	if err != nil {
		return nil, err
	}
	p.Sessions = list.Sessions
	for _, item := range p.Sessions {
		item.ProjectId = p.ID
	}
	child.ProjectId = p.ID
	return child, m.save(ctx, p)
}
