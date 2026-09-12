package tui

import (
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
)

// Style only after sanitizing remote text. State events stay in the journal,
// but the transcript omits them: the live status line is their presentation.
func eventView(s *api.Session, e *api.Event, width int) (out string) {
	if e.Kind != "input" && e.Kind != "turn_end" {
		width = max(1, width-2)
		defer func() { out = indentBlock(out) }()
	}
	wrap := func(text string) string { return ansi.Hardwrap(safeText(text), max(1, width), true) }
	switch e.Kind {
	case "state":
		return ""
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
		return style.Render(name) + "\n" + answer.Render(wrap(e.Text))
	case "approval":
		return warning.Render(wrap("APPROVAL " + e.Text + " [" + e.RequestId + "]\n" + string(e.Payload)))
	case "approval_resolved":
		return teal.Render(wrap("approval: " + e.Text))
	case "tool_call":
		return lavender.Render(wrap("tool › " + e.Text + " " + string(e.Payload)))
	case "tool_result":
		return blue.Render(wrap("result › " + string(e.Payload)))
	case "turn_end":
		return turnSummary(e, nil, 0, width)
	case "diagnostic", "stderr":
		return peach.Render(wrap("diagnostic › " + e.Text + " " + string(e.Payload)))
	}
	return ""
}
