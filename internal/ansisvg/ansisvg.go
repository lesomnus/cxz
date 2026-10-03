// Package ansisvg draws a rendered terminal screen as an SVG, so documentation
// can carry the colours a terminal shows without a bitmap. Output is text: a
// figure stays diffable, greppable and a few kilobytes, and regenerating one
// shows what changed on screen rather than that some pixels moved.
package ansisvg

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/rivo/uniseg"
)

// Options describes the page around the screen. The zero value is a dark
// terminal at a readable size.
type Options struct {
	FontSize   float64 // px; a cell advances 0.6 of it, a row 1.3
	Foreground string  // default text colour
	Background string  // colour behind the screen
	Page       string  // colour behind everything, "" for transparent
	Padding    float64 // px between the screen's text and its edge; negative for none
	Radius     float64 // px corner radius of the screen
	Title      string  // accessible description of the figure
	Labels     []Label // notes in the right margin
	LabelColor string
}

// Label names a component beside the row it occupies, so a figure says what
// its parts are without a second drawing to annotate it.
type Label struct {
	Row  int // zero-based screen row
	Text string
}

const (
	advance  = 0.6 // cell width as a share of the font size
	leading  = 1.3 // row height as a share of the font size
	baseline = 1.0 // where a row's text sits inside it
)

type attrs struct {
	fg, bg                                  string
	bold, faint, italic, underline, reverse bool
}

type cell struct {
	text  string
	width int // display cells; 0 marks the tail of a wide glyph
	attrs
}

type run struct {
	col, width int
	text       string
	attrs
}

// Render converts a screen -- lines of text carrying SGR escape sequences, as a
// terminal UI writes them -- into an SVG of the same grid.
func Render(screen string, o Options) string {
	if o.FontSize <= 0 {
		o.FontSize = 14
	}
	if o.Foreground == "" {
		o.Foreground = "#d4d4d4"
	}
	if o.Background == "" {
		o.Background = "#101317"
	}
	if o.LabelColor == "" {
		o.LabelColor = "#6b7280"
	}
	switch {
	case o.Padding < 0:
		o.Padding = 0
	case o.Padding == 0:
		o.Padding = o.FontSize * 0.6
	}
	if o.Radius <= 0 {
		o.Radius = o.FontSize * 0.4
	}
	cw, ch := o.FontSize*advance, o.FontSize*leading
	rows := parse(strings.Split(strings.TrimRight(screen, "\n"), "\n"), o.Foreground, o.Background)
	cols := 0
	for _, row := range rows {
		cols = max(cols, len(row))
	}
	boxW := float64(cols)*cw + 2*o.Padding
	boxH := float64(len(rows))*ch + 2*o.Padding
	w, h := boxW, boxH
	gap := 2 * cw
	for _, l := range o.Labels {
		w = max(w, boxW+gap+float64(len([]rune(l.Text)))*cw*0.95+o.Padding)
	}
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%s" height="%s" viewBox="0 0 %s %s" font-family="ui-monospace,SFMono-Regular,Menlo,Consolas,'DejaVu Sans Mono',monospace" font-size="%s">`,
		num(w), num(h), num(w), num(h), num(o.FontSize))
	if o.Title != "" {
		fmt.Fprintf(&b, "\n<title>%s</title>", escape(o.Title))
	}
	if o.Page != "" {
		fmt.Fprintf(&b, "\n<rect width=\"%s\" height=\"%s\" fill=\"%s\"/>", num(w), num(h), o.Page)
	}
	fmt.Fprintf(&b, "\n<rect width=\"%s\" height=\"%s\" rx=\"%s\" fill=\"%s\"/>", num(boxW), num(boxH), num(o.Radius), o.Background)
	// Backgrounds first, as a terminal paints them: one rect per span of cells
	// sharing a colour, so a shaded block costs a rect rather than a cell each.
	b.WriteString("\n<g>")
	for _, block := range shaded(rows) {
		fmt.Fprintf(&b, "\n<rect x=\"%s\" y=\"%s\" width=\"%s\" height=\"%s\" fill=\"%s\"/>",
			num(o.Padding+float64(block.col)*cw), num(o.Padding+float64(block.row)*ch),
			num(float64(block.width)*cw), num(float64(block.height)*ch), block.fill)
	}
	b.WriteString("\n</g>")
	// Text keeps the grid by setting every run's own x and its exact advance:
	// a font whose cell is not 0.6 em adjusts its letter spacing instead of
	// drifting away from the column it started in.
	fmt.Fprintf(&b, "\n<g xml:space=\"preserve\" fill=\"%s\">", o.Foreground)
	for y, row := range rows {
		for _, r := range spans(row, func(c cell) string { return c.key() }) {
			if r = trim(r); r.text == "" {
				continue
			}
			fmt.Fprintf(&b, "\n<text x=\"%s\" y=\"%s\" textLength=\"%s\" lengthAdjust=\"spacing\"",
				num(o.Padding+float64(r.col)*cw), num(o.Padding+float64(y)*ch+o.FontSize*baseline), num(float64(r.width)*cw))
			if r.fg != "" && r.fg != o.Foreground {
				fmt.Fprintf(&b, " fill=\"%s\"", r.fg)
			}
			if r.bold {
				b.WriteString(` font-weight="bold"`)
			}
			if r.italic {
				b.WriteString(` font-style="italic"`)
			}
			if r.faint {
				b.WriteString(` opacity=".65"`)
			}
			if r.underline {
				b.WriteString(` text-decoration="underline"`)
			}
			fmt.Fprintf(&b, ">%s</text>", escape(r.text))
		}
	}
	b.WriteString("\n</g>")
	if len(o.Labels) > 0 {
		fmt.Fprintf(&b, "\n<g fill=\"%s\" font-size=\"%s\">", o.LabelColor, num(o.FontSize*0.85))
		labels := append([]Label(nil), o.Labels...)
		sort.SliceStable(labels, func(i, j int) bool { return labels[i].Row < labels[j].Row })
		// Two components can share a row. The note moves down far enough to be
		// read, and its leader bends back to the row it is about, rather than
		// printing one label over another.
		taken := -1e9
		for _, l := range labels {
			at := o.Padding + float64(l.Row)*ch + o.FontSize*baseline
			y := max(at, taken+o.FontSize*1.25)
			taken = y
			fmt.Fprintf(&b, "\n<text x=\"%s\" y=\"%s\">%s</text>", num(boxW+gap), num(y), escape(l.Text))
			fmt.Fprintf(&b, "\n<path d=\"M%s %sL%s %sL%s %s\" fill=\"none\" stroke=\"%s\" stroke-width=\".5\"/>",
				num(boxW), num(at-o.FontSize*0.3), num(boxW+gap*0.45), num(at-o.FontSize*0.3),
				num(boxW+gap*0.8), num(y-o.FontSize*0.3), o.LabelColor)
		}
		b.WriteString("\n</g>")
	}
	b.WriteString("\n</svg>\n")
	return b.String()
}

func (a attrs) key() string {
	s := a.fg
	for _, on := range []bool{a.bold, a.faint, a.italic, a.underline} {
		if on {
			s += "1"
		} else {
			s += "0"
		}
	}
	return s
}

type block struct {
	col, row, width, height int
	fill                    string
}

// shaded turns the coloured cells into rectangles, joining a span to the one
// above it when they cover the same columns in the same colour. A panel that
// shades its whole screen is then one rect rather than one per row.
func shaded(rows [][]cell) []block {
	var out []block
	open := map[string]int{}
	for y, row := range rows {
		seen := map[string]bool{}
		for _, r := range spans(row, func(c cell) string { return c.bg }) {
			if r.bg == "" {
				continue
			}
			k := fmt.Sprint(r.col, " ", r.width, " ", r.bg)
			seen[k] = true
			if i, ok := open[k]; ok && out[i].row+out[i].height == y {
				out[i].height++
				continue
			}
			open[k] = len(out)
			out = append(out, block{col: r.col, row: y, width: r.width, height: 1, fill: r.bg})
		}
		for k := range open {
			if !seen[k] {
				delete(open, k)
			}
		}
	}
	return out
}

// spans groups neighbouring cells that agree on whatever key picks out. Width
// counts columns, so a wide glyph's tail cell -- which carries its attributes
// and no text -- extends the run it belongs to rather than splitting it.
func spans(row []cell, key func(cell) string) []run {
	var out []run
	for i, c := range row {
		if n := len(out); n > 0 && key(row[i-1]) == key(c) {
			out[n-1].width++
			out[n-1].text += c.text
			continue
		}
		out = append(out, run{col: i, width: 1, text: c.text, attrs: c.attrs})
	}
	return out
}

// trim drops a run's blank edges: nothing can show their colour, and a terminal
// screen is mostly padding, so keeping them would be most of the figure.
func trim(r run) run {
	for strings.HasPrefix(r.text, " ") {
		r.text, r.col, r.width = r.text[1:], r.col+1, r.width-1
	}
	for strings.HasSuffix(r.text, " ") {
		r.text, r.width = r.text[:len(r.text)-1], r.width-1
	}
	return r
}

func parse(lines []string, fg, bg string) [][]cell {
	rows := make([][]cell, 0, len(lines))
	var state attrs
	for _, line := range lines {
		var row []cell
		for len(line) > 0 {
			if line[0] == 0x1b {
				rest, codes, ok := sequence(line)
				line = rest
				if ok {
					state = apply(state, codes)
				}
				continue
			}
			g := uniseg.NewGraphemes(line)
			if !g.Next() {
				break
			}
			text, width := g.Str(), g.Width()
			line = line[len(text):]
			if text == "\t" {
				text, width = " ", 1
			}
			if width == 0 {
				continue
			}
			c := cell{text: text, width: width, attrs: state}
			if c.reverse {
				// A reversed cell paints its text colour as a background, which
				// is what a cursor is. The terminal's own two colours stand in
				// for whichever side was left at the default.
				c.fg, c.bg = or(c.bg, bg), or(c.fg, fg)
			}
			row = append(row, c)
			for range width - 1 {
				row = append(row, cell{attrs: c.attrs})
			}
		}
		rows = append(rows, row)
	}
	return rows
}

func or(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// sequence consumes one escape sequence, returning the SGR parameters when it
// was one. Anything else -- a hyperlink, a cursor move -- is dropped, since a
// screen already laid out in rows does not need it.
func sequence(s string) (rest string, codes []string, ok bool) {
	if len(s) < 2 {
		return "", nil, false
	}
	switch s[1] {
	case '[':
		for i := 2; i < len(s); i++ {
			if s[i] >= 0x40 && s[i] <= 0x7e {
				if s[i] == 'm' {
					return s[i+1:], strings.Split(s[2:i], ";"), true
				}
				return s[i+1:], nil, false
			}
		}
		return "", nil, false
	case ']':
		if i := strings.IndexByte(s, 0x07); i >= 0 {
			return s[i+1:], nil, false
		}
		if i := strings.Index(s, "\x1b\\"); i >= 0 {
			return s[i+2:], nil, false
		}
		return "", nil, false
	default:
		return s[2:], nil, false
	}
}

func apply(a attrs, codes []string) attrs {
	for i := 0; i < len(codes); i++ {
		n, err := strconv.Atoi(codes[i])
		if codes[i] == "" {
			n, err = 0, nil
		}
		if err != nil {
			continue
		}
		switch {
		case n == 0:
			a = attrs{}
		case n == 1:
			a.bold = true
		case n == 2:
			a.faint = true
		case n == 3:
			a.italic = true
		case n == 4:
			a.underline = true
		case n == 7:
			a.reverse = true
		case n == 27:
			a.reverse = false
		case n == 22:
			a.bold, a.faint = false, false
		case n == 23:
			a.italic = false
		case n == 24:
			a.underline = false
		case n == 39:
			a.fg = ""
		case n == 49:
			a.bg = ""
		case n >= 30 && n <= 37:
			a.fg = basic[n-30]
		case n >= 90 && n <= 97:
			a.fg = basic[n-90+8]
		case n >= 40 && n <= 47:
			a.bg = basic[n-40]
		case n >= 100 && n <= 107:
			a.bg = basic[n-100+8]
		case n == 38 || n == 48:
			colour, used := extended(codes[i+1:])
			if used == 0 {
				return a
			}
			i += used
			if n == 38 {
				a.fg = colour
			} else {
				a.bg = colour
			}
		}
	}
	return a
}

// extended reads the 256-colour and 24-bit forms of 38/48 and reports how many
// parameters it took, so a sequence that mixes them stays in step.
func extended(codes []string) (string, int) {
	if len(codes) == 0 {
		return "", 0
	}
	value := func(i int) int {
		if i >= len(codes) {
			return -1
		}
		n, err := strconv.Atoi(codes[i])
		if err != nil || n < 0 || n > 255 {
			return -1
		}
		return n
	}
	switch codes[0] {
	case "2":
		r, g, b := value(1), value(2), value(3)
		if r < 0 || g < 0 || b < 0 {
			return "", 0
		}
		return fmt.Sprintf("#%02x%02x%02x", r, g, b), 4
	case "5":
		n := value(1)
		if n < 0 {
			return "", 0
		}
		return indexed(n), 2
	}
	return "", 0
}

// indexed resolves an xterm palette entry. The cube and the greys are formulas;
// the first sixteen are a convention, and this is the xterm default.
func indexed(n int) string {
	switch {
	case n < 16:
		return basic[n]
	case n < 232:
		n -= 16
		level := func(v int) int {
			if v == 0 {
				return 0
			}
			return 55 + 40*v
		}
		return fmt.Sprintf("#%02x%02x%02x", level(n/36), level(n/6%6), level(n%6))
	default:
		v := 8 + 10*(n-232)
		return fmt.Sprintf("#%02x%02x%02x", v, v, v)
	}
}

var basic = [16]string{
	"#000000", "#cd0000", "#00cd00", "#cdcd00", "#0000ee", "#cd00cd", "#00cdcd", "#e5e5e5",
	"#7f7f7f", "#ff0000", "#00ff00", "#ffff00", "#5c5cff", "#ff00ff", "#00ffff", "#ffffff",
}

func escape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}

// num keeps coordinates short: a grid of a few hundred cells does not need more
// than two decimals, and trailing zeros are bytes in every element.
func num(v float64) string {
	s := strconv.FormatFloat(v, 'f', 2, 64)
	s = strings.TrimRight(s, "0")
	return strings.TrimSuffix(s, ".")
}
