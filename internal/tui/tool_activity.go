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
	event        *api.Event
	agent, state string
	width        int
	background   bool
}

// cachedToolBody draws a tool row. The row is the cache key: a row is published
// again when its state moves, so a new version is a new pointer and the old
// drawing is not reused for it.
//
// What used to be here was a pairing: a call, a result, and an approval decision
// reduced into a state. The runtime has already done that -- see cxz#155 -- so
// state arrives on the row, and the only thing still decided here is what a
// background task overrides it with.
func (m *model) cachedToolBody(agent string, e *api.Event, width int, state string, background bool) string {
	key := toolRenderKey{e, agent, state, width, background}
	if body, ok := m.renderedTools[key]; ok {
		return body
	}
	body := toolSummaryBody(e.GetToolSummary(), width, state)
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

// toolSummaryBody draws what a row says a tool is. The four states are the
// projection's own -- pending, working, completed, failed -- and a background
// task's status passes through the same switch.
//
// Nothing is derived here. The exit code, the is_error flag and the provider's
// status strings used to be read in this function from a result record; the
// runtime reads them now, and a reader that read them again could disagree
// with the row it was drawing. See cxz#155.
func toolSummaryBody(label *api.ToolSummary, width int, status string) string {
	activity := agentview.ToolActivity{
		Kind:        label.GetKind(),
		Command:     label.GetCommand(),
		Description: label.GetDescription(),
		Shell:       label.GetShell(),
	}
	for _, f := range label.GetFiles() {
		activity.Files = append(activity.Files, agentview.FileActivity{
			Path: f.GetPath(), MovePath: f.GetMovePath(), Action: f.GetAction(),
			Added: int(f.GetAdded()), Removed: int(f.GetRemoved()), Lines: int(f.GetLines()),
			Measure: f.GetMeasure(), PerMatch: f.GetPerMatch(),
		})
	}
	marker, style := "○", warning
	switch status {
	case "working", "inProgress":
		marker, style = "•", accent
	case "pending", "requested":
		marker, style = "○", warning
	case "done", "completed":
		marker, style = "✓", accent
	case "failed", "denied", "declined", "canceled", "cancelled", "stopped", "interrupted":
		marker, style = "×", failure
	case "ended":
		// A background job that is no longer running and never said how it
		// went. Neither circle fits -- it is not waiting and not working -- and
		// neither judgement is ours to make: codex reports a finished job by
		// listing no tasks, which says it ended and nothing else. A square says
		// that a different kind of thing finished, unjudged.
		marker, style = "▫", muted
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
		// Name the interpreter the script was handed to, so the row can spend the
		// rest of its width on the script rather than on the wrapper.
		if activity.Shell != "" {
			header += muted.Render(activity.Shell) + " · "
		}
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
				lines = append(lines, "  "+row)
			}
		}
	} else if activity.Kind == "tool" {
		text := activity.Description
		if text == "" {
			text = label.GetName()
		}
		lines = append(lines, prefix+" "+safeText(text))
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
	if omitted := label.GetOmittedFiles(); omitted > 0 {
		lines = append(lines, muted.Render(fmt.Sprintf("  %d more files · /details", omitted)))
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
