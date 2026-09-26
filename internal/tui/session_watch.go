package tui

import (
	"context"
	"time"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/core"
)

type sessionWatch struct {
	ctx        context.Context
	cancel     context.CancelFunc
	epoch      uint64
	active     bool
	retryAfter time.Time
}

type sessionWatchEnded struct {
	id    string
	epoch uint64
	err   error
}

func hasConversationEvents(events []*api.Event) bool {
	for _, event := range events {
		switch event.Kind {
		case "input", "assistant", "tool_call", "tool_result", "approval":
			return true
		}
	}
	return false
}

func (m *model) validSessionWatch(id string, epoch uint64) bool {
	if epoch == 0 {
		return true
	}
	if m.sessionWatches == nil {
		return epoch == m.watchEpoch
	}
	w := m.sessionWatches[id]
	return w != nil && w.epoch == epoch
}

// Selection changes only which cached conversation is rendered. Subscriptions
// live as long as the listed session, including while browsing project screens.
func (m *model) watch() {
	if m.program == nil || m.client == nil || m.ctx == nil || m.ctx.Err() != nil {
		return
	}
	if m.activityID == "" {
		m.activityID = core.ID()
	}
	if m.sessionWatches == nil {
		m.sessionWatches = map[string]*sessionWatch{}
		m.watchBootstrap = make(chan struct{}, 4)
		m.watchCancel = func() {
			for _, w := range m.sessionWatches {
				w.cancel()
			}
		}
	}
	sessions := m.allSessions
	if sessions == nil {
		sessions = m.sessions
	}
	live := map[string]bool{}
	// Prioritize the open conversation's first history fetch.
	ordered := append([]*api.Session(nil), sessions...)
	if s := m.current(); s != nil {
		ordered = append([]*api.Session{s}, ordered...)
	}
	for _, s := range ordered {
		if live[s.Id] {
			continue
		}
		live[s.Id] = true
		w := m.sessionWatches[s.Id]
		if w == nil || !w.active && !time.Now().Before(w.retryAfter) {
			m.startSessionWatch(s)
		}
	}
	for id, w := range m.sessionWatches {
		if !live[id] {
			w.cancel()
			delete(m.sessionWatches, id)
		}
	}
	if s := m.current(); s != nil && !m.projectView {
		if w := m.sessionWatches[s.Id]; w != nil {
			m.watchID, m.watchEpoch, m.watchContext = s.Id, w.epoch, w.ctx
		}
	}
}
