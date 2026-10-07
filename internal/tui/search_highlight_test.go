package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

func colored(t *testing.T) {
	t.Helper()
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(old) })
}

// A row is styled text, so a match has to be found in what it says and painted
// without disturbing what it wears -- or the transcript's own colours end where
// the highlight began.
func TestHighlightKeepsTheRowItPaints(t *testing.T) {
	colored(t)
	row := "plain " + blue.Render("the relay refused") + " tail"
	out := highlightRow(row, "relay", searchCurrentMatch)
	if ansi.Strip(out) != ansi.Strip(row) {
		t.Fatalf("the text changed: %q", ansi.Strip(out))
	}
	if ansi.StringWidth(out) != ansi.StringWidth(row) {
		t.Fatal("the row changed width", ansi.StringWidth(out), ansi.StringWidth(row))
	}
	if !strings.Contains(out, searchCurrentMatch.Render("relay")) {
		t.Fatalf("the match is not painted: %q", out)
	}
	// What followed the match is still blue: cutting re-states the style each
	// piece was under.
	if !strings.Contains(out, blue.Render(" refused")) {
		t.Fatalf("the row's own colour ended at the highlight: %q", out)
	}
}

// Every occurrence, and nothing when there is none.
func TestHighlightEveryOccurrence(t *testing.T) {
	colored(t)
	row := "relay one relay two relay"
	out := highlightRow(row, "relay", searchOtherMatch)
	if n := strings.Count(out, searchOtherMatch.Render("relay")); n != 3 {
		t.Fatalf("%d of 3 painted: %q", n, out)
	}
	if got := highlightRow(row, "absent", searchOtherMatch); got != row {
		t.Fatal("a row without a match was rewritten")
	}
	if got := highlightRow(row, "", searchOtherMatch); got != row {
		t.Fatal("an empty query painted something")
	}
	// Case-insensitive, like the search the bar runs.
	out = highlightRow("Relay RELAY", "relay", searchOtherMatch)
	if !strings.Contains(out, searchOtherMatch.Render("Relay")) || !strings.Contains(out, searchOtherMatch.Render("RELAY")) {
		t.Fatalf("case was not ignored: %q", out)
	}
}

// Columns, not bytes: a transcript is full of text that is two columns wide per
// character, and a highlight measured in bytes would land beside the word.
func TestHighlightMeasuresInColumns(t *testing.T) {
	colored(t)
	row := "한글 " + blue.Render("relay 한글") + " 끝"
	out := highlightRow(row, "relay", searchCurrentMatch)
	if ansi.Strip(out) != ansi.Strip(row) || ansi.StringWidth(out) != ansi.StringWidth(row) {
		t.Fatalf("%q", out)
	}
	if !strings.Contains(out, searchCurrentMatch.Render("relay")) {
		t.Fatalf("the match moved: %q", out)
	}
	// And a match that is itself wide, twice in this row.
	out = highlightRow(row, "한글", searchOtherMatch)
	if n := strings.Count(out, searchOtherMatch.Render("한글")); n != 2 {
		t.Fatalf("%d of 2 wide matches painted: %q", n, out)
	}
	if ansi.StringWidth(out) != ansi.StringWidth(row) {
		t.Fatal("a wide highlight changed the row's width")
	}
}

// A pathological query cannot make one row unbounded work.
func TestHighlightIsBounded(t *testing.T) {
	colored(t)
	row := strings.Repeat("a", 200)
	out := highlightRow(row, "a", searchOtherMatch)
	if n := strings.Count(out, searchOtherMatch.Render("a")); n != highlightsPerRow {
		t.Fatalf("%d highlights, expected the cap of %d", n, highlightsPerRow)
	}
	if ansi.Strip(out) != row {
		t.Fatal("the capped row lost text")
	}
}

// The row the transcript is on wears the bright green; the rest wear the quiet
// one. Which row that is comes from the same map the renderer uses to know
// which event a row belongs to.
func TestHighlightMarksTheCurrentMatch(t *testing.T) {
	colored(t)
	m := searchModel(t, &searchClient{visits: searchVisits()})
	run(t, m, press("ctrl+f"))
	typing(t, m, "relay")
	// Row 0 belongs to event 40, row 1 to event 90. The current match is the
	// newest, so it is on row 1.
	m.historyPositions = []float64{40, 90}
	m.view.YOffset = 0
	rows := []string{"a relay here", "a relay there"}
	m.searchHighlightView(rows)
	if !strings.Contains(rows[1], searchCurrentMatch.Render("relay")) {
		t.Fatalf("the current match is not marked: %q", rows[1])
	}
	if !strings.Contains(rows[0], searchOtherMatch.Render("relay")) {
		t.Fatalf("another match is not marked: %q", rows[0])
	}
	// Stepping moves the mark with the transcript.
	run(t, m, press("enter"))
	rows = []string{"a relay here", "a relay there"}
	m.searchHighlightView(rows)
	if !strings.Contains(rows[0], searchCurrentMatch.Render("relay")) {
		t.Fatalf("the mark did not follow the step: %q", rows[0])
	}
}

// With no bar there is nothing to paint, and a wide search marks no row as the
// one the transcript is on, because it is not on one.
func TestHighlightOnlyWhileSearching(t *testing.T) {
	colored(t)
	m := searchModel(t, &searchClient{visits: searchVisits()})
	rows := []string{"a relay here"}
	m.searchHighlightView(rows)
	if rows[0] != "a relay here" {
		t.Fatal("something was painted without a search")
	}
	run(t, m, press("f18"))
	typing(t, m, "relay")
	m.historyPositions = []float64{40}
	rows = []string{"a relay here"}
	m.searchHighlightView(rows)
	if !strings.Contains(rows[0], searchOtherMatch.Render("relay")) {
		t.Fatalf("a wide search paints what it typed: %q", rows[0])
	}
	if strings.Contains(rows[0], searchCurrentMatch.Render("relay")) {
		t.Fatal("a wide search claimed the transcript is on a match")
	}
}

// Painting happens as the query is typed, before the answer that counts the
// matches arrives: the bar matches the same way the server does, so it does not
// have to wait to be sure.
func TestHighlightDoesNotWaitForTheServer(t *testing.T) {
	colored(t)
	m := searchModel(t, &searchClient{})
	run(t, m, press("ctrl+f"))
	m.Update(press("r"))
	m.Update(press("e"))
	rows := []string{"a relay here"}
	m.searchHighlightView(rows)
	if !strings.Contains(rows[0], searchOtherMatch.Render("re")) {
		t.Fatalf("typing did not paint: %q", rows[0])
	}
}

// The transcript is painted where it is drawn, so a scrolled window still marks
// the right row.
func TestHighlightFollowsTheScrolledWindow(t *testing.T) {
	colored(t)
	m := searchModel(t, &searchClient{visits: searchVisits()})
	run(t, m, press("ctrl+f"))
	typing(t, m, "relay")
	m.historyPositions = []float64{10, 20, 40, 90}
	m.view.YOffset = 2
	rows := []string{"a relay here", "a relay there"}
	m.searchHighlightView(rows)
	if !strings.Contains(rows[1], searchCurrentMatch.Render("relay")) {
		t.Fatalf("the offset was not accounted for: %q", rows[1])
	}
}
