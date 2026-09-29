package tui

import (
	"context"
	"fmt"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
	"io"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/vt"
	"github.com/lesomnus/cxz/api"
)

func TestPanelLongListAndDeletedProjects(t *testing.T) {
	m := panelModel()
	projects := []*api.Project{}
	for i := 0; i < 80; i++ {
		projects = append(projects, &api.Project{Id: fmt.Sprint(i), Name: fmt.Sprintf("Project %02d", i)})
	}
	m.updatePanel(listing{projects: projects, projectsLoaded: true})
	m.focusPanel()
	m.Update(tea.KeyMsg{Type: tea.KeyEnd})
	if m.panelIndex != 79 || !strings.Contains(ansi.Strip(m.panelScreen()), "Project 79") {
		t.Fatal("last project inaccessible")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	if m.panelIndex != 69 || !strings.Contains(ansi.Strip(m.panelScreen()), "› Project 69") {
		t.Fatal("page up did not account for divider rows", m.panelIndex)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	if m.panelIndex != 79 || !strings.Contains(ansi.Strip(m.panelScreen()), "› Project 79") {
		t.Fatal("page down selected a divider or hid the cursor", m.panelIndex)
	}
	m.updatePanel(listing{projectsLoaded: true})
	if len(m.panelRows()) != 0 || m.panelIndex != 0 {
		t.Fatal("deleted projects retained")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
}

func panelModel() *model {
	m := conversationModel()
	m.Update(tea.WindowSizeMsg{Width: 200, Height: 30})
	m.updatePanel(listing{projects: []*api.Project{m.project, {Id: "other", Name: "Other project"}}, sessions: []*api.Session{m.current(), {Id: "other-session", Alias: "maple", ProjectId: "other", Agent: "codex", RunId: "other-run"}}})
	return m
}

func TestResponsiveViewWidthsAndBlankRemainder(t *testing.T) {
	for _, size := range []struct{ terminal, content, panel int }{
		{40, 40, 0}, {69, 69, 0}, {70, 40, 28}, {80, 50, 28},
		{100, 70, 28}, {133, 98, 33}, {144, 106, 36}, {150, 112, 36},
		{170, 132, 36}, {171, 133, 36}, {200, 133, 36}, {300, 133, 36},
	} {
		width := size.terminal
		m := panelModel()
		m.Update(tea.WindowSizeMsg{Width: width, Height: 30})
		if m.width != size.content || m.panelVisible() != (size.panel > 0) {
			t.Fatal("wrong breakpoint", width)
		}
		if size.panel > 0 && m.contentOffset() != size.panel+2 {
			t.Fatal("wrong panel width or gap", width, m.contentOffset())
		}
		for _, view := range []string{"session", "project", "accounts"} {
			m.projectView = view == "project"
			m.accountView = view == "accounts"
			lines := strings.Split(m.View(), "\n")
			if len(lines) != 30 {
				t.Fatal("wrong height", width, view)
			}
			for _, line := range lines {
				if ansi.StringWidth(line) != width {
					t.Fatal("wrong screen width", width, view)
				}
				if strings.TrimSpace(ansi.Strip(ansi.Cut(line, m.contentOffset()+m.width, width))) != "" {
					t.Fatal("right remainder not blank")
				}
			}
		}
	}
}

func TestProjectPanelFocusAndSessionSwitch(t *testing.T) {
	m := panelModel()
	m.input.SetValue("keep my draft")
	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.panelFocus {
		t.Fatal("Tab entered project panel")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlQ})
	if !m.panelFocus || m.projectView {
		t.Fatal("Ctrl+Q must focus panel without leaving session view")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if !m.panelFocus {
		t.Fatal("Tab must not leave panel")
	}
	for i, r := range m.panelRows() {
		if r.session != nil && r.session.Id == "other-session" {
			m.panelIndex = i
		}
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.current().Id != "other-session" || m.project.Id != "other" || m.panelFocus || m.projectView {
		t.Fatal("did not switch project/session")
	}
	if m.drafts["s"] != "keep my draft" || m.input.Value() != "" {
		t.Fatal("draft leaked or lost")
	}
	// An in-flight refresh from the old project must not change the destination.
	m.Update(listing{project: &api.Project{Id: "p"}, sessions: m.allSessions})
	if m.project.Id != "other" || m.current().Id != "other-session" {
		t.Fatal("stale refresh undid switch")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlQ})
	m.Update(tea.WindowSizeMsg{Width: 150, Height: 30})
	if !m.panelFocus || m.input.Focused() || !strings.Contains(ansi.Strip(m.View()), "Projects") {
		t.Fatal("narrow navigation lost focus or project list")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlQ})
	if m.panelFocus || m.projectView || !m.input.Focused() {
		t.Fatal("narrow Ctrl+Q did not return to conversation")
	}
}

func TestWorkflowReceivesSelectedProject(t *testing.T) {
	m := panelModel()
	m.project = &api.Project{Id: "other"}
	m.createProjectSession = func(_ context.Context, project, account string, _ io.Reader, _ io.Writer, _ io.Writer) (*api.Session, error) {
		if project != "other" || account != "main" {
			t.Error("workflow targeted original project")
		}
		return &api.Session{Id: "new"}, nil
	}
	cmd := m.startAccountWorkflow("main", "codex", "", true)
	defer m.workflow.close()
	batch := cmd().(tea.BatchMsg)
	if done := batch[0]().(workflowDone); done.err != nil {
		t.Fatal(done.err)
	}
}

func TestProjectNavigatorPreservesFocusAcrossResponsiveLayouts(t *testing.T) {
	m := panelModel()
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	m.input.SetValue("unfinished draft")
	if view := ansi.Strip(m.View()); !strings.Contains(view, "Projects") || !strings.Contains(view, "unfinished draft") || !m.input.Focused() {
		t.Fatal("compact panel must remain visible while composing", view)
	}
	canceled := false
	m.watchCancel = func() { canceled = true }
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlQ})
	if !m.panelFocus || m.projectView || canceled {
		t.Fatal("opening navigator left the conversation")
	}
	for i, r := range m.panelRows() {
		if r.session != nil && r.session.Id == "other-session" {
			m.panelIndex = i
		}
	}
	selected := m.panelRows()[m.panelIndex].key()
	for _, width := range []int{80, 200, 69, 70, 150, 171, 80} {
		m.Update(tea.WindowSizeMsg{Width: width, Height: 30})
		if !m.panelFocus || m.input.Focused() || m.panelRows()[m.panelIndex].key() != selected || m.current().Id != "s" || m.input.Value() != "unfinished draft" {
			t.Fatal("resize lost navigation state", width)
		}
		view := ansi.Strip(m.View())
		if !strings.Contains(view, "Projects") || !strings.Contains(view, "maple") {
			t.Fatal("missing shared navigator", width)
		}
		if strings.Contains(view, "unfinished draft") != (width >= 70) {
			t.Fatal("conversation visibility does not match available space", width)
		}
		for _, row := range strings.Split(view, "\n") {
			if ansi.StringWidth(row) != width {
				t.Fatal("layout width", width, ansi.StringWidth(row))
			}
		}
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.panelFocus || !m.input.Focused() || m.current().Id != "s" || m.input.Value() != "unfinished draft" {
		t.Fatal("return did not preserve conversation")
	}
}

func TestNarrowNavigatorOpensOtherProjectSession(t *testing.T) {
	m := panelModel()
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	m.input.SetValue("saved draft")
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlQ})
	for i, r := range m.panelRows() {
		if r.session != nil && r.session.Id == "other-session" {
			m.panelIndex = i
		}
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.panelFocus || m.projectView || m.current().Id != "other-session" || m.drafts["s"] != "saved draft" {
		t.Fatal("narrow selection did not open destination")
	}
}

func TestNavigatorEntryFocusesRequestedProject(t *testing.T) {
	m := conversationModel()
	target := &api.Project{Id: "target", Name: "Z target"}
	m.initializeNavigation(target, "")
	m.updatePanel(listing{projects: []*api.Project{{Id: "first", Name: "A first"}, target}, projectsLoaded: true, sessions: []*api.Session{{Id: "child", ProjectId: "target"}}})
	if !m.panelFocus || !m.projectView || m.panelRows()[m.panelIndex].key() != "p:target" {
		t.Fatal("startup did not select workspace project")
	}
	m.panelKey(tea.KeyMsg{Type: tea.KeyEnter})
	if !m.panelFocus || m.panelRows()[m.panelIndex].key() != "s:child" {
		t.Fatal("project opened an obsolete screen")
	}
	m.panelKey(tea.KeyMsg{Type: tea.KeyEnter})
	if m.projectView || m.panelFocus || m.current().Id != "child" {
		t.Fatal("session not opened")
	}
}

type navigatorClient struct {
	api.SessionsClient
	stopped, deleted string
	run              string
}

func (c *navigatorClient) Stop(_ context.Context, r *api.Control, _ ...grpc.CallOption) (*api.Receipt, error) {
	c.stopped = r.SessionId
	c.run = r.RunId
	return &api.Receipt{}, nil
}
func (c *navigatorClient) DeleteSession(_ context.Context, id string) error {
	c.deleted = id
	return nil
}

func TestNavigatorActionsUseSelectedSessionAndProject(t *testing.T) {
	for _, width := range []int{69, 80, 200} {
		m := panelModel()
		m.Update(tea.WindowSizeMsg{Width: width, Height: 30})
		m.focusPanel()
		c := &navigatorClient{}
		m.client = c
		for i, r := range m.panelRows() {
			if r.session != nil && r.session.Id == "other-session" {
				m.panelIndex = i
			}
		}
		stop := m.panelKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
		stop()
		if c.stopped != "other-session" || c.run != "other-run" || m.current().Id != "s" {
			t.Fatal("stop targeted viewed session")
		}
		m.busy = false
		m.panelKey(tea.KeyMsg{Type: tea.KeyCtrlX})
		confirm := m.panelKey(tea.KeyMsg{Type: tea.KeyCtrlX})
		m.panelIndex = 0 // Once submitted, navigation must not change the RPC target.
		confirm()
		if c.deleted != "other-session" {
			t.Fatal("delete target drifted")
		}
		m.busy = false
		for i, r := range m.panelRows() {
			if r.project.Id == "other" && r.session == nil {
				m.panelIndex = i
			}
		}
		m.panelKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
		if !m.accountView || m.project.Id != "other" {
			t.Fatal("accounts targeted original project")
		}
		m.accountKey(tea.KeyMsg{Type: tea.KeyEsc})
		if !m.panelFocus || m.accountView {
			t.Fatal("accounts did not return to navigator")
		}
		m.createProjectSession = func(_ context.Context, project, account string, _ io.Reader, _ io.Writer, _ io.Writer) (*api.Session, error) {
			if project != "other" {
				t.Error("creation targeted original project")
			}
			return &api.Session{Id: "new"}, nil
		}
		m.panelKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
		if !m.accountChoosing || !m.accountView || m.project.Id != "other" {
			t.Fatal("new session did not open account choice")
		}
		cmd := m.startAccountWorkflow("main", "codex", "", true)
		batch := cmd().(tea.BatchMsg)
		if done := batch[0]().(workflowDone); done.err != nil {
			t.Fatal(done.err)
		}
		m.workflow.close()
	}
}

func TestProjectNavigatorHasBackgroundAndOnlyFocusedSideBottomRule(t *testing.T) {
	profile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(profile)
	m := panelModel()
	for _, width := range []int{69, 80, 200} {
		m.Update(tea.WindowSizeMsg{Width: width, Height: 30})
		for _, focus := range []bool{false, true} {
			m.panelFocus = focus
			rendered := m.panelScreen()
			plain := ansi.Strip(rendered)
			if strings.ContainsAny(plain, "╭╮╰╯│") {
				t.Fatal("outer border remains")
			}
			if strings.Contains(strings.Split(plain, "\n")[m.height-1], "─") != (width >= 70 && focus) {
				t.Fatal("incorrect focus rule")
			}
			if !strings.Contains(rendered, "48;2;48;48;48") {
				t.Fatal("background missing")
			}
		}
	}
}

func TestNavigatorBackgroundCoversEveryTerminalCell(t *testing.T) {
	profile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(profile)
	for _, width := range []int{69, 80, 150, 200} {
		for _, focused := range []bool{false, true} {
			m := panelModel()
			m.Update(tea.WindowSizeMsg{Width: width, Height: 30})
			m.panelFocus = focused
			if width < 70 {
				m.projectView = true
			}
			terminal := vt.NewEmulator(width, m.height)
			terminal.WriteString(strings.ReplaceAll(m.View(), "\n", "\r\n"))
			panelWidth := m.panelWidth()
			if width < 70 {
				panelWidth = width
			}
			for y := 0; y < m.height; y++ {
				gray := uint32(0x3030)
				if focused && y == projectPanelHeaderRows+m.panelPosition(panelLayout(m.panelRows())) {
					gray = 0x4444
				}
				for x := 0; x < panelWidth; x++ {
					cell := terminal.CellAt(x, y)
					if cell == nil || cell.Style.Bg == nil {
						t.Fatalf("unpainted cell %d,%d at width %d focus %v", x, y, width, focused)
					}
					r, g, b, _ := cell.Style.Bg.RGBA()
					if r != gray || g != gray || b != gray {
						t.Fatalf("wrong background at %d,%d: %x %x %x", x, y, r, g, b)
					}
				}
			}
			if panelWidth < width {
				if cell := terminal.CellAt(panelWidth, 0); cell != nil && cell.Style.Bg != nil {
					t.Fatal("panel background leaked into gap")
				}
			}
			terminal.Close()
		}
	}
}

func TestProjectPanelOneCellPadding(t *testing.T) {
	m := panelModel()
	m.panelFocus = true
	rows := strings.Split(ansi.Strip(m.panelScreen()), "\n")
	if strings.TrimSpace(rows[0]) != "" {
		t.Fatal("top padding missing")
	}
	for i, row := range rows[:len(rows)-1] {
		if !strings.HasPrefix(row, " ") || !strings.HasSuffix(row, " ") {
			t.Fatalf("side padding missing on row %d: %q", i, row)
		}
	}
	m.panelFocus = false
	rows = strings.Split(ansi.Strip(m.panelScreen()), "\n")
	if strings.TrimSpace(rows[len(rows)-1]) != "" {
		t.Fatal("bottom padding missing")
	}
}

// The footer spends no width repeating a key that already opens its label: the
// letter is coloured in place. Keys a label cannot carry keep their own column.
func TestHintMarksTheKeyInPlace(t *testing.T) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(old)
	folded := hintSpec{key: "n", label: "new"}
	if got, want := folded.render(false), accent.Render("n")+muted.Render("ew"); got != want {
		t.Fatalf("leading letter not marked as the key: %q", got)
	}
	// Ctrl+. cannot be folded into "settings", so it stays in front.
	prefixed := hintSpec{key: "Ctrl+.", label: "settings"}
	if got, want := prefixed.render(false), accent.Render("Ctrl+.")+muted.Render(" settings"); got != want {
		t.Fatalf("multi-key shortcut lost its prefix: %q", got)
	}
	// text() is what the mouse measures, so it must match what was drawn.
	for _, h := range []hintSpec{folded, prefixed, {label: "agents run", note: true}} {
		if plain := ansi.Strip(h.render(false)); plain != h.text() {
			t.Fatalf("width source disagrees with the drawing: %q vs %q", h.text(), plain)
		}
	}
	off := hintSpec{key: "r", label: "rename", disabled: true}
	if dim := off.render(false); !strings.Contains(dim, zeroStyle.Render("r")) {
		t.Fatalf("disabled hint kept the key colour: %q", dim)
	}
	if hot := folded.render(true); hot == folded.render(false) || !strings.Contains(hot, strong.Render("ew")) {
		t.Fatalf("hover left the hint unchanged: %q", hot)
	}
}

func TestPanelFooterFoldsSingleLetterKeys(t *testing.T) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(old)
	m := panelModel()
	m.Update(tea.WindowSizeMsg{Width: 90, Height: 22})
	plain := ansi.Strip(m.panelScreen())
	for _, want := range []string{"new · accounts", "rename · stop", "memory · Ctrl+. settings", "Ctrl+D detach · agents run"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("missing footer row %q in %q", want, plain)
		}
	}
	for _, gone := range []string{"n new", "a accounts", "r rename", "m memory"} {
		if strings.Contains(plain, gone) {
			t.Fatalf("key still spelled beside its label: %q", gone)
		}
	}
}

// A hint is only offered when the selected row can act on it, and clicking one
// sends its key so the mouse and the keyboard cannot disagree about the effect.
func TestPanelFooterHintsHoverClickAndDisable(t *testing.T) {
	m := panelModel()
	m.Update(tea.WindowSizeMsg{Width: 90, Height: 22})
	m.focusPanel()
	// The footer is drawn into the rows reserved for it, above the status row.
	if len(m.panelHints()) != projectPanelFooterRows-1 {
		t.Fatal("footer rows and reserved height disagree", len(m.panelHints()), projectPanelFooterRows)
	}
	rows := m.panelRows()
	session, project := -1, -1
	for i, r := range rows {
		if r.session != nil && session < 0 {
			session = i
		}
		if r.session == nil && project < 0 {
			project = i
		}
	}
	if session < 0 || project < 0 {
		t.Fatal("fixture needs both a project row and a session row")
	}

	// "rename" needs a session; a project row must not advertise it.
	m.panelIndex = project
	if hintAt(m.panelHints()[1], 0).disabled != true {
		t.Fatal("rename offered on a project row")
	}
	m.panelIndex = session
	rename := hintAt(m.panelHints()[1], 0)
	if rename.key != "r" || rename.disabled {
		t.Fatal("rename not offered on a session row", rename)
	}

	// Hover follows the pointer and clears when it leaves the row.
	hover := tea.MouseMsg{X: panelHintX, Y: m.panelHintY(1), Action: tea.MouseActionMotion}
	if handled, _ := m.panelMouse(hover); !handled || m.panelHintHover != "r" {
		t.Fatal("hover not tracked", m.panelHintHover)
	}
	if _, _ = m.panelMouse(tea.MouseMsg{X: 0, Y: 0, Action: tea.MouseActionMotion}); m.panelHintHover != "" {
		t.Fatal("hover outlived the pointer")
	}

	// Clicking sends the key rather than acting directly.
	click := tea.MouseMsg{X: panelHintX, Y: m.panelHintY(1), Action: tea.MouseActionPress, Button: tea.MouseButtonLeft}
	_, cmd := m.panelMouse(click)
	if cmd == nil {
		t.Fatal("click produced no key")
	}
	if got, ok := cmd().(tea.KeyMsg); !ok || got.String() != "r" {
		t.Fatalf("click sent the wrong key: %#v", cmd())
	}

	// A disabled hint is inert: no hover, no key.
	m.panelIndex = project
	m.panelHintHover = ""
	if _, cmd := m.panelMouse(click); cmd != nil || m.panelHintHover != "" {
		t.Fatal("disabled hint reacted to a click")
	}
	// The separator and the note belong to no hint.
	settings := m.panelHints()[3]
	if gap := hintAt(settings, ansi.StringWidth(settings[0].text())); gap.key != "" {
		t.Fatal("separator claimed by a hint", gap)
	}
	if note := hintAt(m.panelHints()[5], 99); note.actionable() {
		t.Fatal("trailing space is actionable")
	}
}

func TestPanelHidesDownProjectAndRestoresItAfterUp(t *testing.T) {
	m := panelModel()
	running := &api.Project{Id: "running", Name: "A running", State: "running"}
	target := &api.Project{Id: "target", Name: "Z target", State: "running"}
	sessions := []*api.Session{{Id: "saved", ProjectId: "target", State: "stopped"}}
	update := func(p *api.Project) {
		m.updatePanel(listing{projects: []*api.Project{running, p}, projectsLoaded: true, sessions: sessions})
	}
	update(target)
	m.panelIndex = 2 // Selected session belongs to the project being taken down.
	down := proto.Clone(target).(*api.Project)
	down.State, down.ProvisionState, down.ProvisionStep = "absent", "complete", "down"
	update(down)
	rows := m.panelRows()
	if len(rows) != 1 || rows[0].project.Id != "running" || m.panelIndex != 0 {
		t.Fatal("down project/session still selectable", rows, m.panelIndex)
	}
	if len(m.panelProjects) != 2 || len(m.allSessions) != 1 {
		t.Fatal("hiding navigation discarded retained metadata")
	}
	if view := m.panelScreen(); strings.Contains(ansi.Strip(view), "Z target") {
		t.Fatal("down project still rendered", view)
	}
	update(target)
	rows = m.panelRows()
	if len(rows) != 3 || rows[2].session.Id != "saved" {
		t.Fatal("up did not restore saved session navigation", rows)
	}
	// The empty list remains safe for keyboard navigation.
	m.updatePanel(listing{projects: []*api.Project{down}, projectsLoaded: true, sessions: sessions})
	if len(m.panelRows()) != 0 || m.panelIndex != 0 {
		t.Fatal("hidden-only list selection", m.panelIndex)
	}
	m.focusPanel()
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
}

func TestPanelKeepsProjectsThatWereNotExplicitlyDowned(t *testing.T) {
	for _, p := range []*api.Project{
		{Id: "registered", State: "absent"},
		{Id: "stopped", State: "stopped"},
		{Id: "failed", State: "absent", ProvisionState: "failed", ProvisionStep: "configuration"},
		{Id: "starting", State: "absent", ProvisionState: "running", ProvisionStep: "inventory"},
		{Id: "restarted", State: "running", ProvisionState: "complete", ProvisionStep: "down"},
		{Id: "remote", State: "connection"},
	} {
		t.Run(p.Id, func(t *testing.T) {
			m := panelModel()
			m.updatePanel(listing{projects: []*api.Project{p}, projectsLoaded: true})
			if rows := m.panelRows(); len(rows) != 1 || rows[0].project.Id != p.Id {
				t.Fatal("unrelated project hidden", rows)
			}
		})
	}
}
