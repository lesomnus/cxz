package workspace

import (
	"bytes"
	"context"
	"fmt"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/accounts"
	"github.com/lesomnus/cxz/internal/dockerx"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (m *Manager) ResumeSession(ctx context.Context, r *api.Control) (*api.Session, error) {
	all, err := m.all(ctx)
	if err != nil {
		return nil, err
	}
	for _, p := range all {
		for _, s := range p.Sessions {
			if s.Id != r.SessionId {
				continue
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
			v, err := client.Get(ctx, &api.SessionRef{Id: r.SessionId})
			if err != nil {
				return nil, err
			}
			if liveSession(v) {
				return v, nil
			}
			if v.RunId != r.RunId {
				return nil, status.Error(codes.FailedPrecondition, "stale run_id")
			}
			list, err := client.List(ctx, &api.Empty{})
			if err != nil {
				return nil, err
			}
			for _, other := range list.Sessions {
				if liveSession(other) {
					return nil, status.Error(codes.AlreadyExists, "workspace has another live session")
				}
			}
			if err = m.prepareAccount(ctx, p, v.Account, v.Agent); err != nil {
				return nil, err
			}
			return client.Resume(ctx, r)
		}
	}
	return nil, status.Error(codes.NotFound, "session not found")
}
func liveSession(s *api.Session) bool {
	return s.State == "starting" || s.State == "idle" || s.State == "working" || s.State == "waiting_input"
}
func (m *Manager) prepareAccount(ctx context.Context, p *Project, alias, agent string) error {
	if alias == "" {
		return fmt.Errorf("session has no account; create a new session with --account")
	}
	credential, err := accounts.Credential(m.Root, alias, agent)
	if err != nil {
		return err
	}
	if _, err = dockerx.Owned(ctx, p.ContainerID, m.Owner, p.ID); err != nil {
		return err
	}
	// No credential appears in command arguments, resource audit, or journals.
	return dockerx.Input(ctx, bytes.NewReader(credential), "exec", "-i", "--user", p.RemoteUser, p.ContainerID, "/cxz/tools/cxz", "--state", "/cxz/state/data", "_account-import", alias, agent)
}
