package server

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/distribution"
	"github.com/lesomnus/cxz/internal/supervisor"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *Server) Activity(ctx context.Context, r *api.ActivityInput) (*api.Receipt, error) {
	if s.manager != nil {
		conn, c, e := s.manager.ClientFor(ctx, r.SessionId)
		if e != nil {
			return nil, e
		}
		defer conn.Close()
		return c.Activity(ctx, r)
	}
	if _, e := s.manifest(ctx, r.SessionId); e != nil {
		return nil, e
	}
	var out core.Receipt
	e := supervisor.Call(ctx, s.root, r.SessionId, "activity", core.Command{RunID: r.RunId, ClientID: r.ClientId, Busy: r.Busy}, &out)
	return &api.Receipt{ClientId: r.ClientId, Status: out.Status}, e
}

func validAgentUpdate(kind, path string) bool {
	parts := strings.Split(strings.TrimPrefix(path, "/cxz/tools/"), "/")
	return strings.HasPrefix(path, "/cxz/tools/") && filepath.Clean(path) == path && len(parts) >= 4 && parts[0] == kind && distribution.ValidVersion(parts[1]) && filepath.Base(path) == kind
}

func (s *Server) updateNotice(ctx context.Context, id, run, text string) {
	var receipt core.Receipt
	_ = supervisor.Call(ctx, s.root, id, "update-notice", core.Command{RunID: run, Text: text}, &receipt)
}

func (s *Server) saveAgentManifest(ctx context.Context, m core.Session) error {
	if err := core.WriteJSON(filepath.Join(core.Dir(s.root, m.ID), "session.json"), m); err != nil {
		return err
	}
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, "UPDATE sessions SET manifest=? WHERE id=?", b, m.ID)
	return err
}

type agentUpdateTransaction struct {
	Old        core.Session
	Target     string
	RecoveryAt time.Time
}

func initializedAgent(v *api.Session) bool {
	return v != nil && (v.State == "idle" || v.State == "working" || v.State == "waiting_input")
}

func (s *Server) UpdateAgent(ctx context.Context, r *api.AgentUpdateInput) (*api.AgentUpdateStatus, error) {
	if s.manager != nil {
		conn, c, e := s.manager.ClientFor(ctx, r.SessionId)
		if e != nil {
			return nil, e
		}
		defer conn.Close()
		return c.UpdateAgent(ctx, r)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	m, err := s.manifest(ctx, r.SessionId)
	if err != nil {
		return nil, err
	}
	var probe supervisor.UpdateStatus
	if err = supervisor.Call(ctx, s.root, m.ID, "update-status", core.Command{RunID: r.RunId}, &probe); err != nil {
		return nil, status.Error(codes.FailedPrecondition, "update readiness unavailable; runtime/supervisor may need updating")
	}
	out := &api.AgentUpdateStatus{Ready: probe.Ready, Reason: probe.Reason, Binary: probe.Binary, State: probe.State}
	if r.Binary != "" && r.Binary != probe.Binary && validAgentUpdate(m.Kind, r.Binary) {
		if s.updateQueued == nil {
			s.updateQueued = map[string]string{}
		}
		if s.updateQueued[m.ID] != r.Binary {
			s.updateQueued[m.ID] = r.Binary
			s.updateNotice(ctx, m.ID, r.RunId, "agent update queued · "+m.Kind+" "+strings.Split(strings.TrimPrefix(r.Binary, "/cxz/tools/"), "/")[1])
		}
	}
	if !r.Apply || r.Binary == m.Agent {
		return out, nil
	}
	if !out.Ready {
		return out, nil
	}
	if !validAgentUpdate(m.Kind, r.Binary) {
		return nil, status.Error(codes.InvalidArgument, "binary must be an immutable cxz tools release for this provider")
	}
	if st, e := os.Stat(r.Binary); e != nil || !st.Mode().IsRegular() || st.Mode()&0111 == 0 {
		return nil, status.Error(codes.InvalidArgument, "release binary unavailable")
	}
	transaction := filepath.Join(core.Dir(s.root, m.ID), "agent-update.json")
	if err = core.WriteJSON(transaction, agentUpdateTransaction{Old: m, Target: r.Binary}); err != nil {
		return nil, err
	}
	var receipt core.Receipt
	// This stop checks readiness and fences Send under the SAME supervisor lock.
	if err = supervisor.Call(ctx, s.root, m.ID, "update-stop", core.Command{RunID: r.RunId, ClientID: core.ID()}, &receipt); err != nil {
		if !strings.HasPrefix(err.Error(), "supervisor 409:") {
			return nil, fmt.Errorf("update stop acknowledgement lost; durable recovery will inspect the session, no command retried")
		}
		_ = os.Remove(transaction)
		out.Ready = false
		out.Reason = "idle conditions changed before restart"
		return out, nil
	}
	// Finish or roll back even if the manager's connection disappears.
	return finishAgentUpdate(ctx, m, r.Binary, transaction, agentUpdateActions{
		release: s.awaitSessionRelease,
		save:    s.saveAgentManifest,
		launch:  s.launch,
		notice:  s.updateNotice,
		stop: func(ctx context.Context, id string) {
			var current core.Snapshot
			if supervisor.Call(ctx, s.root, id, "status", nil, &current) == nil {
				_, _ = s.commandUnlocked(ctx, id, "stop", core.Command{RunID: current.RunID, ClientID: core.ID()})
			}
		},
	})
}

type agentUpdateActions struct {
	release func(context.Context, core.Session) error
	save    func(context.Context, core.Session) error
	launch  func(context.Context, core.Session) (*api.Session, error)
	stop    func(context.Context, string)
	notice  func(context.Context, string, string, string)
}

// Called with the runtime command fence held and a durable stop intent saved.
func finishAgentUpdate(ctx context.Context, m core.Session, target, transaction string, a agentUpdateActions) (*api.AgentUpdateStatus, error) {
	work, cancel := context.WithTimeout(context.WithoutCancel(ctx), 90*time.Second)
	defer cancel()
	old := m
	err := a.release(work, m)
	if err == nil {
		m.Agent = target
		err = a.save(work, m)
	}
	var updated *api.Session
	if err == nil {
		updated, err = a.launch(work, m)
		if err == nil && !initializedAgent(updated) {
			err = fmt.Errorf("agent initialization failed")
		}
	}
	if err == nil {
		_ = os.Remove(transaction)
		a.notice(work, m.ID, updated.RunId, "agent update completed")
		return &api.AgentUpdateStatus{Ready: true, Binary: m.Agent, State: "updated"}, nil
	}
	// Never kill a new run that has already accepted user work on an ambiguous
	// startup result. Commands are fenced by s.mu until this function returns.
	rollback, rollbackCancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer rollbackCancel()
	a.stop(rollback, m.ID)
	if e := a.release(rollback, old); e != nil {
		return nil, fmt.Errorf("agent update failed; rollback waiting for process release")
	}
	if e := a.save(rollback, old); e != nil {
		return nil, fmt.Errorf("agent update failed; rollback manifest could not be restored")
	}
	restored, e := a.launch(rollback, old)
	if e != nil || !initializedAgent(restored) {
		_ = core.WriteJSON(transaction, agentUpdateTransaction{Old: old, Target: target, RecoveryAt: time.Now()})
		return nil, fmt.Errorf("agent update failed; old version restored but restart failed")
	}
	_ = os.Remove(transaction)
	a.notice(rollback, old.ID, restored.RunId, "agent update failed; previous version restored")
	return &api.AgentUpdateStatus{Binary: old.Agent, State: "rolled_back", Reason: "new agent failed initialization"}, nil
}

func (s *Server) runUpdateRecovery(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.mu.Lock()
			s.recoverAgentUpdates(ctx)
			s.mu.Unlock()
		}
	}
}

// A runtime crash never turns a half-written release pointer into an implicit
// user-message retry. Restore the old executable if no agent survived; a live
// initialized replacement is left alone, including any work it has accepted.
func (s *Server) recoverAgentUpdates(ctx context.Context) {
	ms, err := s.list(ctx)
	if err != nil {
		return
	}
	for _, m := range ms {
		path := filepath.Join(core.Dir(s.root, m.ID), "agent-update.json")
		b, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var tx agentUpdateTransaction
		if json.Unmarshal(b, &tx) != nil || tx.Old.ID != m.ID || tx.Old.CreateID != m.CreateID || !validAgentUpdate(m.Kind, tx.Target) {
			continue
		}
		if !tx.RecoveryAt.IsZero() && time.Since(tx.RecoveryAt) < 24*time.Hour {
			continue
		}
		work, cancel := context.WithTimeout(ctx, 60*time.Second)
		var snap core.Snapshot
		if supervisor.Call(work, s.root, m.ID, "status", nil, &snap) == nil && (snap.State == "idle" || snap.State == "working" || snap.State == "waiting_input" || snap.State == "starting") {
			if snap.State != "starting" {
				_ = os.Remove(path)
			}
			cancel()
			continue
		}
		tx.RecoveryAt = time.Now()
		if core.WriteJSON(path, tx) != nil {
			cancel()
			continue
		}
		if s.awaitSessionRelease(work, m) == nil && s.saveAgentManifest(work, tx.Old) == nil {
			if v, e := s.launch(work, tx.Old); e == nil && initializedAgent(v) {
				_ = os.Remove(path)
				s.updateNotice(work, m.ID, v.RunId, "interrupted update recovered · previous agent restored")
			}
		}
		cancel()
	}
}
