package tui

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
)

func toolViewModel() *model {
	m := conversationModel()
	m.current().Agent = "claude"
	m.events["s"] = []*api.Event{
		{Seq: 1, RunId: "run", Kind: "input", Text: "inspect"},
		{Seq: 2, RunId: "run", Kind: "tool_call", Text: "Write", RequestId: "w", Payload: []byte(`{"file_path":"main.go","content":"package main"}`)},
		{Seq: 3, RunId: "run", Kind: "tool_result", RequestId: "w", Payload: []byte(`{"content":"written"}`)},
		{Seq: 4, RunId: "run", Kind: "assistant", Text: "Some explanation"},
		{Seq: 5, RunId: "run", Kind: "tool_call", Text: "Read", RequestId: "r", Payload: []byte(`{"file_path":"main.go"}`)},
		{Seq: 6, RunId: "run", Kind: "tool_result", RequestId: "r", Payload: []byte(fmt.Sprintf(`{"content":%q}`, strings.Repeat("package main\n", 30)))},
	}
	m.render()
	return m
}
func TestViewSelectsRenderedToolsAndFocusesPreview(t *testing.T) {
	m := toolViewModel()
	m.input.SetValue("draft text")
	m.viewCommand("/view")
	targets := m.toolTargets()
	if len(targets) != 2 || targets[0].seq != 2 || targets[1].seq != 5 {
		t.Fatal(targets)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyHome})
	if m.toolSelector.seq != 2 || !strings.Contains(ansi.Strip(m.conversationView()), "› ") {
		t.Fatal("missing selection marker")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.toolSelector.seq != 5 {
		t.Fatal("selected non-tool row")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.filePreview == nil || !m.filePreview.focused || m.filePreview.language != "main.go" || !strings.Contains(m.filePreview.source, "package main") {
		t.Fatal("Read result not opened with focus")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	if m.filePreview.offset == 0 {
		t.Fatal("keyboard did not scroll")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}, Paste: true})
	if m.filePreview == nil {
		t.Fatal("pasted x closed preview")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if m.filePreview != nil || !m.selectingTools() || m.input.Value() != "draft text" {
		t.Fatal("close lost selection or edited draft")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.selectingTools() || !m.input.Focused() {
		t.Fatal("Esc did not restore composer")
	}
}
func TestPreviewMouseAndKeyboardSharePayloadAndBorders(t *testing.T) {
	m := toolViewModel()
	if !m.openPreviewSequence(2) {
		t.Fatal("open")
	}
	source := m.filePreview.source
	if source != "package main" {
		t.Fatal("Write should preview input")
	}
	for _, width := range []int{30, 80, 180} {
		text := ansi.Strip(m.previewRows(width, 8))
		rows := strings.Split(text, "\n")
		if len(rows) != 8 || !strings.Contains(rows[0], "Input") || !strings.Contains(rows[0], "Output") || !strings.Contains(rows[1], "package main") || !strings.Contains(rows[7], "─") {
			t.Fatal(text)
		}
		for _, row := range rows {
			if ansi.StringWidth(row) != width {
				t.Fatalf("width %d: %q", width, row)
			}
		}
	}
	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.filePreview.focused || !m.input.Focused() {
		t.Fatal("Tab did not leave preview")
	}
	top := m.view.Height + 1 + m.approvalHeight()
	m.filePreviewMouse(tea.MouseMsg{X: m.contentOffset() + 4, Y: top + 2, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if !m.filePreview.focused {
		t.Fatal("click did not focus")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if m.filePreview != nil {
		t.Fatal("mouse-opened preview ignored x")
	}
	m.viewCommand("/view")
	m.Update(tea.KeyMsg{Type: tea.KeyHome})
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.filePreview.source != source {
		t.Fatal("keyboard preview differs")
	}
}
func TestViewPreservesSelectionAsEventsArriveAndDoesNotCrossRuns(t *testing.T) {
	m := toolViewModel()
	m.viewCommand("/view")
	m.Update(tea.KeyMsg{Type: tea.KeyHome})
	for i := uint64(7); i < 100; i++ {
		m.events["s"] = append(m.events["s"], &api.Event{Seq: i, Kind: "assistant", Text: "incoming output"})
	}
	m.render()
	if m.toolSelector.seq != 2 || !strings.Contains(ansi.Strip(m.conversationView()), "› ") {
		t.Fatal("new events displaced selection")
	}
	m.events["s"] = append(m.events["s"], &api.Event{Seq: 100, RunId: "new-run", Kind: "tool_result", RequestId: "r", Payload: []byte(`{"content":"wrong run"}`)})
	p := m.previewForSequence(5)
	if strings.Contains(p.source, "wrong run") {
		t.Fatal("matched another run")
	}
	m.openReport("/help", "")
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.report != nil || !m.selectingTools() {
		t.Fatal("selector intercepted modal key")
	}
}

func TestViewCommandIsLocalAndResizeKeepsSelection(t *testing.T) {
	m := toolViewModel()
	m.input.SetValue("/view")
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if !m.selectingTools() || len(m.client.(*recordingClient).inputs) != 0 {
		t.Fatal("view was sent to agent")
	}
	seq := m.toolSelector.seq
	m.Update(tea.WindowSizeMsg{Width: 180, Height: 35})
	if m.toolSelector.seq != seq {
		t.Fatal("resize changed selected event")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.filePreview.focused || !m.selectingTools() || m.input.Focused() {
		t.Fatal("Tab did not return to selector")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if m.toolSelector.seq != 2 {
		t.Fatal("selection keys did not resume")
	}
}

func TestToolPreviewInputOutputTabs(t *testing.T) {
	m := toolViewModel()
	m.openPreviewSequence(5)
	if m.filePreview.tab != 1 {
		t.Fatal("Read should open output")
	}
	m.previewRows(60, 8)
	m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	offset := m.filePreview.offset
	m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	if m.filePreview.tab != 0 || !strings.Contains(m.filePreview.source, `"file_path": "main.go"`) || m.filePreview.offset != 0 {
		t.Fatal("input not selected", m.filePreview)
	}
	rendered := ansi.Strip(m.previewRows(60, 8))
	if !strings.Contains(rendered, "file_path") || strings.Contains(rendered, "package main") {
		t.Fatal("stale output render", rendered)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRight})
	if m.filePreview.tab != 1 || m.filePreview.offset != offset || !strings.Contains(m.filePreview.source, "package main") {
		t.Fatal("output/scroll not restored")
	}
	m.openPreviewSequence(2)
	m.Update(tea.KeyMsg{Type: tea.KeyRight})
	if m.filePreview.source != "written" {
		t.Fatal("Write output hidden", m.filePreview.source)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	if m.filePreview.source != "package main" {
		t.Fatal("Write input lost")
	}
}

func TestToolPreviewTabClicksInBothLayouts(t *testing.T) {
	for _, width := range []int{80, 240} {
		m := toolViewModel()
		m.Update(tea.WindowSizeMsg{Width: width, Height: 36})
		m.openPreviewSequence(2)
		x, y := m.contentOffset()+2, m.view.Height+1+m.approvalHeight()+m.questionHeight()
		w, h := max(1, m.width-4), m.previewHeight()
		if side := m.previewSideWidth(); side > 0 {
			x, y, w, h = m.contentOffset()+m.width+2, 0, side, m.height
		}
		m.Update(tea.MouseMsg{X: x + 11, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
		if m.filePreview.tab != 1 || m.filePreview.source != "written" || !m.filePreview.focused {
			t.Fatal("output click", width)
		}
		var copied bytes.Buffer
		m.cursorOutput = &cursorWriter{out: &copied}
		m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
		if !strings.Contains(copied.String(), ansi.SetSystemClipboard("written")) {
			t.Fatal("did not copy active output")
		}
		rows := strings.Split(ansi.Strip(m.previewRows(w, h)), "\n")
		if !strings.Contains(rows[0], "Input") || !strings.Contains(rows[0], "Output") || !strings.Contains(rows[1], "written") {
			t.Fatal(rows)
		}
		for _, row := range rows {
			if ansi.StringWidth(row) != w {
				t.Fatal("panel width changed", row)
			}
		}
		m.Update(tea.MouseMsg{X: x + 3, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
		if m.filePreview.tab != 0 || m.filePreview.source != "package main" {
			t.Fatal("input click", width)
		}
	}
}

func TestCodexPreviewTabsAndMissingPayloads(t *testing.T) {
	m := conversationModel()
	m.current().Agent = "codex"
	m.events["s"] = []*api.Event{
		{Seq: 1, RunId: "r", Kind: "tool_call", RequestId: "cmd", Payload: []byte(`{"item":{"type":"commandExecution","command":"echo hello"}}`)},
		{Seq: 2, RunId: "r", Kind: "tool_result", RequestId: "cmd", Payload: []byte(`{"item":{"type":"commandExecution","aggregatedOutput":"hello"}}`)},
		{Seq: 3, RunId: "other", Kind: "tool_result", RequestId: "cmd", Payload: []byte(`{"item":{"type":"commandExecution","aggregatedOutput":"wrong run"}}`)},
	}
	p := m.previewForSequence(2)
	if p.source != "hello" {
		t.Fatal(p.source)
	}
	p.selectTab(0)
	if p.source != "echo hello" || p.language != "bash" {
		t.Fatal(p)
	}
	m.events["s"] = m.events["s"][:1]
	p = m.previewForSequence(1)
	if p.source != "echo hello" {
		t.Fatal("pending input missing")
	}
	p.selectTab(1)
	if p.source != "No output available yet." {
		t.Fatal(p.source)
	}
	m.events["s"] = []*api.Event{{Seq: 4, Kind: "tool_result", Text: "orphan output"}}
	p = m.previewForSequence(4)
	if p.source != "orphan output" {
		t.Fatal(p.source)
	}
	p.selectTab(0)
	if !strings.Contains(p.source, "not available") {
		t.Fatal("missing input not explained")
	}
}
