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
	confirm bool // Cancel is selected by default.
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
	if strings.TrimSpace(text) != "/restart" {
		m.notice = "Usage: /restart — choose Confirm or Cancel in the dialog."
		return nil
	}
	m.report = nil
	if m.modelPicker != nil && m.modelPicker.cancel != nil {
		m.modelPicker.cancel()
	}
	m.modelPicker = nil
	m.restartConfirm = &restartConfirmation{id: s.Id, run: s.RunId}
	m.notice = ""
	return nil
}

func (m *model) confirmRestart() tea.Cmd {
	if m.restartBusy {
		return nil
	}
	p := m.restartConfirm
	m.restartConfirm = nil
	s := m.current()
	if p == nil || s == nil || p.id != s.Id || p.run != s.RunId {
		m.notice = "Session changed. Open /restart again."
		return nil
	}
	m.restartBusy = true
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
