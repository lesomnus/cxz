package tui

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
)

func humanCount(n float64) string {
	units := []string{"", "k", "m", "b", "t"}
	u := 0
	for n >= 1000 && u < len(units)-1 {
		n /= 1000
		u++
	}
	if u == 0 {
		return fmt.Sprintf("%.0f", n)
	}
	n = math.Round(n*10) / 10
	if n >= 1000 && u < len(units)-1 {
		n /= 1000
		u++
	}
	return strings.TrimSuffix(fmt.Sprintf("%.1f", n), ".0") + units[u]
}

// Three numeric cells plus two unit cells (scale and symbol).
func compactMetric(n float64) (string, string) {
	units := []string{" ", "k", "m", "b", "t"}
	u := 0
	for n >= 999.5 && u < len(units)-1 {
		n /= 1000
		u++
	}
	value := fmt.Sprintf("%.0f", n)
	if n < 9.95 && n != math.Trunc(n) {
		value = fmt.Sprintf("%.1f", n)
	}
	if len(value) > 3 {
		value = ">99"
	}
	return fmt.Sprintf("%3s", value), units[u]
}

func costMetric(n float64) string {
	symbol := "$"
	if n > 0 && n < 0.1 {
		symbol, n = "¢", n*100
	}
	value, scale := compactMetric(n)
	if n > 0 && n < 0.05 {
		value = "<.1"
	}
	return symbol + value + scale
}

func clockMetric(ms float64, measured bool) string {
	seconds := int64(ms / 1000)
	units := []int64{seconds / 3600, seconds / 60 % 60, seconds % 60}
	var parts []string
	for _, unit := range units {
		text := fmt.Sprintf("%02d", unit)
		if unit == 0 {
			text = zeroStyle.Render(text)
		} else {
			text = metricStyle.Render(text)
		}
		parts = append(parts, text)
	}
	suffix := " ◷"
	if measured {
		suffix = " ≈◷"
	}
	return strings.Join(parts, metricStyle.Render(":")) + metricStyle.Render(suffix)
}

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
	width = max(1, width-2)
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
			value, scale := compactMetric(n)
			parts = append(parts, value+scale+field.symbol)
		}
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
		parts = append([]string{clockMetric(duration, measured)}, parts...)
	}
	if n, hasCost := root.number("total_cost_usd", "costUSD", "costUsd"); hasCost {
		idx := 0
		if ok {
			idx = 1
		}
		parts = append(parts[:idx], append([]string{costMetric(n)}, parts[idx:]...)...)
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
	// Wrap at fixed-cell boundaries; never shift symbols with numeric magnitude.
	row := ""
	flush := func() {
		if row != "" {
			lines = append(lines, metricStyle.Render(strings.TrimRight(row, " ")))
			row = ""
		}
	}
	for _, part := range parts {
		next := part
		if row != "" {
			next = row + " " + part
		}
		if ansi.StringWidth(next) > width {
			flush()
			next = part
		}
		row = clip(next, width)
	}
	flush()
	return indentBlock(strings.Join(lines, "\n"))
}
