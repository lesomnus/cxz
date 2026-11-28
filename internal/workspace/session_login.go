package workspace

import (
	"context"

	"github.com/lesomnus/cxz/internal/accounts"
	"github.com/lesomnus/cxz/internal/resourceclient"
)

func (m *Manager) LoginSession(ctx context.Context, r accounts.SessionLogin) (accounts.SessionLoginResult, error) {
	var out accounts.SessionLoginResult
	p, err := m.resolve(ctx, r.Project)
	if err != nil {
		return out, err
	}
	lock := m.projectLock(p.ID)
	lock.Lock()
	locked := true
	defer func() {
		if locked {
			lock.Unlock()
		}
	}()
	p, err = m.resolve(ctx, p.ID)
	if err != nil {
		return out, err
	}
	// client checks live container ownership and carries the project's token.
	conn, _, err := m.client(ctx, p)
	if err != nil {
		return out, err
	}
	defer conn.Close()
	if err = m.prepareAgent(ctx, p, "claude"); err != nil {
		return out, err
	}
	// Interactive login may wait for a person; preparation is serialized, the
	// conversation with that person must not lock every other project action.
	lock.Unlock()
	locked = false
	r.Project = p.ID
	return resourceclient.New(conn).LoginSession(ctx, r)
}
