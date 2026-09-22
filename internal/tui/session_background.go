package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type sessionBackgroundCheck struct {
	run        string
	seq        uint64
	checking   bool
	checked    bool
	retryAfter time.Time
}

type sessionBackgroundChecked struct {
	backgroundHistory
	run string
	seq uint64
}

// The project list includes sessions whose transcript has never been opened.
// Fetch only their reduced metadata, outside Update/View, with bounded fan-out.
// Working sessions already have a spinner; inspect them when they become idle.
func (m *model) refreshSessionBackground() tea.Cmd {
	if m.client == nil || m.ctx == nil || m.ctx.Err() != nil {
		return nil
	}
	if m.sessionBackground == nil {
		m.sessionBackground = map[string]*sessionBackgroundCheck{}
	}
	inFlight := 0
	for _, check := range m.sessionBackground {
		if check.checking {
			inFlight++
		}
	}
	var cmds []tea.Cmd
	for _, s := range m.allSessions {
		if inFlight >= 4 {
			break
		}
		if s.State != "idle" || s.RunId == "" {
			continue
		}
		check := m.sessionBackground[s.Id]
		if check == nil {
			check = &sessionBackgroundCheck{}
			m.sessionBackground[s.Id] = check
		}
		if check.checking || check.run == s.RunId && (time.Now().Before(check.retryAfter) || check.checked && check.seq >= s.LastSeq) {
			continue
		}
		if snapshot, ok := m.backgroundSnapshots[s.Id]; ok && snapshot.lastSeq >= s.LastSeq {
			continue
		}
		check.run, check.seq, check.checking = s.RunId, s.LastSeq, true
		id, run, seq := s.Id, s.RunId, s.LastSeq
		fetch := m.fetchBackground(m.ctx, id, 0)
		cmds = append(cmds, func() tea.Msg {
			return sessionBackgroundChecked{backgroundHistory: fetch().(backgroundHistory), run: run, seq: seq}
		})
		inFlight++
	}
	return tea.Batch(cmds...)
}

func (m *model) receiveSessionBackground(v sessionBackgroundChecked) {
	check := m.sessionBackground[v.id]
	if check == nil || !check.checking || check.run != v.run || check.seq != v.seq {
		return
	}
	check.checking = false
	if v.err != nil {
		check.checked = false
		check.retryAfter = time.Now().Add(30 * time.Second)
		return
	}
	check.checked, check.seq = true, max(v.seq, v.snapshot.lastSeq)
	check.retryAfter = time.Time{}
	for _, s := range m.allSessions {
		if s.Id != v.id || s.RunId != v.run {
			continue
		}
		m.storeBackgroundSnapshot(v.id, v.snapshot)
		if current := m.current(); current != nil && current.Id == v.id {
			m.render()
		}
		break
	}
}
