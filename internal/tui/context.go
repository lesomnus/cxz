package tui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/agentview"
	"github.com/lesomnus/cxz/internal/core"
)

func (m *model) contextCommand() tea.Cmd {
	s := m.current()
	if s == nil {
		m.recordLocal("/context", "Select a session first.")
		return nil
	}
	if s.Agent == "claude" {
		if c := m.contextCapture; c != nil && c.id == s.Id && c.run == s.RunId {
			m.report = c.report
			return nil
		}
		if s.State != "idle" {
			m.recordLocal("/context", "Claude /context requires an idle session.")
			return nil
		}
		// Native command returns the authoritative breakdown as an assistant
		// report (including system/tools/memory), without inventing token counts.
		if m.contextCapture != nil {
			return nil
		}
		m.openReport("/context", "Loading provider context…")
		m.report.setContext(agentview.ContextReport{Provider: s.Agent, Model: s.Model, Basis: "Native /context snapshot", Note: "Loading provider context…"}, "", 0)
		id, run, request := s.Id, s.RunId, core.ID()
		m.contextCapture = &contextCapture{id: id, run: run, request: request, report: m.report}
		return func() tea.Msg {
			ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
			defer cancel()
			_, err := m.client.Send(ctx, &api.Input{SessionId: id, RunId: run, ClientId: request, Text: "/context"})
			return contextSent{request: request, err: err}
		}
	}
	m.openReport("/context", "")
	report := agentview.ContextReport{Provider: s.Agent, Model: s.Model, Basis: "Last reported turn", Note: "Provider has not reported context usage yet. No estimate is substituted."}
	m.report.setContext(report, "", 0)
	if s.Agent != "codex" {
		return nil
	}
	events := m.events[s.Id]
	for i := len(events) - 1; i >= 0; i-- {
		e := events[i]
		if e.RunId != s.RunId {
			continue
		}
		if e.Kind == "compact" {
			m.report.contextNote("Context compacted; waiting for a new token-usage snapshot.")
			return nil
		}
		if e.Kind != "usage" || e.Text != "thread/tokenUsage/updated" {
			continue
		}
		report = agentview.ParseCodexContext(e.Payload)
		report.Model = s.Model
		m.report.setContext(report, string(e.Payload), e.TimeMs)
		return nil
	}
	return nil
}

type contextCapture struct {
	id, run, request string
	active           bool
	text             string
	report           *reportOverlay
}
type contextSent struct {
	request string
	err     error
}

// Correlate with the echo of OUR client ID before consuming any answer. Another
// client's turn is never taken simply because it arrived after opening a modal.
func (m *model) captureContext(id string, e *api.Event) {
	c := m.contextCapture
	if c == nil || c.id != id || c.run != e.RunId {
		return
	}
	hide := false
	switch e.Kind {
	case "input":
		if e.RequestId == c.request {
			c.active = true
			hide = true
		} else if c.active {
			c.report.contextNote("Context query was superseded by another input. No unrelated response was captured.")
			m.contextCapture = nil
			return
		}
	case "assistant":
		if c.active {
			hide = true
			if c.text != "" {
				c.text += "\n"
			}
			c.text += e.Text
			c.report.setContext(agentview.ParseClaudeContext(c.text), c.text, e.TimeMs)
			c.report.contextNote("Receiving provider context…")
		}
	case "turn_end":
		if c.active {
			c.report.setContext(agentview.ParseClaudeContext(c.text), c.text, e.TimeMs)
			if p, ok := agentview.ClaudeContextReport(c.text); ok {
				if m.contextReports == nil {
					m.contextReports = make(map[string]contextReportSnapshot)
				}
				m.contextReports[id] = contextReportSnapshot{run: c.run, seq: e.Seq, percent: p}
			}
			hide = true
			if c.text == "" {
				c.report.contextNote("No text context report received. Inspect session events for provider details.")
			}
			m.contextCapture = nil
		}
	case "state":
		if e.Text == "failed" || e.Text == "stopped" || e.Text == "interrupted" {
			c.report.contextNote("Context query ended: " + e.Text)
			m.contextCapture = nil
		}
	}
	if hide {
		if m.hiddenEvents == nil {
			m.hiddenEvents = map[*api.Event]bool{}
		}
		m.hiddenEvents[e] = true
	}
}
