package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// A find bar that moves the transcript to a match but does not say which word
// it matched leaves the last step to the reader. So the matches on screen are
// painted, in the two greens that already mean this elsewhere: the bright one
// for the match the transcript is on, because that is what "you are here" wears
// everywhere else, and the quiet one for the rest.
//
// This runs on the rows that are about to be drawn, not on the transcript that
// was rendered into the window. The rendered window is cached per event and
// costs Markdown to rebuild; repainting it on every keystroke would make typing
// in the bar as expensive as receiving the conversation. A screen is fifty rows,
// and this is the only place that knows which fifty.

// A row of nothing but matches is a row of noise, and a pathological query
// should not be able to make painting one unbounded work either.
const highlightsPerRow = 32

// searchHighlightView paints the visible matches in place.
func (m *model) searchHighlightView(rows []string) {
	p := m.search
	if p == nil {
		return
	}
	// Matching here is the bar's own, not the server's: the bar only ever asks
	// for a case-insensitive substring, so what it paints is what it asked for,
	// and it can paint it before the answer arrives. There is no minimum length
	// either -- a single letter painting half the screen is what was typed.
	query := p.query
	if strings.TrimSpace(query) == "" {
		return
	}
	current := uint64(0)
	if p.scope == scopeSession && p.index < len(p.matches) {
		current = p.matches[p.index].seq
	}
	for i, row := range rows {
		content := m.view.YOffset + i
		style := searchOtherMatch
		if current > 0 && content >= 0 && content < len(m.historyPositions) && uint64(m.historyPositions[content]) == current {
			style = searchCurrentMatch
		}
		rows[i] = highlightRow(row, query, style)
	}
}

// highlightRow paints every occurrence of query in one already-styled row.
//
// The row is styled text, so the match cannot be found in it directly: it is
// located in the row's plain text, measured in display columns -- which is what
// the terminal and the cut below both count in, and what makes this work for
// text that is two columns wide per character -- and the row is rebuilt from the
// pieces either side. Cutting re-states the style each piece was under, so the
// row's own colours survive having a hole punched in it.
func highlightRow(row, query string, style lipgloss.Style) string {
	plain := ansi.Strip(row)
	lower := strings.ToLower(plain)
	needle := strings.ToLower(query)
	if needle == "" || !strings.Contains(lower, needle) {
		return row
	}
	var out strings.Builder
	column := 0 // Display columns of the row already written.
	from := 0   // Bytes of the plain text already written.
	for count := 0; count < highlightsPerRow; count++ {
		at := strings.Index(lower[from:], needle)
		if at < 0 {
			break
		}
		start := from + at
		end := start + len(needle)
		lead := column + ansi.StringWidth(plain[from:start])
		stop := lead + ansi.StringWidth(plain[start:end])
		out.WriteString(ansi.Cut(row, column, lead))
		// The highlight wins over whatever the match was wearing, which is the
		// point of a highlight; the text itself is kept.
		out.WriteString(style.Render(ansi.Strip(ansi.Cut(row, lead, stop))))
		column, from = stop, end
	}
	out.WriteString(ansi.Cut(row, column, column+ansi.StringWidth(plain[from:])))
	return out.String()
}
