package workspace

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/accounts"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"os/exec"
)

func (m *Manager) connectAccount(ctx context.Context, p *Project, account, backend string) error {
	if backend != accounts.BrokeredAccessToken {
		return nil
	}
	grant, err := accounts.IssueGrant(m.Root, p.ID, account)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(grant)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "docker", "exec", "-i", "--user", p.RemoteUser, p.ContainerID, "/cxz/tools/cxz", "--state", "/cxz/state/data", "_account-bind")
	cmd.Stdin = bytes.NewReader(raw)
	if err = cmd.Run(); err != nil {
		return fmt.Errorf("could not connect central account to project")
	}
	return nil
}

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
			if err = m.connectAccount(ctx, p, v.Account, v.AuthBackend); err != nil {
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
