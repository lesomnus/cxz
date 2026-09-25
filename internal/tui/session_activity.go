package tui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/notification"
)

// Read state belongs to this frontend, keyed by the full connection/session ID.
// Opening the TUI establishes a baseline, not hundreds of historical alerts.
type sessionActivity struct {
	run, state          string
	lastSeq, done, seen uint64
	checking            bool
	notified            uint64
	pendingAlerts       map[pendingAlertKey]bool
}

type completionChecked struct {
	id, run   string
	from, end uint64
	done      uint64
	err       error
}

func workingState(state string) bool {
	return state == "working" || state == "waiting_input"
}

func workingSpinner(pulse int) string {
	frames := []rune("⣟⣯⣷⣾⣽⣻⢿⡿")
	return string(frames[pulse%len(frames)])
}

func (m *model) observeSessions(sessions []*api.Session) tea.Cmd {
	if m.sessionActivity == nil {
		m.sessionActivity = map[string]*sessionActivity{}
	}
	var cmds []tea.Cmd
	for _, s := range sessions {
		a := m.sessionActivity[s.Id]
		if a == nil {
			a = &sessionActivity{run: s.RunId, state: s.State, lastSeq: s.LastSeq, seen: s.LastSeq, notified: s.LastSeq}
			a.observePending(s)
			m.sessionActivity[s.Id] = a
			continue
		}
		if s.LastSeq < a.lastSeq {
			continue // An older list result must not resurrect completion markers.
		}
		if a.run != s.RunId {
			// Resuming the agent does not acknowledge its previous reply.
			from := a.lastSeq
			a.run, a.state, a.lastSeq, a.checking = s.RunId, s.State, s.LastSeq, false
			a.notified, a.pendingAlerts = from, map[pendingAlertKey]bool{}
			if a.observePending(s) {
				cmds = append(cmds, requestSound(notification.Attention))
			}
			// A new run can finish before its first resource snapshot arrives.
			if s.State == "idle" && s.LastSeq > from && m.client != nil {
				a.lastSeq, a.checking = from, true
				cmds = append(cmds, m.checkCompletion(s.Id, s.RunId, from, s.LastSeq))
			}
			continue
		}
		if a.observePending(s) {
			cmds = append(cmds, requestSound(notification.Attention))
		}
		if workingState(a.state) && s.State == "idle" && s.LastSeq > a.seen {
			a.done = s.LastSeq
		}
		if workingState(a.state) && s.State == "idle" && s.LastSeq > a.notified {
			a.notified = s.LastSeq
			cmds = append(cmds, requestSound(notification.Complete))
		}
		// Resource updates are coalesced. A short turn can begin and end between
		// two idle snapshots. Inspect only the new range in that case; unrelated
		// quota, permission, or updater events must never create a '+' marker.
		if !a.checking && a.state == "idle" && s.State == "idle" && s.LastSeq > a.lastSeq && s.LastSeq > a.notified && m.client != nil {
			a.checking = true
			cmds = append(cmds, m.checkCompletion(s.Id, s.RunId, a.lastSeq, s.LastSeq))
		} else if !a.checking {
			a.lastSeq = s.LastSeq
		}
		a.state = s.State
	}
	return tea.Batch(cmds...)
}

func (m *model) checkCompletion(id, run string, from, end uint64) tea.Cmd {
	parent := m.ctx
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, 5*time.Second)
		defer cancel()
		out := completionChecked{id: id, run: run, from: from, end: end}
		// Read backwards in bounded pages so the latest completion usually needs
		// one request. No transcript rendering or additional live watch is needed.
		for before := end; before > from; {
			after := from
			if before-from > historyPageSize {
				after = before - historyPageSize
			}
			batch, err := m.fetchHistory(ctx, id, after, "completion")
			if err != nil {
				out.err = err
				return out
			}
			for _, e := range batch.GetEvents() {
				if e.RunId == run && e.Kind == "turn_end" && e.Seq > from && e.Seq <= end {
					out.done = max(out.done, e.Seq)
				}
			}
			if out.done > 0 {
				return out
			}
			before = after
		}
		return out
	}
}

func (m *model) receiveCompletion(v completionChecked) tea.Cmd {
	a := m.sessionActivity[v.id]
	if a == nil || a.run != v.run || !a.checking || a.lastSeq != v.from {
		return nil
	}
	a.checking = false
	if v.err == nil {
		a.lastSeq = v.end
		a.done = max(a.done, v.done)
		if v.done > a.notified {
			a.notified = v.done
			return requestSound(notification.Complete)
		}
	}
	return nil
}

func (m *model) sessionIndicator(s *api.Session) string {
	if workingState(s.State) || len(m.pendingInputs[s.Id]) > 0 || m.hasActiveBackground(s) {
		return accent.Render(workingSpinner(m.pulse))
	}
	if a := m.sessionActivity[s.Id]; a != nil && a.done > a.seen {
		return accent.Render("+")
	}
	return " "
}

// Run only after the actual transcript is shown, not while its skeleton or an
// overlay is visible. Scrolling older messages does not acknowledge a new reply.
func (m *model) acknowledgeSession() {
	s := m.current()
	if s == nil || m.projectView || m.accountView || m.creating || m.panelFocus ||
		m.settingsPage != nil || m.memoryPage != nil || m.workflow != nil ||
		!m.previewInteraction() || m.errorFocused() || m.terminalFocused() || m.focusApproval ||
		m.focusList || m.selectingTools() || (m.previewVisible() && m.filePreview.focused) ||
		!m.view.AtBottom() || m.historyOpening[s.Id] {
		return
	}
	if a := m.sessionActivity[s.Id]; a != nil {
		a.seen = max(a.seen, m.cursor[s.Id])
	}
}
