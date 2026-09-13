package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/core"
)

type restartConfirmation struct {
	id, run string
	expires time.Time
}
type restartFinished struct{ err error }

func (m *model) restartCommand(text string) tea.Cmd {
	if m.restartBusy {
		m.notice = "Agent restart already in progress."
		return nil
	}
	s := m.current()
	if s == nil {
		return nil
	}
	f := strings.Fields(text)
	if len(f) == 1 {
		m.restartConfirm = &restartConfirmation{s.Id, s.RunId, time.Now().Add(30 * time.Second)}
		m.notice = "Restart stops active work. /restart confirm (30s) · /restart cancel · history/login kept; approval resets."
		return nil
	}
	if len(f) == 2 && f[1] == "cancel" {
		m.restartConfirm = nil
		m.notice = "Restart canceled."
		return nil
	}
	if len(f) != 2 || f[1] != "confirm" {
		m.notice = "Usage: /restart · /restart confirm · /restart cancel"
		return nil
	}
	p := m.restartConfirm
	m.restartConfirm = nil
	if p == nil || p.id != s.Id || p.run != s.RunId || time.Now().After(p.expires) {
		m.notice = "Restart confirmation expired or session changed. Submit /restart again."
		return nil
	}
	m.restartBusy = true
	delete(m.fullPermission, s.Id)
	m.notice = "Restarting agent…"
	client, parent := m.client, m.ctx
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, 45*time.Second)
		defer cancel()
		current, err := client.Get(ctx, &api.SessionRef{Id: p.id})
		if err != nil {
			return restartFinished{err}
		}
		if current.RunId != p.run {
			return restartFinished{fmt.Errorf("session run changed; restart canceled")}
		}
		if current.State != "stopped" && current.State != "failed" && current.State != "interrupted" {
			if _, err = client.Stop(ctx, &api.Control{SessionId: p.id, RunId: p.run, ClientId: core.ID()}); err != nil {
				return restartFinished{fmt.Errorf("restart stop failed; resume was not attempted: %w", err)}
			}
		}
		// Never adopt a newer run started by a different client after confirmation.
		current, err = client.Get(ctx, &api.SessionRef{Id: p.id})
		if err != nil {
			return restartFinished{err}
		}
		if current.RunId != p.run {
			return restartFinished{fmt.Errorf("session run changed; restart canceled")}
		}
		_, err = client.Resume(ctx, &api.Control{SessionId: p.id, RunId: p.run, ClientId: core.ID()})
		if err != nil {
			return restartFinished{fmt.Errorf("restart resume failed; check session status before retrying: %w", err)}
		}
		return restartFinished{}
	}
}
