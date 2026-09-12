package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func (m *model) contextCommand() tea.Cmd {
	s := m.current()
	if s == nil {
		m.recordLocal("/context", "Select a session first.")
		return nil
	}
	if s.Agent == "claude" {
		if s.State != "idle" {
			m.recordLocal("/context", "Claude /context requires an idle session.")
			return nil
		}
		// Native command returns the authoritative breakdown as an assistant
		// report (including system/tools/memory), without inventing token counts.
		return m.action("send", "/context")
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
