package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func (m *model) cycleFocus(reverse bool) tea.Cmd {
	// Physical top-to-bottom order: approvals, composer, bottom session bar.
	order := []string{"input", "session"}
	if m.selectedApproval() != nil {
		order = append([]string{"approval"}, order...)
	}
	current := "input"
	if m.focusApproval {
		current = "approval"
	} else if m.focusList {
		current = "session"
	}
	index := 0
	for i, name := range order {
		if name == current {
			index = i
		}
	}
	step := 1
	if reverse {
		step = len(order) - 1
	}
	next := order[(index+step)%len(order)]
	m.focusApproval, m.focusList = next == "approval", next == "session"
	if m.focusApproval {
		m.approvalID = m.selectedApproval().RequestId
	}
	if next == "input" {
		return m.input.Focus()
	}
	m.input.Blur()
	return nil
}

func (m *model) confirmInterrupt(now time.Time) tea.Cmd {
	s := m.current()
	if s == nil || (s.State != "working" && s.State != "waiting_input") {
		return nil
	}
	key := s.Id + "/" + s.RunId
	if m.interruptKey == key && now.Before(m.interruptUntil) {
		m.interruptUntil = time.Time{}
		m.interruptKey = ""
		return m.action("interrupt", "")
	}
	m.interruptKey, m.interruptUntil = key, now.Add(3*time.Second)
	return nil
}

func (m *model) workingLabel(now time.Time) string {
	s := m.current()
	if s == nil {
		return ""
	}
	started := m.workingSince
	duration := "--:--:--"
	if started > 0 {
		sec := max(int64(0), (now.UnixMilli()-started)/1000)
		duration = fmt.Sprintf("%02d:%02d:%02d", sec/3600, sec/60%60, sec%60)
	}
	activity := "working"
	if s.State == "waiting_input" {
		if p := m.selectedApproval(); p != nil {
			activity = "waiting for approval"
			if question(p) {
				activity = "waiting for answer"
			}
		} else if s.PermissionMode != "full" || s.RunId == "" {
			activity = "waiting for input"
		}
	}
	label := activity + " " + duration + " · Esc interrupt"
	if m.interruptKey == s.Id+"/"+s.RunId && now.Before(m.interruptUntil) {
		label = activity + " " + duration + " · Esc again to interrupt (3s)"
	}
	return label
}

// A tool permission round-trip does not end the turn. Both rendering stages
// must agree on the reserved activity row, including during automatic approval.
func (m *model) activeWork() bool {
	s := m.current()
	return s != nil && (s.State == "working" || s.State == "waiting_input")
}

func (m *model) compactCommand(text string) tea.Cmd {
	s := m.current()
	if s == nil || s.State != "idle" {
		m.recordLocal("/compact", "Compaction requires an idle session.")
		return nil
	}
	if strings.TrimSpace(text) != "/compact" {
		m.recordLocal("/compact", "Usage: /compact")
		return nil
	}
	return m.action("send", "/compact")
}
