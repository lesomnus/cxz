package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	extast "github.com/yuin/goldmark/extension/ast"
)

// Restore the requested indexed background after nested syntax/style resets.
// Keep indexed colors even on truecolor terminals: their palette is user-owned.
func indexedBackground(line string, index int) string {
	if lipgloss.ColorProfile().Name() == "Ascii" {
		return line
	}
	background := fmt.Sprintf("\x1b[48;5;%dm", index)
	return background + panelStyleSequence.ReplaceAllStringFunc(line, func(style string) string { return style + background }) + "\x1b[0m"
}

func markdownTable(rows [][]string, alignments []extast.Alignment, width int) string {
	if len(rows) == 0 || len(alignments) == 0 {
		return ""
	}
	width = max(1, width)
	var output []string
	// On very narrow screens, repeat the header in column groups so no cell is lost.
	groupSize := max(1, (width+1)/3)
	for start := 0; start < len(alignments); start += groupSize {
		end := min(len(alignments), start+groupSize)
		widths := make([]int, end-start)
		for i := range widths {
			widths[i] = min(2, width)
		}
		for _, row := range rows {
			for col := start; col < min(end, len(row)); col++ {
				for _, line := range strings.Split(row[col], "\n") {
					widths[col-start] = max(widths[col-start], min(width, ansi.StringWidth(line)))
				}
			}
		}
		total := len(widths) - 1
		for _, w := range widths {
			total += w
		}
		for total > width {
			largest := 0
			for i, w := range widths {
				if w > widths[largest] {
					largest = i
				}
			}
			widths[largest]--
			total--
		}
		if start > 0 {
			output = append(output, "")
		}
		for rowIndex, row := range rows {
			cells := make([][]string, len(widths))
			height := 1
			for i, w := range widths {
				value := ""
				if start+i < len(row) {
					value = strings.ReplaceAll(row[start+i], "\t", "    ")
				}
				cells[i] = strings.Split(ansi.Hardwrap(value, w, true), "\n")
				height = max(height, len(cells[i]))
			}
			for lineIndex := 0; lineIndex < height; lineIndex++ {
				parts := make([]string, len(widths))
				for i, w := range widths {
					line := ""
					if lineIndex < len(cells[i]) {
						line = cells[i][lineIndex]
					}
					padding := max(0, w-ansi.StringWidth(line))
					left := 0
					switch alignments[start+i] {
					case extast.AlignRight:
						left = padding
					case extast.AlignCenter:
						left = padding / 2
					}
					parts[i] = strings.Repeat(" ", left) + line + strings.Repeat(" ", padding-left)
				}
				line := strings.Join(parts, " ")
				if rowIndex == 0 {
					line = strong.Render(line)
				}
				output = append(output, line)
			}
			if rowIndex == 0 || rowIndex < len(rows)-1 {
				glyph, color := "─", 240
				if rowIndex == 0 {
					glyph, color = "━", 22
				}
				parts := make([]string, len(widths))
				for i, w := range widths {
					parts[i] = strings.Repeat(glyph, w)
					if lipgloss.ColorProfile().Name() != "Ascii" {
						parts[i] = fmt.Sprintf("\x1b[38;5;%dm%s\x1b[0m", color, parts[i])
					}
				}
				output = append(output, strings.Join(parts, " "))
			}
		}
	}
	return strings.Join(output, "\n")
}
