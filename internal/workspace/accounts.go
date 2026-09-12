package workspace

import (
	"context"
	"github.com/lesomnus/cxz/api"
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
			return client.Resume(ctx, r)
		}
	}
	return nil, status.Error(codes.NotFound, "session not found")
}
func liveSession(s *api.Session) bool {
	return s.State == "starting" || s.State == "idle" || s.State == "working" || s.State == "waiting_input"
}
