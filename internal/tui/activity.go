package tui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lesomnus/cxz/api"
)

type activityReported struct{}

func (m *model) reportActivity() tea.Cmd {
	s := m.current()
	if m.program == nil || s == nil || m.activityID == "" || time.Since(m.lastActivityReport) < 10*time.Second {
		return nil
	}
	m.lastActivityReport = time.Now()
	busy := m.settingsPage != nil || m.redactDialog != nil || m.redactSending || m.input.Value() != "" || m.busy || m.workflow != nil || m.creating || m.renaming || m.restartConfirm != nil || m.pasteDialog != nil || time.Since(m.lastUIInput) < 10*time.Second
	r := &api.ActivityInput{SessionId: s.Id, RunId: s.RunId, ClientId: m.activityID, Busy: busy}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 3*time.Second)
		defer cancel()
		_, _ = m.client.Activity(ctx, r)
		return activityReported{}
	}
}
