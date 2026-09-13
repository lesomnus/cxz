package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

func TestQuestionPreviewBoxAlignment(t *testing.T) {
	for _, width := range []int{20, 36, 74, 128} {
		view := ansi.Strip(questionPreview("/model\n  default\n  한글 모델\n\n```go\nfmt.Println(\"hello\")\n```", width))
		rows := strings.Split(view, "\n")
		if !strings.HasPrefix(rows[0], "    ╭─ Preview ") || !strings.HasPrefix(rows[len(rows)-1], "    ╰") {
			t.Fatal(view)
		}
		for _, row := range rows {
			if ansi.StringWidth(row) != width {
				t.Fatalf("width %d: got %d %q", width, ansi.StringWidth(row), row)
			}
		}
	}
}

func TestQuestionButtonsAndPendingSpace(t *testing.T) {
	m := questionModel()
	m.resize()
	before := m.view.Height
	if m.approvalHeight() == 0 {
		t.Fatal("missing initial pending panel")
	}
	m.openQuestion(m.selectedApproval())
	if m.approvalBox() != "" || m.approvalHeight() != 0 || m.view.Height <= before {
		t.Fatal("pending panel space not reclaimed")
	}
	view := ansi.Strip(m.sessionScreen())
	found := false
	for _, row := range strings.Split(view, "\n") {
		if strings.Contains(row, "[ Next ]") && strings.Contains(row, "[ Back ]") && strings.Contains(row, "[ Cancel ]") {
			found = true
		}
	}
	if !found || strings.Contains(view, "Pending questions") {
		t.Fatal(view)
	}
	d := m.questionDialog
	d.row = d.count()
	m.Update(tea.KeyMsg{Type: tea.KeyRight})
	if d.row != d.count()+1 {
		t.Fatal("right did not select Back")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	if d.row != d.count() {
		t.Fatal("left did not select Next")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.view.Height != before || !strings.Contains(m.approvalBox(), "Pending questions") {
		t.Fatal("pending area not restored")
	}
}

func TestQuestionMagentaAndTextCheckboxes(t *testing.T) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(old) })
	m := questionModel()
	m.syncQuestion()
	d := m.questionDialog
	d.page = 1
	d.selected[1][0] = true
	d.row = 1
	view := m.questionOverlay(strings.Repeat("\n", 20))
	for _, want := range []string{magenta.Render("  [✓] X"), magenta.Bold(true).Render("› [ ] Y")} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing selected color: %q", view)
		}
	}
	for state, marker := range map[string]string{"requested": "[ ]", "allowed": "[✓]", "denied": "[×]", "canceled": "[×]"} {
		got := ansi.Strip(approvalLine(m.current(), m.selectedApproval(), state, 80))
		if !strings.Contains(got, marker) {
			t.Fatal(got)
		}
	}
}
