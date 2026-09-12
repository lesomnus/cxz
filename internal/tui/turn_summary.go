package tui

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
)

type metrics map[string]json.RawMessage

func fields(raw []byte) metrics {
	var m metrics
	_ = json.Unmarshal(raw, &m)
	return m
}
func (m metrics) object(key string) metrics { return fields(m[key]) }
func (m metrics) number(keys ...string) (float64, bool) {
	for _, key := range keys {
		var n float64
		if raw, ok := m[key]; ok && string(raw) != "null" && json.Unmarshal(raw, &n) == nil && n >= 0 {
			return n, true
		}
	}
	return 0, false
}
func (m metrics) text(key string) string {
	var s string
	_ = json.Unmarshal(m[key], &s)
	return safeText(s)
}

// Only display values the provider reports. Codex "last" usage is per-turn;
// cumulative thread totals must never be presented as the current reply's usage.
func turnSummary(e, usageEvent *api.Event, started int64, width int) string {
	root := fields(e.Payload)
	usage := root.object("usage")
	if len(usage) == 0 {
		usage = root.object("turn").object("usage")
	}
	if len(usage) == 0 && usageEvent != nil {
		usage = fields(usageEvent.Payload).object("tokenUsage").object("last")
	}
	var parts []string
	for _, field := range []struct {
		symbol string
		keys   []string
	}{
		{"↑", []string{"input_tokens", "inputTokens"}},
		{"↓", []string{"output_tokens", "outputTokens"}},
		{"↺", []string{"cache_read_input_tokens", "cachedInputTokens"}},
		{"⊕", []string{"cache_creation_input_tokens"}},
		{"∑", []string{"total_tokens", "totalTokens"}},
	} {
		if n, ok := usage.number(field.keys...); ok {
			parts = append(parts, fmt.Sprintf("%s %.0f", field.symbol, n))
		}
	}
	if n, ok := root.number("total_cost_usd", "costUSD", "costUsd"); ok {
		parts = append(parts, fmt.Sprintf("$%.4f USD", n))
	}
	duration, ok := root.number("duration_ms", "durationMs")
	if !ok {
		duration, ok = root.object("turn").number("durationMs", "duration_ms")
	}
	measured := false
	if !ok && started > 0 && e.TimeMs >= started {
		duration = float64(e.TimeMs - started)
		ok = true
		measured = true
	}
	if ok {
		symbol := "◷"
		if measured {
			symbol += "≈"
		}
		parts = append(parts, fmt.Sprintf("%s %.1fs", symbol, duration/1000))
	}
	var lines []string
	if e.Text != "completed" && e.Text != "" {
		detail := root.text("result")
		if detail == "" {
			detail = root.object("turn").object("error").text("message")
		}
		if detail == "" {
			detail = root.text("message")
		}
		status := "! " + safeText(e.Text)
		if detail != "" {
			status += " · " + detail
		}
		lines = append(lines, peach.Render(ansi.Hardwrap(status, max(1, width), true)))
	}
	// Wrap at metric boundaries, then right-align each row in terminal cells.
	row := ""
	flush := func() {
		if row != "" {
			lines = append(lines, muted.Render(strings.Repeat(" ", max(0, width-ansi.StringWidth(row)))+row))
			row = ""
		}
	}
	for _, part := range parts {
		next := part
		if row != "" {
			next = row + "  ·  " + part
		}
		if ansi.StringWidth(next) > width {
			flush()
			next = part
		}
		row = clip(next, width)
	}
	flush()
	return strings.Join(lines, "\n")
}
