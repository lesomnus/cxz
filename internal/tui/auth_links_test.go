package tui

import (
	"bytes"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func TestAuthURLWrapPreservesFullTarget(t *testing.T) {
	link := "https://example.invalid/authorize?code=true&challenge=" + strings.Repeat("abc-_123", 70) + "&state=end"
	for _, width := range []int{40, 80, 133} {
		rows, got := workflowAuthOutput("\x1b[?1049lIf browser did not open: "+link+"\nPaste code >", width)
		if got != link {
			t.Fatal("URL changed")
		}
		var visible strings.Builder
		for _, row := range rows {
			if ansi.StringWidth(row) > width || strings.Contains(row, "\x1b[?1049l") {
				t.Fatal("unsafe layout")
			}
			if strings.Contains(row, ansi.SetHyperlink(link)) {
				visible.WriteString(ansi.Strip(row))
				if !strings.HasSuffix(row, ansi.ResetHyperlink()) {
					t.Fatal("unclosed hyperlink")
				}
			}
		}
		if visible.String() != link {
			t.Fatal("visible URL truncated or padded")
		}
	}
}

func TestAuthURLWaitsForCompleteOutput(t *testing.T) {
	partial := "Visit https://example.invalid/login?state=part"
	if _, link := workflowAuthOutput(partial, 40); link != "" {
		t.Fatal("partial URL actionable")
	}
	if _, link := workflowAuthOutput(partial+"two\n", 40); link != "https://example.invalid/login?state=parttwo" {
		t.Fatal("chunk reassembly failed")
	}
	for _, raw := range []string{"file:///tmp/secret\n", "javascript:alert(1)\n", "https://user:password@example.invalid/\n"} {
		if _, link := workflowAuthOutput(raw, 40); link != "" {
			t.Fatal("unsafe URL", link)
		}
	}
}

func TestWorkflowCopyUsesOriginalURLNotCode(t *testing.T) {
	m := projectModel()
	link := "https://example.invalid/login?state=" + strings.Repeat("x", 500)
	m.workflow = &accountWorkflow{provider: "claude", output: "Visit " + link + "\n"}
	var terminal bytes.Buffer
	m.cursorOutput = &cursorWriter{out: &terminal}
	m.workflowKey(tea.KeyMsg{Type: tea.KeyCtrlY})
	if terminal.String() != ansi.SetSystemClipboard(link) {
		t.Fatal("copy did not preserve original URL")
	}
	if !strings.Contains(m.workflow.message, "requested") {
		t.Fatal("claimed unverified clipboard success")
	}
	m.width, m.height = 80, 30
	view := m.workflowScreen()
	if !strings.Contains(view, ansi.SetHyperlink(link)) {
		t.Fatal("screen stripped hyperlink target")
	}
	for _, row := range strings.Split(view, "\n") {
		if strings.Contains(row, ansi.SetHyperlink(link)) && strings.HasPrefix(ansi.Strip(row), " ") {
			t.Fatal("URL row has left margin")
		}
		if ansi.StringWidth(row) > m.width {
			t.Fatal("screen overflow")
		}
	}
}
