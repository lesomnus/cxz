package workspace

import (
	"context"
	"io"

	"github.com/lesomnus/cxz/internal/resourceclient"
)

func (m *Manager) LoginSession(ctx context.Context, project, account, key string, input io.ReadCloser, output io.Writer) error {
	p, err := m.resolve(ctx, project)
	if err != nil {
		return err
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
		return err
	}
	// client checks live container ownership and carries the project's token.
	conn, _, err := m.client(ctx, p)
	if err != nil {
		return err
	}
	defer conn.Close()
	if err = m.prepareAgent(ctx, p, "claude"); err != nil {
		return err
	}
	// Interactive login may wait for a person; preparation is serialized, the
	// conversation with that person must not lock every other project action.
	lock.Unlock()
	locked = false
	return resourceclient.New(conn).LoginSession(ctx, p.ID, account, key, input, output)
}
