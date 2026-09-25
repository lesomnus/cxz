package tui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/notification"
)

type soundRequested struct{ sound notification.Sound }

func requestSound(sound notification.Sound) tea.Cmd {
	return func() tea.Msg { return soundRequested{sound} }
}

func (m *model) playNotification(sound notification.Sound) tea.Cmd {
	if m.cursorOutput == nil {
		return nil
	}
	ctx := m.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	if m.alertPlayer == nil {
		m.alertPlayer = &notification.Player{}
	}
	player, output := m.alertPlayer, m.cursorOutput
	return func() tea.Msg {
		if !player.Play(ctx, sound) && ctx.Err() == nil {
			// Use the same locked writer as the renderer, including over SSH.
			_, _ = output.Write([]byte{'\a'})
		}
		return nil
	}
}

type pendingAlertKey struct {
	run, request string
	seq          uint64
}

// Track requests independently of focus and read markers. Repeated snapshots,
// selection changes and journal replay must not re-announce the same request.
func (a *sessionActivity) observePending(s *api.Session) bool {
	baseline := a.pendingAlerts == nil
	if baseline {
		a.pendingAlerts = map[pendingAlertKey]bool{}
	}
	attention := false
	for _, p := range s.Pending {
		if p == nil || (p.RunId != "" && p.RunId != s.RunId) {
			continue
		}
		key := pendingAlertKey{s.RunId, p.RequestId, p.Seq}
		if a.pendingAlerts[key] {
			continue
		}
		a.pendingAlerts[key] = true
		// Automatically handled approvals do not require the user's attention,
		// regardless of which screen is currently visible.
		if !baseline && !(s.PermissionMode == "full" && automaticApproval(p)) {
			attention = true
		}
	}
	return attention
}
