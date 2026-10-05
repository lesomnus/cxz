package tui

import (
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
	"github.com/rivo/uniseg"
)

// Decorate cells after layout: soft wraps are not source newlines, and visible
// whitespace must never change the draft, wrapping, chip labels or cursor SGR.
func (m *model) composerDisplay(view string) string {
	layout := m.composerRows()
	offset := m.composerScroll(layout)
	rows := strings.Split(view, "\n")
	value := m.input.Value()
	var source []rune
	var hidden []bool
	if m.showInputWhitespace {
		source = []rune(value)
		hidden = make([]bool, len(source))
		for token := range m.pastes {
			if token == "" {
				continue
			}
			for start := 0; start < len(value); {
				i := strings.Index(value[start:], token)
				if i < 0 {
					break
				}
				i += start
				a := utf8.RuneCountInString(value[:i])
				for j := a; j < a+utf8.RuneCountInString(token); j++ {
					hidden[j] = true
				}
				start = i + len(token)
			}
		}
	}
	for y := range rows {
		cells := map[int]string{0: " ", 1: " "}
		if index := y + offset; index < len(layout) {
			row := layout[index]
			if row.column == 0 {
				if row.line == 0 {
					cells[0] = "❯"
				} else {
					cells[0] = strconv.Itoa(row.line % 10)
				}
			}
			if m.showInputWhitespace {
				x, pos := 2, row.start
				g := uniseg.NewGraphemes(string(source[row.start:row.end]))
				for g.Next() {
					text := g.Str()
					if text == " " && !hidden[pos] {
						cells[x] = "·"
					}
					x += g.Width()
					pos += utf8.RuneCountInString(text)
				}
				// Only real newline characters get a mark, never a soft wrap or EOF.
				if row.end < len(source) && source[row.end] == '\n' {
					cells[x] = "↵"
				}
			}
		}
		rows[y] = replaceComposerCells(rows[y], cells)
	}
	return strings.Join(rows, "\n")
}

func replaceComposerCells(text string, cells map[int]string) string {
	var out strings.Builder
	state := byte(0)
	column := 0
	for len(text) > 0 {
		seq, width, n, next := ansi.DecodeSequence(text, state, nil)
		if n == 0 {
			break
		}
		if replacement, ok := cells[column]; ok && width == 1 {
			out.WriteString(replacement)
		} else {
			out.WriteString(seq)
		}
		column += width
		text, state = text[n:], next
	}
	return out.String()
}
