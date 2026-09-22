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
	// client checks live container ownership and carries the project's token.
	conn, _, err := m.client(ctx, p)
	if err != nil {
		return err
	}
	defer conn.Close()
	return resourceclient.New(conn).LoginSession(ctx, p.ID, account, key, input, output)
}
