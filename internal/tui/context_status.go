package tui

import (
	"fmt"
	"github.com/lesomnus/cxz/internal/agentview"
)

type contextReportSnapshot struct {
	run     string
	seq     uint64
	percent int
}

// Braille dots fill bottom-up, left then right within each row.
func contextBadge(percent int, known bool) string {
	if !known {
		return "⠀—"
	}
	percent = max(0, min(99, percent))
	dots := []rune{0x40, 0x80, 0x04, 0x20, 0x02, 0x10, 0x01, 0x08}
	r := rune(0x2800)
	for _, dot := range dots[:min(8, percent/11)] {
		r |= dot
	}
	return fmt.Sprintf("%c%d%%", r, percent)
}

func (m *model) contextStatus() string {
	s := m.current()
	if s == nil {
		return contextBadge(0, false)
	}
	var limits []byte
	events := m.events[s.Id]
	if s.Agent == "claude" {
		if report, ok := m.contextReports[s.Id]; ok && report.run == s.RunId {
			valid := true
			for i := len(events) - 1; i >= 0; i-- {
				e := events[i]
				if e.RunId != s.RunId {
					continue
				}
				if e.Seq <= report.seq {
					break
				}
				if e.Kind == "input" || e.Kind == "compact" || e.Kind == "usage" && e.Text == "context/message" {
					valid = false
					break
				}
			}
			if valid {
				return contextBadge(report.percent, true)
			}
		}
	}
	for i := len(events) - 1; i >= 0; i-- {
		e := events[i]
		if e.RunId != s.RunId {
			continue
		}
		if e.Kind == "compact" {
			break
		}
		if s.Agent == "claude" && e.Kind == "turn_end" && limits == nil && agentview.HasContextLimits(e.Payload) {
			limits = e.Payload
		}
		if e.Kind != "usage" {
			continue
		}
		if s.Agent == "codex" && e.Text == "thread/tokenUsage/updated" || s.Agent == "claude" && e.Text == "context/message" {
			// A live Claude message can precede the result carrying its window.
			if s.Agent == "claude" && limits == nil {
				for j := i - 1; j >= 0; j-- {
					if events[j].RunId == s.RunId && events[j].Kind == "turn_end" && agentview.HasContextLimits(events[j].Payload) {
						limits = events[j].Payload
						break
					}
				}
			}
			p, ok := agentview.ContextPercent(s.Agent, e.Payload, limits)
			return contextBadge(p, ok)
		}
	}
	return contextBadge(0, false)
}
