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
		id, run, request := s.Id, s.RunId, core.ID()
		m.contextCapture = &contextCapture{id: id, run: run, request: request, report: m.report}
		return func() tea.Msg {
			ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
			defer cancel()
			_, err := m.client.Send(ctx, &api.Input{SessionId: id, RunId: run, ClientId: request, Text: "/context"})
			return contextSent{request: request, err: err}
		}
	}
	lines := []string{"Context · " + pickerLabel(s.Agent)}
	events := m.events[s.Id]
	for i := len(events) - 1; i >= 0; i-- {
		e := events[i]
		if e.Kind == "compact" {
			lines = append(lines, "Context compacted; waiting for a new token-usage snapshot.")
			m.recordLocal("/context", strings.Join(lines, "\n"))
			return nil
		}
		if e.Kind != "usage" || e.Text != "thread/tokenUsage/updated" {
			continue
		}
		u := fields(e.Payload).object("tokenUsage")
		last := u.object("last")
		if e.TimeMs > 0 {
			lines = append(lines, "Provider snapshot · "+time.UnixMilli(e.TimeMs).Local().Format("01-02 15:04"))
		}
		used, hasUsed := last.number("totalTokens")
		window, hasWindow := u.number("modelContextWindow")
		if hasUsed {
			lines = append(lines, "Last turn footprint  "+humanCount(used)+" tokens")
		}
		if hasWindow && window > 0 {
			lines = append(lines, "Model context window "+humanCount(window)+" tokens")
			if hasUsed {
				lines = append(lines, fmt.Sprintf("Reported utilization %.1f%% · %s tokens headroom", used/window*100, humanCount(max(0, window-used))))
			}
		}
		for _, key := range []string{"inputTokens", "outputTokens", "cachedInputTokens", "reasoningOutputTokens"} {
			if n, ok := last.number(key); ok {
				lines = append(lines, fmt.Sprintf("%-21s%s", key, humanCount(n)))
			}
		}
		lines = append(lines, "Last reported turn, not a live token count. System/tool category breakdown is not reported. /compact reduces agent context; the cxz journal is retained.")
		m.recordLocal("/context", strings.Join(lines, "\n"))
		return nil
	}
	m.recordLocal("/context", strings.Join(lines, "\n")+"\nProvider has not reported context usage yet. No estimate is substituted.")
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
			c.report.text = "Context query was superseded by another input. No unrelated response was captured."
			m.contextCapture = nil
			return
		}
	case "assistant":
		if c.active {
			hide = true
			c.text += e.Text + "\n"
			c.report.text = c.text
		}
	case "turn_end":
		if c.active {
			hide = true
			if c.text == "" {
				c.report.text = "No text context report received. Inspect session events for provider details."
			}
			m.contextCapture = nil
		}
	case "state":
		if e.Text == "failed" || e.Text == "stopped" || e.Text == "interrupted" {
			c.report.text = "Context query ended: " + e.Text
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
