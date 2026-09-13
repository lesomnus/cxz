package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/agentview"
)

func toolActivityView(activity agentview.ToolActivity, result *api.Event, width int) string {
	return indentBlock(toolActivityBody(activity, result, width))
}

func toolActivityBody(activity agentview.ToolActivity, result *api.Event, width int) string {
	prefix := "◇"
	status := "requested"
	if result != nil {
		prefix = "↳"
		status = "done"
		root := fields(result.Payload)
		if string(root["is_error"]) == "true" {
			status = "failed"
		}
		if state := root.object("item").text("status"); state != "" {
			status = state
		}
	}
	var lines []string
	if activity.Kind == "command" {
		text := activity.Description
		if text == "" {
			text = activity.Command
		}
		text = strings.SplitN(text, "\n", 2)[0]
		lines = append(lines, clip(safeText(prefix+" run · "+text), max(1, width-2)))
	} else {
		for _, f := range activity.Files {
			path := f.Path
			if path == "" {
				path = "(path not reported)"
			}
			line := prefix + " " + f.Action + " · " + path
			if f.MovePath != "" {
				line += " → " + f.MovePath
			}
			switch f.Measure {
			case "content":
				line += fmt.Sprintf(" · %d lines supplied", f.Lines)
			case "replacement":
				line += fmt.Sprintf(" · −%d +%d replacement lines", f.Removed, f.Added)
			case "diff":
				line += fmt.Sprintf(" · −%d +%d lines", f.Removed, f.Added)
			case "unknown":
				line += " · change count unavailable"
			}
			if f.PerMatch {
				line += " · per match; total unknown"
			}
			line += " · " + status
			lines = append(lines, line)
		}
	}
	if len(lines) == 0 {
		lines = append(lines, prefix+" files · "+status+" · details not reported")
	}
	style := lavender
	if result != nil {
		style = muted
	}
	for i, line := range lines {
		lines[i] = style.Render(ansi.Wrap(safeText(line), max(1, width-2), ""))
	}
	return strings.Join(lines, "\n")
}
