package workspace

import (
	"context"
	"fmt"

	"github.com/lesomnus/cxz/api"
	"google.golang.org/grpc"
)

type historyConnection struct {
	project, container, token string
	conn                      *grpc.ClientConn
	client                    api.SessionsClient
}

// History pages share a transport. Registry changes invalidate it immediately;
// a failed RPC invalidates stale network addresses for the following request.
func (m *Manager) historyClient(ctx context.Context, id string) (*historyConnection, error) {
	projects, err := m.all(ctx)
	if err != nil {
		return nil, err
	}
	var project *Project
	for _, p := range projects {
		for _, session := range p.Sessions {
			if session.Id == id {
				project = p
				break
			}
		}
	}
	if project == nil {
		return nil, fmt.Errorf("session not found: %s", id)
	}
	m.historyMu.Lock()
	defer m.historyMu.Unlock()
	if m.historyClients == nil {
		m.historyClients = map[string]*historyConnection{}
	}
	if old := m.historyClients[project.ID]; old != nil {
		if old.container == project.ContainerID && old.token == project.Token {
			return old, nil
		}
		old.conn.Close()
		delete(m.historyClients, project.ID)
	}
	conn, client, err := m.client(ctx, project)
	if err != nil {
		return nil, err
	}
	c := &historyConnection{project: project.ID, container: project.ContainerID, token: project.Token, conn: conn, client: client}
	m.historyClients[project.ID] = c
	return c, nil
}

func (m *Manager) dropHistoryClient(c *historyConnection) {
	m.historyMu.Lock()
	defer m.historyMu.Unlock()
	if m.historyClients[c.project] == c {
		c.conn.Close()
		delete(m.historyClients, c.project)
	}
}

func (m *Manager) Close() {
	m.historyMu.Lock()
	defer m.historyMu.Unlock()
	for id, c := range m.historyClients {
		c.conn.Close()
		delete(m.historyClients, id)
	}
}

func (m *Manager) Background(ctx context.Context, r *api.SessionRef) (*api.BackgroundReply, error) {
	c, err := m.historyClient(ctx, r.Id)
	if err != nil {
		return nil, err
	}
	v, err := c.client.Background(ctx, r)
	if err != nil && ctx.Err() == nil {
		m.dropHistoryClient(c)
	}
	return v, err
}
