package tui

import (
	"context"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/cxzupdate"
	"time"
)

type cxzUpdateResult struct {
	state cxzupdate.State
	path  string
	err   error
}

func (m *model) frontendUpdate() tea.Cmd {
	c, ok := cxzupdate.ClientFrom(m.ctx)
	if !ok || m.program == nil {
		return nil
	}
	if m.autoCandidate != "" {
		m.autoState.Reason = m.frontendWaitReason(time.Now())
	}
	if m.autoCandidate != "" && m.autoState.Release != nil && m.frontendIdle(time.Now()) {
		policy, e := cxzupdate.Policy(c.Root)
		if e != nil || !policy.Active() {
			return nil
		}
		resume := cxzupdate.Resume{Revision: m.autoState.Release.Revision, Connection: m.connectionRef()}
		if s := m.current(); s != nil {
			resume.Session = s.Id
			resume.Project = s.ProjectId
		}
		if i := m.view.YOffset; i >= 0 && i < len(m.historyPositions) {
			resume.Position = m.historyPositions[i]
		}
		// Only navigation metadata is persisted. Drafts, secrets and clipboard data
		// are never serialized as part of an automatic restart.
		if e = core.WriteJSON(cxzupdate.ResumePath(c.Root), resume); e != nil {
			m.notice = e.Error()
			return nil
		}
		m.autoRestart = &cxzupdate.Restart{Client: c, Candidate: m.autoCandidate, Release: *m.autoState.Release, Resume: resume}
		return tea.Quit
	}
	if m.autoChecking || time.Since(m.autoChecked) < time.Minute {
		return nil
	}
	m.autoChecking = true
	m.autoChecked = time.Now()
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 6*time.Minute)
		defer cancel()
		s, path, e := cxzupdate.ClientCandidate(ctx, c)
		return cxzUpdateResult{s, path, e}
	}
}
func (m *model) frontendIdle(now time.Time) bool {
	if m.autoStarted.IsZero() || now.Sub(m.autoStarted) < cxzupdate.IdlePeriod || now.Sub(m.lastUIInput) < cxzupdate.IdlePeriod {
		return false
	}
	if m.download != nil || m.input.Value() != "" || m.busy || m.creating || m.renaming || m.renameBusy || m.restartBusy || m.deletingID != "" || m.workflow != nil || m.redactDialog != nil || m.redactSending || m.pasteDialog != nil || m.questionDialog != nil || m.restartConfirm != nil || m.modelPicker != nil || m.settingsPage != nil || m.memoryPage != nil || m.library != nil || m.report != nil || m.errorDialog != nil || m.sessionArchive != nil || m.accountView || m.focusApproval {
		return false
	}
	if len(m.terminals) > 0 || m.recordingSaving || m.recordingPending != nil || m.recordingTask != nil {
		return false
	}
	if m.debugRecorder != nil && m.debugRecorder.Active() {
		return false
	}
	for _, s := range m.drafts {
		if s != "" {
			return false
		}
	}
	for _, s := range m.pendingInputs {
		if len(s) > 0 {
			return false
		}
	}
	if len(m.questionDrafts) > 0 || len(m.redactions) > 0 {
		return false
	}
	for _, p := range m.pastes {
		if p != nil && (p.attachment != nil || p.upload != nil) {
			return false
		}
	}
	return true
}

func (m *model) frontendWaitReason(now time.Time) string {
	if m.input.Value() != "" {
		return "waiting: unsent draft"
	}
	for _, draft := range m.drafts {
		if draft != "" {
			return "waiting: unsent draft in another session"
		}
	}
	if len(m.terminals) > 0 {
		return "waiting: terminal session open"
	}
	if now.Sub(m.lastUIInput) < cxzupdate.IdlePeriod || now.Sub(m.autoStarted) < cxzupdate.IdlePeriod {
		return "waiting: 5 minutes without frontend input"
	}
	if !m.frontendIdle(now) {
		return "waiting: dialog, attachment or request still active"
	}
	return "ready to restart frontend"
}
