package tui

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
)

// Physical total lines depend on width/Markdown and cannot be known without
// rendering every page. Journal coordinates provide a stable logical range:
// prepending history does not change the coordinate of the visible event.
func (m *model) scrollTrack() string {
	width := max(1, m.width)
	end := max(0, m.view.TotalLineCount()-m.view.Height)
	position := 0
	if end > 0 {
		position = int(math.Round(float64(min(max(0, m.view.YOffset), end)) * float64(width-1) / float64(end)))
	}
	if s := m.current(); s != nil && len(m.historyPositions) == m.view.TotalLineCount() && len(m.historyPositions) > 0 {
		total := max(s.LastSeq, m.cursor[s.Id])
		if total > 0 {
			coordinate := m.historyPositions[min(max(0, m.view.YOffset), len(m.historyPositions)-1)]
			position = min(width-1, max(0, int(math.Round(coordinate/float64(total)*float64(width-1)))))
			if m.view.AtBottom() {
				position = width - 1
			} else if m.view.AtTop() && m.historyStart[s.Id] == 0 {
				position = 0
			}
		}
	}
	return muted.Render(strings.Repeat("─", position)) + muted.Render("◆︎") + zeroStyle.Render(strings.Repeat("─", width-position-1))
}

func (m *model) scrollStatus() string {
	start := m.view.YOffset
	stamp := "local"
	for i := start; i < min(len(m.historyTimes), start+m.view.Height); i++ {
		if m.historyTimes[i] > 0 {
			stamp = time.UnixMilli(m.historyTimes[i]).Local().Format("01-02 15:04:05")
			break
		}
	}
	label := "L"
	if s := m.current(); s != nil && m.historyStart[s.Id] > 0 {
		label = "loaded L"
	}
	return fmt.Sprintf("%s %d–%d/%d · %s · track: journal · Ctrl+End latest", label, start+1, min(len(m.historyTimes), start+m.view.Height), len(m.historyTimes), stamp)
}

func (m *model) conversationView() string {
	rows := strings.Split(m.view.View(), "\n")
	if s := m.current(); s != nil && s.State == "working" && m.view.AtBottom() && len(rows) > 0 {
		frames := []rune("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏")
		// render reserves a final transcript row for this transient indicator.
		index := min(len(rows)-1, max(0, len(m.historyTimes)-m.view.YOffset-1))
		rows[index] = indentBlock(accent.Render(string(frames[m.pulse%len(frames)])) + " " + muted.Render(clip(m.workingLabel(time.Now()), max(1, m.width-4))))
	}
	if m.latestPrompt != "" && m.lastPromptEnd > 0 && m.lastPromptEnd <= m.view.YOffset {
		prompt := strings.Split(ansi.Hardwrap(safeText(m.latestPrompt), max(1, m.width-2), true), "\n")
		for i := 0; i < min(2, min(len(prompt), len(rows))); i++ {
			prefix := "  "
			if i == 0 {
				prefix = "> "
			}
			text := prefix + prompt[i]
			if i == 1 && len(prompt) > 2 {
				text = clip(text+" …", m.width)
			}
			text = clip(text, m.width)
			rows[i] = pinnedPrompt.Render(text + strings.Repeat(" ", max(0, m.width-ansi.StringWidth(text))))
		}
	}
	return strings.Join(rows, "\n")
}
