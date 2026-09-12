package tui

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
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
	m.quotaWindows = nil
	s := m.current()
	if s == nil {
		return
	}
	windows := map[string]agentview.Window{}
	for _, e := range m.events[s.Id] {
		if e.Kind != "usage" || (e.RunId != "" && e.RunId != s.RunId) {
			continue
		}
		all, prefix := agentview.QuotaReplacement(s.Agent, e.Text, e.Payload)
		if all {
			clear(windows)
		} else if prefix != "" {
			for key := range windows {
				if strings.HasPrefix(key, prefix) {
					delete(windows, key)
				}
			}
		}
		for _, w := range agentview.Quota(s.Agent, e.Text, e.Payload) {
			if e.TimeMs > 0 {
				w.Observed = time.UnixMilli(e.TimeMs)
			}
			windows[w.Key] = w
		}
	}
	for _, w := range windows {
		m.quotaWindows = append(m.quotaWindows, w)
	}
	sort.Slice(m.quotaWindows, func(i, j int) bool { return m.quotaWindows[i].Key < m.quotaWindows[j].Key })
}

func (m *model) quotaStatus(now time.Time, width int) string {
	if len(m.quotaWindows) == 0 {
		return muted.Render("quota —")
	}
	var parts []string
	for _, w := range m.quotaWindows {
		value := "—"
		if w.Remaining != nil && (w.Reset.IsZero() || w.Reset.After(now)) {
			value = fmt.Sprintf("%.0f%% %s", *w.Remaining, quotaBar(*w.Remaining))
		}
		if w.Observed.IsZero() || now.Sub(w.Observed) > 2*time.Minute {
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
