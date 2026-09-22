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

// pulseTimer ticks every 100 ms: one complete sweep takes two seconds.
const historySkeletonCycleTicks = 20

type historyShimmer struct {
	id         string
	cycleStart int
}

func (m *model) historySkeletonView() (string, bool) {
	s := m.current()
	if s == nil || m.watchID != s.Id || len(m.pendingInputs[s.Id]) > 0 {
		m.historyShimmer = nil
		return "", false
	}
	if _, help := m.localHelp[s.Id]; help {
		m.historyShimmer = nil
		return "", false
	}
	if m.historyShimmer != nil && m.historyShimmer.id != s.Id {
		m.historyShimmer = nil
	}
	if m.historyShimmer == nil {
		if !m.historyOpening[s.Id] {
			return "", false
		}
		// Start at the first visible frame, independent of the global spinner.
		m.historyShimmer = &historyShimmer{id: s.Id, cycleStart: m.pulse}
	}
	elapsed := m.pulse - m.historyShimmer.cycleStart
	if elapsed >= historySkeletonCycleTicks {
		if !m.historyOpening[s.Id] {
			m.historyShimmer = nil
			return "", false
		}
		m.historyShimmer.cycleStart += elapsed / historySkeletonCycleTicks * historySkeletonCycleTicks
		elapsed %= historySkeletonCycleTicks
	}
	return historySkeleton(m.width, m.view.Height, elapsed), true
}

func (m *model) conversationView() string {
	if skeleton, visible := m.historySkeletonView(); visible {
		return skeleton
	}
	m.acknowledgeSession()
	rows := strings.Split(m.view.View(), "\n")
	// Blink only the dot in visible running-tool headers. The cached transcript,
	// its geometry and tool previews stay unchanged between animation frames.
	if m.pulse/5%2 == 1 {
		for i, row := range rows {
			if m.workingToolRows[m.view.YOffset+i] {
				rows[i] = strings.Replace(row, "[•]", "[ ]", 1)
			}
		}
	}
	promptRows := map[int]bool{}
	for _, span := range m.promptSpans {
		for row := max(0, span.start-m.view.YOffset); row < min(len(rows), span.end-m.view.YOffset); row++ {
			promptRows[row] = true
		}
	}
	if m.activeWork() && m.view.AtBottom() && len(rows) > 0 {
		// render reserves a final transcript row for this transient indicator.
		index := min(len(rows)-1, max(0, len(m.historyTimes)-m.view.YOffset-1))
		rows[index] = indentBlock(accent.Render(workingSpinner(m.pulse)) + " " + muted.Render(clip(m.workingLabel(time.Now()), max(1, m.view.Width-4))))
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
	m.codeButtonView(rows, promptRows)
	m.selectionView(rows)
	return strings.Join(rows, "\n")
}

func historySkeleton(width, height, pulse int) string {
	rows := make([]string, max(0, height))
	inner := max(1, min(64, width-4))
	if len(rows) > 0 {
		rows[0] = "  " + muted.Render(clip("Loading conversation…", inner))
	}
	color := lipgloss.ColorProfile().Name() != "Ascii"
	front, tail := 4.0, max(14.0, float64(inner)*0.4)
	phase := float64(pulse%historySkeletonCycleTicks) / float64(historySkeletonCycleTicks-1)
	head := phase*(float64(inner)+front+tail) - front
	// A fixed ordered pattern avoids random flicker as the light passes over it.
	dither := [4][4]int{{0, 8, 2, 10}, {12, 4, 14, 6}, {3, 11, 1, 9}, {15, 7, 13, 5}}
	shades := [...]rune{' ', '░', '▒', '▓', '█'}
	// One short paragraph at the top; leave the rest of the viewport empty.
	for row, percent := range []int{90, 75, 50} {
		y := row + 2
		if y >= len(rows) {
			break
		}
		var line strings.Builder
		line.WriteString("  ")
		lastFG, lastBG := -1, -1
		for x := 0; x < max(1, inner*percent/100); x++ {
			distance := head - float64(x)
			span := tail
			if distance < 0 {
				span = front
			}
			// Smooth both ends, with a much longer fade behind the moving peak.
			level := max(0.0, 1-math.Abs(distance)/span)
			level = level * level * (3 - 2*level)
			coverage := min(4, int(level*4+(float64(dither[row%4][x%4])+0.5)/16))
			glyph := shades[coverage]
			if color {
				fg := 38 + int(math.Round(58*level))
				bg := 38 + int(math.Round(29*level))
				if fg != lastFG || bg != lastBG {
					// Emit RGB directly: SSH often reports ANSI256 even when the
					// terminal supports the intermediate shades needed for this fade.
					fmt.Fprintf(&line, "\x1b[38;2;%d;%d;%dm\x1b[48;2;%d;%d;%dm", fg, fg, fg, bg, bg, bg)
					lastFG, lastBG = fg, bg
				}
			} else if glyph == ' ' {
				glyph = '░'
			}
			line.WriteRune(glyph)
		}
		if color {
			line.WriteString("\x1b[0m")
		}
		rows[y] = line.String()
	}
	return screen(strings.Join(rows, "\n"), width, height)
}
