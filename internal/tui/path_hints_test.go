package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/lesomnus/cxz/internal/containerterm"
	"github.com/muesli/termenv"
)

func TestPathTokensRequireBackticks(t *testing.T) {
	for _, value := range []string{"/", "~", "/help", "http://example.com/path", "`/tmp`", "`~/foo` 그리고", "`word /tmp"} {
		if _, ok := pathTokenAt(value, len([]rune(value))); ok {
			t.Fatal("unexpected filesystem trigger", value)
		}
	}
	for _, tc := range []struct{ value, parent, query string }{{"`/", "/", ""}, {"`~", "~/", ""}, {"먼저 `~/한글 경로/파", "~/한글 경로/", "파"}, {"`/tmp` 그리고 `/usr/lo", "/usr/", "lo"}} {
		token, ok := pathTokenAt(tc.value, len([]rune(tc.value)))
		if !ok || token.parent != tc.parent || token.query != tc.query {
			t.Fatal(tc, token, ok)
		}
	}
}

func pathModel() *model {
	m := conversationModel()
	m.ctx = context.Background()
	m.program = tea.NewProgram(m)
	m.lastUIInput = time.Now()
	m.lastActivityReport = time.Now()
	return m
}
func loadPathFixture(m *model, entries ...containerterm.PathEntry) {
	m.receivePathHints(pathHintResult{generation: m.pathHints.generation, listing: containerterm.PathListing{Entries: entries}})
}

func TestPathCompletionTypingCloseAndEscape(t *testing.T) {
	m := pathModel()
	m.input.SetValue("파일 `/usr/lc")
	m.syncPathHints()
	loadPathFixture(m, containerterm.PathEntry{Name: "local", Directory: true}, containerterm.PathEntry{Name: "libexec", Directory: true})
	if got := m.pathOptions(); len(got) != 2 || got[0].text != "/usr/local/" {
		t.Fatal("fuzzy matching", got)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.pathHints.selected != 1 {
		t.Fatal("selection did not move")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.input.Value() != "파일 `/usr/local/" || m.focusList || m.pathHints.token.parent != "/usr/local/" {
		t.Fatal("entry completion", m.input.Value())
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	if m.input.Value() != "파일 `/usr/local/s" {
		t.Fatal("typing left composer")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.pathHints != nil {
		t.Fatal("escape failed")
	}
	m.syncPathHints()
	if m.pathHints != nil {
		t.Fatal("escape reopened immediately")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	if m.pathHints == nil {
		t.Fatal("editing did not reopen")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("`")})
	if m.pathHints != nil {
		t.Fatal("closing backtick did not finish exploration")
	}
	if len(m.client.(*recordingClient).inputs) != 0 {
		t.Fatal("completion submitted a conversation")
	}
}

func TestPathCompletionPreservesSurroundingTextAndStaleResults(t *testing.T) {
	m := pathModel()
	m.input.SetValue("설명\n`/tmp/한` 뒤 문장\n다음 줄")
	for m.input.Line() > 1 {
		m.input.CursorStart()
		m.input.CursorUp()
	}
	m.input.SetCursor(len([]rune("`/tmp/한")))
	m.syncPathHints()
	old := m.pathHints.generation
	loadPathFixture(m, containerterm.PathEntry{Name: "한글 파일.txt"})
	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.input.Value() != "설명\n`/tmp/한글 파일.txt` 뒤 문장\n다음 줄" || m.input.Line() != 1 {
		t.Fatal("surrounding text changed", m.input.Value(), m.input.Line())
	}
	m.input.SetValue("`/var/")
	m.syncPathHints()
	m.receivePathHints(pathHintResult{generation: old, listing: containerterm.PathListing{Entries: []containerterm.PathEntry{{Name: "stale"}}}})
	if len(m.pathOptions()) != 0 {
		t.Fatal("old directory result replaced new one")
	}
	m.input.SetValue("/help")
	m.syncPathHints()
	if m.pathHints != nil || len(m.commandHints()) == 0 {
		t.Fatal("slash command regressed")
	}
	if strings.Contains(m.commandOverlay(strings.Repeat("line\n", 15)), "Container paths") {
		t.Fatal("path overlay leaked")
	}
}

func TestPathOverlayFitsFourRows(t *testing.T) {
	m := pathModel()
	m.input.SetValue("`/usr/")
	m.syncPathHints()
	loadPathFixture(m, containerterm.PathEntry{Name: "local", Directory: true})
	view := m.pathHintOverlay("row\nrow\nrow\nrow")
	if len(strings.Split(view, "\n")) != 4 || !strings.Contains(view, "local/") {
		t.Fatal("hint invisible in small viewport", view)
	}
}

func TestPathEnterClosesAndPreservesSuffix(t *testing.T) {
	for _, suffix := range []string{"", "` 뒤 문장"} {
		m := pathModel()
		m.setPathInput("먼저 `/tmp/한"+suffix, len([]rune("먼저 `/tmp/한")))
		m.syncPathHints()
		loadPathFixture(m, containerterm.PathEntry{Name: "한글.txt"})
		m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		want := "먼저 `/tmp/한글.txt`" + strings.TrimPrefix(suffix, "`")
		if m.input.Value() != want || m.pathHints != nil {
			t.Fatal(m.input.Value(), m.pathHints)
		}
		_, pos, _, _, _ := m.chipInput()
		if pos != len([]rune("먼저 `/tmp/한글.txt`")) {
			t.Fatal("cursor", pos)
		}
		if len(m.client.(*recordingClient).inputs) != 0 {
			t.Fatal("sent conversation")
		}
	}
}

func TestPathStreamingKeepsSelectionAndSuccessor(t *testing.T) {
	m := pathModel()
	m.input.SetValue("`/tmp/")
	m.syncPathHints()
	loadPathFixture(m, containerterm.PathEntry{Name: "b"}, containerterm.PathEntry{Name: "c"}, containerterm.PathEntry{Name: "z"})
	m.pathHints.selected = 0
	loadPathFixture(m, containerterm.PathEntry{Name: "a"}, containerterm.PathEntry{Name: "b"}, containerterm.PathEntry{Name: "c"}, containerterm.PathEntry{Name: "d"}, containerterm.PathEntry{Name: "z"})
	opts := m.pathOptions()
	for i, want := range []string{"b", "c", "a", "d", "z"} {
		if opts[i].text != "/tmp/"+want {
			t.Fatal(opts)
		}
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	if opts = m.pathOptions(); len(opts) != 1 || opts[0].text != "/tmp/a" {
		t.Fatal("query did not reset ordering", opts)
	}
}

func TestPathWordDeleteStopsAtSlash(t *testing.T) {
	m := pathModel()
	m.input.SetValue("설명 `/tmp/한글.txt")
	for _, want := range []string{"설명 `/tmp/", "설명 `/tmp", "설명 `/"} {
		m.Update(tea.KeyMsg{Type: tea.KeyCtrlW})
		if m.input.Value() != want {
			t.Fatal(m.input.Value(), want)
		}
	}
}

func TestPathMatchingHighlight(t *testing.T) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(old) })
	got := highlightPath("Local", "lc", blue)
	want := magenta.Render("L") + blue.Render("o") + magenta.Render("c") + blue.Render("a") + blue.Render("l")
	if got != want {
		t.Fatalf("match colors: %q != %q", got, want)
	}
}
