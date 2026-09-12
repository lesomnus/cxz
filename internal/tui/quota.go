package tui

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/agentview"
)

func quotaCountdown(reset, now time.Time) string {
	if reset.IsZero() {
		return "—"
	}
	if !reset.After(now) {
		return "refresh"
	}
	mins := int(math.Ceil(reset.Sub(now).Minutes()))
	if mins >= 1440 {
		return fmt.Sprintf("%dd%dh", mins/1440, mins/60%24)
	}
	if mins >= 60 {
		return fmt.Sprintf("%dh%dm", mins/60, mins%60)
	}
	return fmt.Sprintf("%dm", mins)
}

func quotaBar(percent float64) string {
	// Eight cells, with four levels per cell; the baseline remains visible.
	levels := []rune{'⣀', '⣄', '⣤', '⣶', '⣿'}
	filled := int(math.Round(max(0, min(100, percent)) * 32 / 100))
	var b strings.Builder
	for i := 0; i < 8; i++ {
		b.WriteRune(levels[max(0, min(4, filled-i*4))])
	}
	return b.String()
}

func (m *model) updateQuota() {
	s := m.current()
	if s == nil {
		m.quotaWindows, m.quotaState = nil, "waiting"
		return
	}
	m.quotaWindows, m.quotaState = quotaSnapshot(s.Agent, s.RunId, m.events[s.Id])
}

func quotaSnapshot(provider, run string, events []*api.Event) ([]agentview.Window, string) {
	state := "waiting"
	windows := map[string]agentview.Window{}
	for _, e := range events {
		if e.RunId != "" && e.RunId != run {
			continue
		}
		if e.Kind == "usage_status" {
			switch e.Text {
			case "unsupported", "error", "unavailable":
				state = e.Text
			}
			continue
		}
		if e.Kind != "usage" {
			continue
		}
		all, prefix := agentview.QuotaReplacement(provider, e.Text, e.Payload)
		if all {
			clear(windows)
		} else if prefix != "" {
			for key := range windows {
				if strings.HasPrefix(key, prefix) {
					delete(windows, key)
				}
			}
		}
		loaded := agentview.Quota(provider, e.Text, e.Payload)
		if len(loaded) > 0 {
			state = "available"
		} else if all || prefix != "" {
			state = "unavailable"
		}
		for _, w := range loaded {
			if e.TimeMs > 0 {
				w.Observed = time.UnixMilli(e.TimeMs)
			}
			windows[w.Key] = w
		}
	}
	var result []agentview.Window
	for _, w := range windows {
		result = append(result, w)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Key < result[j].Key })
	return result, state
}

func (m *model) quotaStatus(now time.Time, width int) string {
	if len(m.quotaWindows) == 0 {
		state := m.quotaState
		if state == "" {
			state = "waiting"
		}
		return muted.Render(clip("quota "+state, width))
	}
	var parts []string
	for _, w := range m.quotaWindows {
		value := "—"
		if w.Remaining != nil && (w.Reset.IsZero() || w.Reset.After(now)) {
			value = fmt.Sprintf("%.0f%% %s", *w.Remaining, quotaBar(*w.Remaining))
		}
		if m.quotaState == "error" || w.Observed.IsZero() || now.Sub(w.Observed) > 2*time.Minute {
			value = "~" + value
		}
		parts = append(parts, value+" "+safeText(w.Label)+" "+quotaCountdown(w.Reset, now))
	}
	for len(parts) > 1 && ansi.StringWidth(strings.Join(parts, " · ")) > width {
		parts = parts[:len(parts)-1]
		parts[len(parts)-1] += " +"
	}
	return muted.Render(clip(strings.Join(parts, " · "), width))
}

func quotaHistoryReport(provider, run string, events []*api.Event) string {
	windows, state := quotaSnapshot(provider, run, events)
	report := "Account quota · " + provider + " · " + state + "\n"
	switch state {
	case "waiting":
		report += "No quota telemetry received for this run. Existing supervisors keep their old code: update and restart the agent if this persists."
	case "unsupported":
		report += "The running provider CLI rejected the usage-query method. Update the provider CLI and restart the agent."
	case "unavailable":
		report += "The provider returned no quota windows for this login. API-key accounts or missing profile scope may not expose subscription limits."
	case "error":
		report += "Provider quota query failed; it will retry. Check provider login/network. This does not fail the conversation."
	default:
		report += "Remaining quota is provider-reported, not inferred from session tokens."
	}
	for _, w := range windows {
		value := "percentage not reported"
		if w.Remaining != nil {
			value = fmt.Sprintf("%.0f%% remaining (snapshot)", *w.Remaining)
		}
		report += "\n" + safeText(w.Label) + ": " + value
		if !w.Observed.IsZero() {
			report += " · observed " + w.Observed.Local().Format("01-02 15:04")
		}
		if !w.Reset.IsZero() {
			report += " · resets " + w.Reset.Local().Format("01-02 15:04")
		}
	}
	return report
}
