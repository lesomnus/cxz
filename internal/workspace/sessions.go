package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/lesomnus/cxz/api"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// How long one project runtime has to list its sessions in a sweep. Projects
// are asked at once, so this is also about how long a sweep can take.
var sessionSweepTimeout = 5 * time.Second

// How long a running but unresponsive runtime keeps its sessions' last known
// states. A loaded host makes a runtime miss a deadline for a while; that is
// not its sessions stopping, and reporting it as such hides them from any view
// that leaves out stopped sessions. A runtime silent for longer than this is
// treated as gone.
var slowRuntimeGrace = 5 * time.Minute

type sweepAnswer struct {
	project *Project
	// slow: the container runs but its runtime missed the deadline.
	slow bool
	err  error
}

// Sessions asks every project runtime for its sessions, all at once.
func (m *Manager) Sessions(ctx context.Context) (*api.SessionList, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	all, e := m.all(ctx)
	if e != nil {
		return nil, e
	}
	answers := make([]sweepAnswer, len(all))
	var wg sync.WaitGroup
	for i, p := range all {
		wg.Add(1)
		go func() {
			defer wg.Done()
			answers[i] = m.sweepProject(ctx, p)
		}()
	}
	wg.Wait()
	if m.slowSince == nil {
		m.slowSince = map[string]time.Time{}
	}
	now := time.Now()
	out := &api.SessionList{}
	for _, a := range answers {
		if a.err != nil {
			return nil, a.err
		}
		p := a.project
		if !a.slow {
			delete(m.slowSince, p.ID)
		} else {
			since, seen := m.slowSince[p.ID]
			if !seen {
				since = now
				m.slowSince[p.ID] = now
			}
			if now.Sub(since) >= slowRuntimeGrace {
				interrupt(p)
			}
		}
		out.Sessions = append(out.Sessions, p.Sessions...)
	}
	if c, err := m.auxiliaryController(); err == nil {
		for _, v := range out.Sessions {
			if title, e := c.Title(v.Id); e == nil && title.Text != "" {
				v.Title = title.Text
			}
		}
	}
	return out, nil
}

// sweepProject refreshes one project's sessions from its runtime. A project
// another operation holds answers with what it last recorded.
func (m *Manager) sweepProject(ctx context.Context, p *Project) sweepAnswer {
	lock := m.projectLock(p.ID)
	if !lock.TryLock() {
		return sweepAnswer{project: p}
	}
	defer lock.Unlock()
	latest, err := m.resolve(ctx, p.ID)
	if err != nil {
		return sweepAnswer{err: err}
	}
	p = latest
	dial := m.client
	if m.sweepClient != nil {
		dial = m.sweepClient
	}
	c, client, e := dial(ctx, p)
	var list *api.SessionList
	if e == nil {
		q, cancel := context.WithTimeout(ctx, sessionSweepTimeout)
		list, e = client.List(q, &api.Empty{})
		cancel()
		if c != nil {
			c.Close()
		}
		// client found the container running, so a deadline here is a runtime
		// that is slow, not one that is gone.
		if e != nil && ctx.Err() == nil && (errors.Is(e, context.DeadlineExceeded) || status.Code(e) == codes.DeadlineExceeded) {
			fmt.Fprintf(os.Stderr, "project %s: runtime did not list its sessions within %s; keeping their last known states\n", p.ID, sessionSweepTimeout)
			return sweepAnswer{project: p, slow: true}
		}
	}
	if e != nil {
		interrupt(p)
		return sweepAnswer{project: p}
	}
	for _, s := range list.Sessions {
		s.ProjectId = p.ID
	}
	before, _ := json.Marshal(p.Sessions)
	after, _ := json.Marshal(list.Sessions)
	p.Sessions = list.Sessions
	// The answer is also the proof that whatever the last attempt
	// recorded as broken is not broken now.
	if p.settle() || string(before) != string(after) {
		if e = m.save(ctx, p); e != nil {
			return sweepAnswer{err: e}
		}
	}
	return sweepAnswer{project: p}
}

// interrupt reports what an unreachable runtime's sessions are: not running.
func interrupt(p *Project) {
	for _, s := range p.Sessions {
		if s.State != "stopped" && s.State != "failed" {
			s.State = "interrupted"
		}
		s.Pending = nil
	}
}
