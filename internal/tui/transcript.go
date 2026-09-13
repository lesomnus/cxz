package tui

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/agentview"
)

// Style only after sanitizing remote text. State events stay in the journal,
// but the transcript omits them: the live status line is their presentation.
func eventView(s *api.Session, e *api.Event, width int) (out string) {
	if e.Kind != "input" && e.Kind != "turn_end" {
		width = max(1, width-2)
		if e.Kind != "assistant" {
			defer func() { out = indentBlock(out) }()
		}
	}
	wrap := func(text string) string { return ansi.Hardwrap(safeText(text), max(1, width), true) }
	switch e.Kind {
	case "state":
		return ""
	case "setting":
		var v struct{ Value string }
		_ = json.Unmarshal(e.Payload, &v)
		if v.Value == "" {
			v.Value = "default"
		}
		return teal.Render(wrap("✓ " + e.Text + " · " + v.Value))
	case "setting_status":
		return muted.Render(wrap("Settings · " + e.Text))
	case "input":
		stamp := "-- -- --:--"
		if e.TimeMs > 0 {
			stamp = time.UnixMilli(e.TimeMs).Local().Format("01-02 15:04")
		}
		text := strings.Split(ansi.Hardwrap(safeText(e.Text), max(1, width-2), true), "\n")
		for i := range text {
			prefix := "  "
			if i == 0 {
				prefix = "> "
			}
			text[i] = blue.Render(prefix + text[i])
		}
		return timestamp.Render("  "+stamp) + "\n" + strings.Join(text, "\n")
	case "assistant":
		name := strings.ToUpper(pickerLabel(safeText(s.Agent)))
		style := lavender.Bold(true)
		switch s.Agent {
		case "claude":
			style = claude
		case "codex":
			style = codex
		}
		if name == "" {
			name = "AGENT"
		}
		return style.Render("•") + " " + style.Render(name) + "\n" + indentBlock(markdownView(e.Text, width))
	case "approval":
		return strings.TrimPrefix(approvalLine(s, e, "requested", width+2), "  ")
	case "approval_resolved":
		return ""
	case "tool_call":
		if question(e) {
			return lavender.Render("? Question · open /answer to choose")
		}
		return lavender.Render(wrap("tool › " + e.Text + " " + string(e.Payload)))
	case "tool_result":
		return muted.Render(clip(toolResultSummary(e), width))
	case "compact":
		return teal.Render(wrap("◇ Context compacted · journal retained"))
	case "turn_end":
		return turnSummary(e, nil, 0, width)
	case "diagnostic", "stderr":
		return peach.Render(wrap("diagnostic › " + e.Text + " " + string(e.Payload)))
	}
	return ""
}

func approvalLine(s *api.Session, e *api.Event, state string, width int) string {
	icon, style := "☐", warning
	switch state {
	case "allowed":
		icon, style = "🗹", accent
	case "denied":
		icon, style = "☒", peach
	case "canceled":
		icon, style = "☒", muted
	}
	title := agentview.ApprovalView(s.Agent, e.Text, nil).Title
	if question(e) {
		if state == "allowed" {
			state = "answered"
		}
		return indentBlock(style.Render(ansi.Hardwrap(fmt.Sprintf("%s question %s", icon, state), max(1, width-2), true)))
	}
	return indentBlock(style.Render(ansi.Hardwrap(fmt.Sprintf("%s approval %-9s · %s", icon, state, safeText(title)), max(1, width-2), true)))
}

func toolResultSummary(e *api.Event) string {
	root := fields(e.Payload)
	item := root.object("item")
	text := e.Text
	content := e.Payload
	if raw, ok := root["content"]; ok {
		content = raw
	}
	if text == "" {
		_ = json.Unmarshal(content, &text)
	}
	if text == "" {
		text = item.text("aggregatedOutput")
	}
	if text == "" {
		var blocks []struct {
			Text string `json:"text"`
		}
		_ = json.Unmarshal(content, &blocks)
		for _, b := range blocks {
			text += b.Text + "\n"
		}
	}
	status := "done"
	if item.text("status") != "" {
		status = item.text("status")
	}
	if string(root["is_error"]) == "true" {
		status = "failed"
	}
	if code, ok := item.number("exitCode"); ok {
		status += fmt.Sprintf(" · exit %.0f", code)
	}
	count := len(strings.Split(strings.TrimRight(text, "\n"), "\n"))
	if text == "" {
		count = 0
	}
	size := len(e.Payload)
	preview := strings.TrimSpace(strings.SplitN(safeText(text), "\n", 2)[0])
	result := fmt.Sprintf("↳ %s · %d lines · %sB · /details", status, count, humanCount(float64(size)))
	if preview != "" {
		result += " · " + preview
	}
	return result
}

func (m *model) toolDetails() {
	if s := m.current(); s != nil {
		events := m.events[s.Id]
		for i := len(events) - 1; i >= 0; i-- {
			e := events[i]
			if e.Kind != "tool_result" {
				continue
			}
			var value any
			text := string(e.Payload)
			if json.Unmarshal(e.Payload, &value) == nil {
				b, _ := json.MarshalIndent(value, "", "  ")
				text = string(b)
			}
			m.recordLocal("/details", fmt.Sprintf("Tool result · event %d\n%s\n%s", e.Seq, e.Text, text))
			return
		}
	}
	m.recordLocal("/details", "No tool result in the received history yet.")
}
