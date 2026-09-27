package server

import (
	"context"
	"fmt"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/cxzupdate"
	"github.com/lesomnus/cxz/internal/supervisor"
)

func (s *Server) quietForUpdate(ctx context.Context) (map[string]string, error) {
	if s.manager != nil {
		return s.manager.QuietForUpdate(ctx)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	sessions, e := s.list(ctx)
	if e != nil {
		return nil, e
	}
	runs := map[string]string{}
	var blocked error
	for _, m := range sessions {
		snap, e := s.snapshot(ctx, m)
		if e != nil {
			return runs, e
		}
		if snap.State == "stopped" || snap.State == "failed" {
			continue
		}
		runs[m.ID] = snap.RunId
		var p supervisor.UpdateStatus
		if e = supervisor.Call(ctx, s.root, m.ID, "update-status", core.Command{RunID: snap.RunId}, &p); e != nil {
			return runs, e
		}
		if !p.Ready || p.Protocol != cxzupdate.Protocol {
			blocked = fmt.Errorf("session %s: %s (protocol %d)", m.ID, p.Reason, p.Protocol)
		}
	}
	return runs, blocked
}
