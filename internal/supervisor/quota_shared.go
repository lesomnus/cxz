package supervisor

import (
	"context"
	"encoding/json"
	"time"

	"github.com/lesomnus/cxz/internal/agentview"
	"github.com/lesomnus/cxz/internal/quotashare"
)

func (s *Supervisor) exchangeQuota(lease string, payload json.RawMessage) (quotashare.Response, error) {
	r := quotashare.Request{Project: s.session.ProjectID, Session: s.session.ID, Account: s.session.Account, Lease: lease, Payload: payload}
	if s.quotaToken != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		socket := s.quotaSocket
		if socket == "" {
			socket = quotashare.Socket
		}
		return quotashare.Remote(ctx, socket, s.quotaToken, r)
	}
	return quotashare.Exchange(s.quotaRoot, r, time.Now())
}
func (s *Supervisor) claimClaudeQuota() bool {
	if s.quotaDisabled {
		return false
	}
	if s.quotaRoot == "" || s.session.Account == "" {
		return true
	}
	// Let requestQuota mark an outstanding provider request as timed out.
	if !s.quotaRequested.IsZero() && time.Since(s.quotaRequested) < time.Minute {
		return false
	}
	out, err := s.exchangeQuota("", nil)
	if err != nil {
		// An older manager may not provide the coordinator yet.
		return time.Since(s.quotaFallbackAt) >= time.Minute
	}
	if out.Snapshot.Observed > s.quotaObserved && len(out.Snapshot.Payload) > 0 {
		s.recordSharedQuota(out.Snapshot.Payload, out.Snapshot.Observed)
	}
	s.quotaLease = out.Snapshot.Lease
	return out.Poll
}
func (s *Supervisor) recordSharedQuota(raw json.RawMessage, observed int64) {
	var data map[string]json.RawMessage
	if json.Unmarshal(raw, &data) != nil || data == nil {
		return
	}
	data["_cxz_observed_ms"], _ = json.Marshal(observed)
	s.quotaObserved = observed
	s.event("usage", "get_usage", "", data, nil)
}
func (s *Supervisor) publishClaudeQuota(raw json.RawMessage) {
	observed := time.Now().UnixMilli()
	if s.quotaLease != "" && len(agentview.Quota("claude", "get_usage", raw)) > 0 {
		if s.quotaToken != "" {
			lease := s.quotaLease
			payload := append(json.RawMessage(nil), raw...)
			s.quotaWorkers.Go(func() { _, _ = s.exchangeQuota(lease, payload) })
			s.quotaLease = ""
			s.recordSharedQuota(raw, observed)
			return
		}
		if out, err := s.exchangeQuota(s.quotaLease, raw); err == nil && out.Snapshot.Observed > 0 {
			s.recordSharedQuota(out.Snapshot.Payload, out.Snapshot.Observed)
			s.quotaLease = ""
			return
		}
	}
	s.quotaLease = ""
	s.recordSharedQuota(raw, observed)
}

// Network coordination never holds up the provider's stdout reader.
func (s *Supervisor) readRemoteClaudeQuota() {
	if s.quotaChecking || s.quotaDisabled || (!s.quotaRequested.IsZero() && time.Since(s.quotaRequested) < time.Minute) {
		return
	}
	s.quotaChecking = true
	s.quotaWorkers.Go(func() {
		out, err := s.exchangeQuota("", nil)
		s.mu.Lock()
		defer s.mu.Unlock()
		s.quotaChecking = false
		select {
		case <-s.done:
			return
		default:
		}
		if s.snap.State != "idle" && s.snap.State != "working" && s.snap.State != "waiting_input" {
			return
		}
		if err != nil {
			if time.Since(s.quotaFallbackAt) >= time.Minute {
				s.sendClaudeQuota()
			}
			return
		}
		if out.Snapshot.Observed > s.quotaObserved && len(out.Snapshot.Payload) > 0 {
			s.recordSharedQuota(out.Snapshot.Payload, out.Snapshot.Observed)
		}
		s.quotaLease = out.Snapshot.Lease
		if out.Poll {
			s.sendClaudeQuota()
		}
	})
}
