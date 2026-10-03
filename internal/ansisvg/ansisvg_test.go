package ansisvg

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
)

var textPattern = regexp.MustCompile(`<text ([^>]*)>([^<]*)</text>`)
var rectPattern = regexp.MustCompile(`<rect x="([^"]*)" y="([^"]*)" width="([^"]*)" height="([^"]*)" fill="([^"]*)"/>`)

type drawn struct {
	x, y, length float64
	fill, text   string
	attrs        string
}

func texts(t *testing.T, svg string) []drawn {
	t.Helper()
	var out []drawn
	for _, m := range textPattern.FindAllStringSubmatch(svg, -1) {
		d := drawn{attrs: m[1], text: m[2]}
		d.x, d.y = number(t, attr(t, m[1], "x")), number(t, attr(t, m[1], "y"))
		if length := attr(t, m[1], "textLength"); length != "" {
			d.length = number(t, length)
		}
		d.fill = attr(t, m[1], "fill")
		out = append(out, d)
	}
	return out
}

func attr(t *testing.T, attrs, name string) string {
	t.Helper()
	m := regexp.MustCompile(` ?\b` + name + `="([^"]*)"`).FindStringSubmatch(" " + attrs)
	if m == nil {
		return ""
	}
	return m[1]
}

func number(t *testing.T, s string) float64 {
	t.Helper()
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		t.Fatalf("%q is not a number: %v", s, err)
	}
	return v
}

// The grid is the whole point: a run says where it starts and how wide it is,
// so a font whose cell is not exactly 0.6 em stays in its column.
func TestRunsKeepTheGrid(t *testing.T) {
	svg := Render("ab\n\x1b[38;2;255;136;0mcd\x1b[0mef", Options{FontSize: 10, Padding: -1})
	got := texts(t, svg)
	if len(got) != 3 {
		t.Fatal("three runs became", len(got), got)
	}
	want := []drawn{{x: 0, y: 10, length: 12, text: "ab"}, {x: 0, y: 23, length: 12, fill: "#ff8800", text: "cd"}, {x: 12, y: 23, length: 12, text: "ef"}}
	for i, w := range want {
		if got[i].x != w.x || got[i].y != w.y || got[i].length != w.length || got[i].fill != w.fill || got[i].text != w.text {
			t.Fatalf("run %d is %+v, want %+v", i, got[i], w)
		}
	}
	// The default colour is the group's, not repeated on every run.
	if strings.Contains(got[0].attrs, "fill") {
		t.Fatal("an unstyled run carries its own colour:", got[0].attrs)
	}
}

// Blank cells cannot show a colour, so they are not drawn. A terminal screen is
// mostly padding; keeping it would be most of the file.
func TestBlankCellsAreNotDrawn(t *testing.T) {
	svg := Render("  text   \n         ", Options{FontSize: 10, Padding: -1})
	got := texts(t, svg)
	if len(got) != 1 {
		t.Fatal("blank cells were drawn:", got)
	}
	if got[0].text != "text" || got[0].x != 12 {
		t.Fatalf("run is %+v, want the text alone at its own column", got[0])
	}
	// Interior spaces stay, with the space they were written in preserved.
	svg = Render("\x1b[32ma b\x1b[0m", Options{FontSize: 10, Padding: -1})
	if got = texts(t, svg); len(got) != 1 || got[0].text != "a b" {
		t.Fatal("an interior space was lost:", got)
	}
	if !strings.Contains(svg, `xml:space="preserve"`) {
		t.Fatal("spaces are not preserved, so a viewer may collapse them")
	}
}

// A span of one colour is one rect, whatever is written on it.
func TestBackgroundsSpanTheirCells(t *testing.T) {
	svg := Render("\x1b[48;2;40;40;40m one two \x1b[0m", Options{FontSize: 10, Padding: -1})
	var shade []string
	for _, m := range rectPattern.FindAllStringSubmatch(svg, -1) {
		if m[5] == "#282828" {
			shade = append(shade, m[1]+"+"+m[3])
		}
	}
	if len(shade) != 1 || shade[0] != "0+54" {
		t.Fatal("the shaded span drew", shade, "want one rect over nine cells")
	}
}

// A reversed cell is the cursor: the text colour becomes the background, and
// the terminal's own colours stand in for whichever side was default.
func TestReverseBecomesABlock(t *testing.T) {
	svg := Render("a\x1b[7mb\x1b[0mc", Options{FontSize: 10, Padding: -1, Foreground: "#eeeeee", Background: "#111111"})
	if !strings.Contains(svg, `<rect x="6" y="0" width="6" height="13" fill="#eeeeee"/>`) {
		t.Fatal("the reversed cell has no block:", svg)
	}
	for _, d := range texts(t, svg) {
		if d.text == "b" && d.fill != "#111111" {
			t.Fatal("the reversed cell's text is", d.fill, "want the screen's colour")
		}
	}
}

// Wide glyphs take two columns, so what follows them has to know that.
func TestWideGlyphsAdvanceTwoColumns(t *testing.T) {
	got := texts(t, Render("한글\x1b[31mx", Options{FontSize: 10, Padding: -1}))
	if len(got) != 2 {
		t.Fatal("expected two runs, got", got)
	}
	if got[0].length != 24 {
		t.Fatal("two wide glyphs measured", got[0].length, "want four cells")
	}
	if got[1].x != 24 {
		t.Fatal("the run after them starts at", got[1].x, "want the fifth column")
	}
}

func TestPaletteForms(t *testing.T) {
	for _, tc := range []struct{ codes, want string }{
		{"31", "#cd0000"},
		{"91", "#ff0000"},
		{"38;5;9", "#ff0000"},
		{"38;5;208", "#ff8700"},
		{"38;5;240", "#585858"},
		{"38;2;1;2;3", "#010203"},
		{"1;38;5;33", "#0087ff"},
	} {
		got := texts(t, Render("\x1b["+tc.codes+"mx", Options{FontSize: 10, Padding: -1}))
		if len(got) != 1 || got[0].fill != tc.want {
			t.Fatalf("SGR %s drew %+v, want %s", tc.codes, got, tc.want)
		}
	}
	// Bold, faint, italic and underline survive as presentation, not colour.
	svg := Render("\x1b[1mb\x1b[0m \x1b[2mf\x1b[0m \x1b[3mi\x1b[0m \x1b[4mu\x1b[0m", Options{FontSize: 10, Padding: -1})
	for _, want := range []string{`font-weight="bold"`, `opacity=".65"`, `font-style="italic"`, `text-decoration="underline"`} {
		if !strings.Contains(svg, want) {
			t.Fatal("missing", want)
		}
	}
}

// Sequences that are not SGR carry no colour and no width, and a figure that
// kept them would show them as text.
func TestNonColourSequencesAreDropped(t *testing.T) {
	svg := Render("\x1b]8;;https://example.com\x07link\x1b]8;;\x07\x1b[2Jtail", Options{FontSize: 10, Padding: -1})
	got := texts(t, svg)
	if len(got) != 1 || got[0].text != "linktail" {
		t.Fatal("escape sequences leaked into the figure:", got)
	}
	if strings.Contains(svg, "example.com") || strings.Contains(svg, "\x1b") {
		t.Fatal("the figure carries an escape sequence")
	}
}

func TestMarkupIsEscaped(t *testing.T) {
	svg := Render(`<a> & "b"`, Options{FontSize: 10, Padding: -1})
	if !strings.Contains(svg, "&lt;a&gt; &amp;") {
		t.Fatal("markup was not escaped:", svg)
	}
}

// Labels name a component beside the row it occupies, and widen the page rather
// than overprinting the screen.
func TestLabelsSitBesideTheirRow(t *testing.T) {
	plain := Render("one\ntwo", Options{FontSize: 10, Padding: -1})
	svg := Render("one\ntwo", Options{FontSize: 10, Padding: -1, Labels: []Label{{Row: 1, Text: "composer"}}})
	if !strings.Contains(svg, ">composer</text>") {
		t.Fatal("the label is missing")
	}
	width := func(s string) float64 {
		return number(t, regexp.MustCompile(`width="([^"]*)"`).FindStringSubmatch(s)[1])
	}
	if width(svg) <= width(plain) {
		t.Fatal("the label did not widen the page")
	}
	for _, d := range texts(t, svg) {
		if d.text == "composer" && d.y != 23 {
			t.Fatal("the label is at", d.y, "want the row it names")
		}
	}
}

// A screen is text, so a figure of one is kilobytes rather than a bitmap's
// hundreds. This is the reason the package exists; it is worth a test.
func TestAFullScreenStaysSmall(t *testing.T) {
	var rows []string
	for i := range 30 {
		rows = append(rows, "\x1b[38;2;80;200;120m"+strings.Repeat("x", 3)+"\x1b[0m "+strings.Repeat("text ", 8)+strconv.Itoa(i)+strings.Repeat(" ", 20))
	}
	svg := Render(strings.Join(rows, "\n"), Options{})
	if len(svg) > 12<<10 {
		t.Fatal("a 30-row screen rendered", len(svg), "bytes")
	}
}
