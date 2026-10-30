package server

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/cxzupdate"
	"github.com/lesomnus/cxz/internal/supervisor"
)

// quietSweep bounds a readiness sweep. The caller that asks for one -- the
// update helper over the maintenance socket -- gives up after 30 seconds, so a
// sweep that cannot answer inside this says which session it was waiting for
// instead of letting the caller time out with nothing.
const quietSweep = 10 * time.Second

// quietSweepProbes is how many sessions are asked at once. The answer comes
// from each session's supervisor over its own socket, so these overlap; the
// bound is there because every probe also reads that session's projection, and
// this server holds one database connection.
const quietSweepProbes = 6

func (s *Server) quietForUpdate(ctx context.Context) (map[string]string, error) {
	if s.manager != nil {
		return s.manager.QuietForUpdate(ctx)
	}
	s.mu.Lock()
	sessions, e := s.list(ctx)
	s.mu.Unlock()
	if e != nil {
		return nil, e
	}
	ctx, cancel := context.WithTimeout(ctx, quietSweep)
	defer cancel()
	type answer struct {
		id, run string
		blocked error
	}
	out := make([]answer, len(sessions))
	gate := make(chan struct{}, quietSweepProbes)
	var wait sync.WaitGroup
	for i, m := range sessions {
		wait.Add(1)
		go func(i int, m core.Session) {
			defer wait.Done()
			gate <- struct{}{}
			defer func() { <-gate }()
			if ctx.Err() != nil {
				out[i] = answer{id: m.ID, blocked: fmt.Errorf("session %s: not asked before the sweep ran out of time", m.ID)}
				return
			}
			out[i] = s.sessionQuiet(ctx, m)
		}(i, m)
	}
	wait.Wait()
	runs := map[string]string{}
	var blocked error
	for _, a := range out {
		if a.run != "" {
			runs[a.id] = a.run
		}
		if a.blocked != nil && blocked == nil {
			blocked = a.blocked
		}
	}
	return runs, blocked
}

// sessionQuiet asks one session whether an update may take it. A stopped
// session has nothing to ask and nothing to wait for.
func (s *Server) sessionQuiet(ctx context.Context, m core.Session) (a struct {
	id, run string
	blocked error
}) {
	a.id = m.ID
	snap, e := s.snapshot(ctx, m)
	if e != nil {
		a.blocked = fmt.Errorf("session %s: %w", m.ID, e)
		return a
	}
	if snap.State == "stopped" || snap.State == "failed" {
		return a
	}
	a.run = snap.RunId
	var p supervisor.UpdateStatus
	if e = supervisor.Call(ctx, s.root, m.ID, "update-status", core.Command{RunID: snap.RunId}, &p); e != nil {
		// Nothing listening means no agent is running, which is what quiet is.
		// snapshot already calls that "interrupted" for the same reason, and
		// counting it as busy let one dead session block every update of the
		// installation -- with no way to stop it, because stopping goes through
		// this socket too. A supervisor on its way up is the exception: it
		// says "starting", and failing closed there is still right.
		// See cxz#223.
		if supervisor.Unreachable(e) && snap.State != "starting" {
			return a
		}
		a.blocked = fmt.Errorf("session %s: %w", m.ID, e)
		return a
	}
	if !p.Ready || p.Protocol != cxzupdate.Protocol {
		reason := fmt.Sprintf("session %s: %s (protocol %d)", m.ID, p.Reason, p.Protocol)
		if p.Protocol != cxzupdate.Protocol {
			a.blocked = cxzupdate.RecoveryBlocked("protocol_mismatch", reason)
		} else {
			a.blocked = cxzupdate.RecoveryWaiting("not_idle", reason)
		}
	}
	return a
}
