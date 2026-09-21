package tui

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
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
		// The last reachable TOP row is the endpoint, not the final event.
		// Rows already visible below it cannot be scrolled past the viewport.
		// Using the journal tail here makes a tiny upward scroll jump left by
		// an entire screen (or further when the tail contains hidden events).
		endCoordinate := m.historyPositions[min(end, len(m.historyPositions)-1)]
		if endCoordinate > 0 {
			coordinate := m.historyPositions[min(max(0, m.view.YOffset), len(m.historyPositions)-1)]
			position = min(width-1, max(0, int(math.Round(coordinate/endCoordinate*float64(width-1)))))
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
	loading := ""
	if s := m.current(); s != nil && m.historyLoading[s.Id] > 0 {
		loading = " · loading earlier…"
	}
	return fmt.Sprintf("%s %d–%d/%d%s · %s · track: journal · Ctrl+End latest", label, start+1, min(len(m.historyTimes), start+m.view.Height), len(m.historyTimes), loading, stamp)
}

type promptSpan struct {
	start, end int
	text       string
}

func (m *model) conversationView() string {
	if s := m.current(); s != nil && m.historyOpening && m.watchID == s.Id && len(m.events[s.Id]) == 0 && len(m.pendingInputs[s.Id]) == 0 {
		if _, help := m.localHelp[s.Id]; !help {
			return historySkeleton(m.width, m.view.Height, m.pulse)
		}
	}
	rows := strings.Split(m.view.View(), "\n")
	promptRows := map[int]bool{}
	for _, span := range m.promptSpans {
		for row := max(0, span.start-m.view.YOffset); row < min(len(rows), span.end-m.view.YOffset); row++ {
			promptRows[row] = true
		}
	}
	if m.activeWork() && m.view.AtBottom() && len(rows) > 0 {
		frames := []rune("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏")
		// render reserves a final transcript row for this transient indicator.
		index := min(len(rows)-1, max(0, len(m.historyTimes)-m.view.YOffset-1))
		rows[index] = indentBlock(accent.Render(string(frames[m.pulse%len(frames)])) + " " + muted.Render(clip(m.workingLabel(time.Now()), max(1, m.view.Width-4))))
	}
	pinned := ""
	for _, span := range m.promptSpans {
		if span.end <= m.view.YOffset {
			pinned = span.text
		} else {
			break
		}
	}
	if pinned != "" && !m.selectingTools() {
		prompt := strings.Split(ansi.Hardwrap(safeText(pinned), max(1, m.view.Width-2), true), "\n")
		for i := 0; i < min(2, min(len(prompt), len(rows))); i++ {
			prefix := "  "
			if i == 0 {
				prefix = "> "
			}
			text := prefix + prompt[i]
			if i == 1 && len(prompt) > 2 {
				text = clip(text+" …", m.view.Width)
			}
			text = clip(text, m.view.Width)
			rows[i] = blue.Render(text)
			promptRows[i] = true
		}
	}
	m.toolSelectorView(rows)
	for i, row := range rows {
		rows[i] = row + strings.Repeat(" ", max(0, m.width-ansi.StringWidth(row)))
		if promptRows[i] {
			rows[i] = indexedBackground(rows[i], 236)
		}
	}
	return strings.Join(rows, "\n")
}

func historySkeleton(width, height, pulse int) string {
	rows := make([]string, max(0, height))
	inner := max(1, width-4)
	band := max(3, inner/6)
	position := (pulse*3)%(inner+band) - band
	for y := range rows {
		if y == 0 {
			rows[y] = "  " + muted.Render(clip("Loading conversation…", inner))
			continue
		}
		part := (y - 1) % 7
		if part == 0 || part == 6 {
			continue
		}
		length := max(1, inner*[]int{0, 25, 90, 75, 85, 50, 0}[part]/100)
		left := min(length, max(0, position))
		right := min(length, max(left, position+band))
		if lipgloss.ColorProfile().Name() == "Ascii" {
			rows[y] = "  " + strings.Repeat("░", left) + strings.Repeat("▒", right-left) + strings.Repeat("░", length-right)
		} else {
			rows[y] = "  " + indexedBackground(strings.Repeat(" ", left), 235) + indexedBackground(strings.Repeat(" ", right-left), 237) + indexedBackground(strings.Repeat(" ", length-right), 235)
		}
	}
	return screen(strings.Join(rows, "\n"), width, height)
}
