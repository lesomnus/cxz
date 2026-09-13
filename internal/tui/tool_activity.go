package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/agentview"
)

func toolActivityView(activity agentview.ToolActivity, result *api.Event, width int) string {
	return indentBlock(toolActivityBody(activity, result, width))
}

func toolActivityBody(activity agentview.ToolActivity, result *api.Event, width int) string {
	return toolActivityStateBody(activity, result, width, "pending")
}

// Claude tool_use announces an intention, not execution. In particular, other
// tools in the same message may still be queued behind an approval. Only use
// a native execution state (or a correlated approval decision in render).
func toolInitialState(provider string, e *api.Event) string {
	if provider == "codex" && fields(e.Payload).object("item").text("status") == "inProgress" {
		return "working"
	}
	return "pending"
}

func toolActivityStateBody(activity agentview.ToolActivity, result *api.Event, width int, status string) string {
	if result != nil {
		status = "done"
		root := fields(result.Payload)
		if string(root["is_error"]) == "true" {
			status = "failed"
		}
		if state := root.object("item").text("status"); state != "" {
			status = state
		}
		if code, ok := root.object("item").number("exitCode"); ok && code != 0 {
			status = "failed"
		}
	}
	marker, style := "[ ]", warning
	switch status {
	case "working", "inProgress":
		marker, style = "[•]", accent
	case "pending", "requested":
		marker, style = "[ ]", warning
	case "done", "completed":
		marker, style = "[✓]", accent
	case "failed", "denied", "declined", "canceled", "interrupted":
		marker, style = "[×]", failure
	}
	prefix := style.Render(marker)
	positive := func(n int) string { return accent.Render(fmt.Sprintf("+%d", n)) }
	negative := func(n int) string {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#F49BAA")).Render(fmt.Sprintf("-%d", n))
	}
	var lines []string
	if activity.Kind == "command" {
		text := activity.Command
		if text == "" {
			text = activity.Description
		}
		rows := strings.Split(ansi.Wrap(safeText(text), max(1, width-12), ""), "\n")
		for i, row := range rows[:min(2, len(rows))] {
			if i == 1 && len(rows) > 2 {
				row = clip(row, max(1, width-14)) + "…"
			}
			if i == 0 {
				lines = append(lines, prefix+" Bash · "+row)
			} else {
				lines = append(lines, "    "+row)
			}
		}
	} else if activity.Kind == "tool" {
		lines = append(lines, prefix+" "+safeText(activity.Description))
	} else {
		for _, f := range activity.Files {
			path := f.Path
			if path == "" {
				path = "(path not reported)"
			}
			action := map[string]string{"write": "Write", "edit": "Edit", "update": "Edit", "add": "Write", "delete": "Delete", "read": "Read"}[f.Action]
			if action == "" {
				action = f.Action
			}
			line := prefix + " " + safeText(action) + " " + safeText(path)
			if f.MovePath != "" {
				line += " → " + safeText(f.MovePath)
			}
			switch f.Measure {
			case "content":
				line += " · " + positive(f.Lines) + " content"
			case "replacement":
				line += " · " + positive(f.Added) + " " + negative(f.Removed)
			case "diff":
				line += " · " + positive(f.Added) + " " + negative(f.Removed)
			case "unknown":
				line += " · Δ?"
			}
			if f.PerMatch {
				line += " /match"
			}
			lines = append(lines, line)
		}
	}
	if len(lines) == 0 {
		lines = append(lines, prefix+" Files · Δ?")
	}
	for i, line := range lines {
		lines[i] = ansi.Wrap(line, max(1, width-2), "")
	}
	return strings.Join(lines, "\n")
}
