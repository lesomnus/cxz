package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
)

func (m *model) scrollStatus() string {
	start := m.view.YOffset
	stamp := "local"
	for i := start; i < min(len(m.historyTimes), start+m.view.Height); i++ {
		if m.historyTimes[i] > 0 {
			stamp = time.UnixMilli(m.historyTimes[i]).Local().Format("01-02 15:04:05")
			break
		}
	}
	return fmt.Sprintf("L %d–%d/%d · %s · Ctrl+End latest", start+1, min(len(m.historyTimes), start+m.view.Height), len(m.historyTimes), stamp)
}

func (m *model) conversationView() string {
	rows := strings.Split(m.view.View(), "\n")
	if s := m.current(); s != nil && s.State == "working" && m.view.AtBottom() && len(rows) > 0 {
		frames := []rune("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏")
		// render reserves a final transcript row for this transient indicator.
		index := min(len(rows)-1, max(0, len(m.historyTimes)-m.view.YOffset-1))
		rows[index] = indentBlock(accent.Render(string(frames[m.pulse%len(frames)])))
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
