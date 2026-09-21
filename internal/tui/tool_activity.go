package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/agentview"
)

// Tool events are immutable journal entries. Cache by event identity and the
// context that changes their presentation, keeping live output outside the cache.
type toolActivityKey struct {
	event *api.Event
	agent string
}

type cachedToolActivity struct {
	activity agentview.ToolActivity
	ok       bool
}

type toolRenderKey struct {
	event, result *api.Event
	agent, state  string
	width         int
	background    bool
}

func (m *model) cachedToolView(agent string, e *api.Event) (agentview.ToolActivity, bool) {
	key := toolActivityKey{e, agent}
	if cached, ok := m.toolActivities[key]; ok {
		return cached.activity, cached.ok
	}
	activity, ok := agentview.ToolView(agent, e.Text, e.Payload)
	if m.toolActivities == nil {
		m.toolActivities = map[toolActivityKey]cachedToolActivity{}
	}
	m.toolActivities[key] = cachedToolActivity{activity, ok}
	return activity, ok
}

func (m *model) cachedToolBody(agent string, e *api.Event, activity agentview.ToolActivity, result *api.Event, width int, state string, background bool) string {
	if result != nil && !background {
		state = "" // A completed result determines its own status.
	}
	key := toolRenderKey{e, result, agent, state, width, background}
	if body, ok := m.renderedTools[key]; ok {
		return body
	}
	if background {
		result = nil
	}
	body := toolActivityStateBody(activity, result, width, state)
	if background {
		const badge = " · background"
		rows := strings.Split(body, "\n")
		last := len(rows) - 1
		rows[last] = clip(rows[last], max(1, width-2-ansi.StringWidth(badge))) + badge
		body = strings.Join(rows, "\n")
	}
	if m.renderedTools == nil {
		m.renderedTools = map[toolRenderKey]string{}
	}
	m.renderedTools[key] = body
	return body
}

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
	case "failed", "denied", "declined", "canceled", "cancelled", "stopped", "interrupted":
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
		header := prefix + " Bash · "
		// Reserve the transcript indent and the entire header before wrapping.
		// Wrapping again after adding the header can turn two preview rows into
		// three and leave an operator by itself on the extra row.
		commandWidth := max(1, width-2-ansi.StringWidth(header))
		rows := strings.Split(ansi.Wrap(highlightCode(text, "bash"), commandWidth, ""), "\n")
		for i, row := range rows[:min(2, len(rows))] {
			if i == 1 && len(rows) > 2 {
				row = ansi.Truncate(row, max(0, commandWidth-1), "") + "…"
			}
			if i == 0 {
				lines = append(lines, header+row)
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
		if activity.Kind == "command" {
			lines[i] = clip(line, max(1, width-2))
		} else {
			lines[i] = ansi.Wrap(line, max(1, width-2), "")
		}
	}
	return strings.Join(lines, "\n")
}
