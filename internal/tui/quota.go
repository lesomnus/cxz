package tui

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
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
		if mins/60%24 == 0 {
			return fmt.Sprintf("%dd", mins/1440)
		}
		return fmt.Sprintf("%dd%dh", mins/1440, mins/60%24)
	}
	if mins >= 60 {
		if mins%60 == 0 {
			return fmt.Sprintf("%dh", mins/60)
		}
		return fmt.Sprintf("%dh%dm", mins/60, mins%60)
	}
	return fmt.Sprintf("%dm", mins)
}

func quotaBar(percent float64) string {
	return quotaBarWidth(percent, 8)
}

func quotaBarWidth(percent float64, cells int) string {
	// Eight cells, with four levels per cell; the baseline remains visible.
	levels := []rune{'⣀', '⣄', '⣤', '⣶', '⣿'}
	filled := int(math.Round(max(0, min(100, percent)) * float64(cells*4) / 100))
	var b strings.Builder
	for i := 0; i < cells; i++ {
		b.WriteRune(levels[max(0, min(4, filled-i*4))])
	}
	return b.String()
}

func quotaBarStyle(percent float64) lipgloss.Style {
	switch {
	case percent <= 15:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#F49BAA"))
	case percent <= 30:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#F5AF98"))
	case percent <= 50:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#F5CA9A"))
	default:
		return muted
	}
}

func (m *model) updateQuota() {
	s := m.current()
	if s == nil {
		m.quotaWindows, m.quotaState = nil, "waiting"
		return
	}
	events := m.events[s.Id]
	run := s.RunId
	if s.Agent == "claude" && s.Account != "" {
		events = nil
		sessions := m.allSessions
		if len(sessions) == 0 {
			sessions = m.sessions
		}
		for _, peer := range sessions {
			if peer.Agent != s.Agent || peer.Account != s.Account {
				continue
			}
			for _, e := range m.events[peer.Id] {
				if (e.Kind == "usage" || e.Kind == "usage_status") && (e.RunId == "" || e.RunId == peer.RunId) {
					events = append(events, e)
				}
			}
		}
		sort.SliceStable(events, func(i, j int) bool { return events[i].TimeMs < events[j].TimeMs })
		run = ""
	}
	m.quotaWindows, m.quotaState = quotaSnapshot(s.Agent, run, events)
	m.quotaState = quotaPendingState(m.quotaState, s.RunId, m.events[s.Id], time.Now())
	if s.Agent == "claude" && s.Account != "" {
		if m.accountQuotas == nil {
			m.accountQuotas = map[string][]agentview.Window{}
		}
		previous := m.accountQuotas[s.Account]
		if len(m.quotaWindows) == 0 {
			m.quotaWindows = previous
		} else {
			for i, w := range m.quotaWindows {
				for _, old := range previous {
					if w.Key == old.Key && old.Observed.After(w.Observed) {
						m.quotaWindows[i] = old
					}
				}
			}
			m.accountQuotas[s.Account] = append([]agentview.Window(nil), m.quotaWindows...)
		}
	}
}

func quotaPendingState(state, run string, events []*api.Event, now time.Time) string {
	if state != "polling" {
		return state
	}
	for i := len(events) - 1; i >= 0; i-- {
		e := events[i]
		if e.Kind == "usage_status" && e.Text == "polling" && (e.RunId == "" || e.RunId == run) && e.TimeMs > 0 {
			if now.Sub(time.UnixMilli(e.TimeMs)) >= time.Minute {
				return "timeout"
			}
			break
		}
	}
	return state
}

func quotaSnapshot(provider, run string, events []*api.Event) ([]agentview.Window, string) {
	state := "waiting"
	windows := map[string]agentview.Window{}
	for _, e := range events {
		if run != "" && e.RunId != "" && e.RunId != run {
			continue
		}
		if e.Kind == "usage_status" {
			switch e.Text {
			case "unsupported", "error", "unavailable", "timeout":
				state = e.Text
			case "polling":
				if state == "waiting" {
					state = "polling"
				}
			}
			continue
		}
		if e.Kind != "usage" {
			continue
		}
		loaded := agentview.Quota(provider, e.Text, e.Payload)
		all, prefix := agentview.QuotaReplacement(provider, e.Text, e.Payload)
		if provider == "claude" && all && len(loaded) == 0 && len(windows) > 0 {
			state = "unavailable"
			continue
		}
		previous := windows
		if all {
			previous = map[string]agentview.Window{}
			for key, w := range windows {
				previous[key] = w
			}
		}
		if all {
			clear(windows)
		} else if prefix != "" {
			for key := range windows {
				if strings.HasPrefix(key, prefix) {
					delete(windows, key)
				}
			}
		}
		if len(loaded) > 0 {
			state = "available"
		} else if all || prefix != "" {
			state = "unavailable"
		}
		for _, w := range loaded {
			if e.TimeMs > 0 {
				w.Observed = time.UnixMilli(e.TimeMs)
			}
			if observed, ok := fields(e.Payload).number("_cxz_observed_ms"); ok && observed > 0 {
				w.Observed = time.UnixMilli(int64(observed))
			}
			if old, ok := previous[w.Key]; ok {
				if old.Observed.After(w.Observed) {
					windows[w.Key] = old
					continue
				}
				if w.Remaining == nil && (w.Reset.IsZero() || w.Reset.Equal(old.Reset)) {
					w.Remaining = old.Remaining
					w.Observed = old.Observed
				}
				if w.Reset.IsZero() {
					w.Reset = old.Reset
				}
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
	provider, current := "", ""
	if s := m.current(); s != nil {
		provider = s.Agent
		current = m.quotaModel()
	}
	windows, folded := statusQuotaWindows(provider, current, m.quotaWindows)
	render := func(w agentview.Window, cells int) string {
		value := "—"
		if w.Remaining != nil && (w.Reset.IsZero() || w.Reset.After(now)) {
			value = fmt.Sprintf("%.0f%%", *w.Remaining)
			if cells > 0 {
				value += " " + quotaBarStyle(*w.Remaining).Render(quotaBarWidth(*w.Remaining, cells))
			}
		}
		if m.quotaState == "error" || m.quotaState == "timeout" || m.quotaState == "unavailable" || w.Observed.IsZero() || now.Sub(w.Observed) > 2*time.Minute {
			value = "~" + value
		}
		return value + " " + safeText(w.Label) + " " + quotaCountdown(w.Reset, now)
	}
	for count := len(windows); count >= 0; count-- {
		for _, cells := range []int{8, 4, 0} {
			var parts []string
			for _, w := range windows[:count] {
				parts = append(parts, render(w, cells))
			}
			if n := folded + len(windows) - count; n > 0 {
				parts = append(parts, fmt.Sprintf("+%d limits", n))
			}
			text := strings.Join(parts, " · ")
			if ansi.StringWidth(text) <= width {
				return muted.Render(text)
			}
		}
	}
	return muted.Render(clip("quota · /usage", width))
}

func quotaHistoryReport(provider, run string, events []*api.Event) string {
	windows, state := quotaSnapshot(provider, run, events)
	state = quotaPendingState(state, run, events, time.Now())
	report := "Account quota · " + provider + " · " + state + "\n"
	switch state {
	case "polling":
		report += "The supervisor sent a quota request and is awaiting the provider response. Active agents poll every minute, as well as after initialization and turns."
	case "timeout":
		report += "The provider has not answered a quota request for at least one minute. The supervisor retries on its minute polling tick; conversation state is unaffected. Check provider CLI/login/network."
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
	for i := len(events) - 1; i >= 0; i-- {
		e := events[i]
		if e.Kind == "usage_status" && e.Text == "polling" && (e.RunId == "" || e.RunId == run) && e.TimeMs > 0 {
			report += "\nLast quota request · " + time.UnixMilli(e.TimeMs).Local().Format("01-02 15:04:05") + " · poll interval 1m"
			break
		}
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
