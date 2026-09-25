package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lesomnus/cxz/internal/transport"
)

func TestHostPathHintsReadClientFilesystemEvenOnRemoteConnection(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "자료"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "report.txt"), []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}
	m := pathModel()
	m.ctx = transport.WithRemote(m.ctx)
	client := &remotePathClient{}
	m.client = client
	m.project, m.panelProjects = nil, nil
	m.input.SetValue("참고 `!" + filepath.ToSlash(dir) + "/")
	m.syncPathHints()
	if m.pathHints == nil || !m.pathHints.token.host {
		t.Fatal("host scope missing")
	}
	old := m.pathHints.generation
	reply := m.fetchPathHints(pathHintDue{old})().(pathHintResult)
	m.receivePathHints(reply)
	if reply.err != nil || client.path != "" || m.wisp != nil {
		t.Fatal("host lookup reached container", reply.err, client.path)
	}
	options := m.pathOptions()
	if len(options) != 2 || options[0].entry.Name != "자료" || options[1].entry.Name != "report.txt" {
		t.Fatal("client entries unavailable", options)
	}
	if view := m.pathHintOverlay(strings.Repeat("row\n", 15)); !strings.Contains(view, "Host files") {
		t.Fatal("scope not shown", view)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !strings.HasSuffix(m.input.Value(), "/자료/") || !strings.Contains(m.input.Value(), "`!") {
		t.Fatal("directory selection closed host reference", m.input.Value())
	}
	// Identical spelling in the other scope must not reuse host entries.
	m.input.SetValue("참고 `" + filepath.ToSlash(dir) + "/")
	m.syncPathHints()
	if m.pathHints.token.host || m.pathHints.generation == old || len(m.pathOptions()) != 0 {
		t.Fatal("host listing reused as container listing")
	}
	m.receivePathHints(reply)
	if len(m.pathOptions()) != 0 {
		t.Fatal("stale host listing crossed scopes")
	}
}

func TestHostPathCompletionProducesAttachmentAtCursor(t *testing.T) {
	m := pathModel()
	path := testHostFile(t, "한글 report.txt")
	parent := filepath.ToSlash(filepath.Dir(path)) + "/"
	before := "먼저 `!\"" + parent + "한"
	m.setPathInput(before+"` 뒤 설명", len([]rune(before)))
	m.syncPathHints()
	reply := m.fetchPathHints(pathHintDue{m.pathHints.generation})().(pathHintResult)
	m.receivePathHints(reply)
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if want := "먼저 `!\"" + filepath.ToSlash(path) + "\"` 뒤 설명"; m.input.Value() != want {
		t.Fatal("host completion lost marker/quotes", m.input.Value())
	}
	_ = resolveHostFiles(t, m)
	if !strings.HasPrefix(m.input.Value(), "먼저 [한글 report.txt ") || !strings.HasSuffix(m.input.Value(), " 뒤 설명") {
		t.Fatal("host completion did not become inline chip", m.input.Value())
	}
	for _, p := range m.pastes {
		p.attachment.cancel()
	}
}

func TestPathCompletionNeverReplacesLaterLines(t *testing.T) {
	m := pathModel()
	m.setPathInput("`/tmp/한\n다음 문장", len([]rune("`/tmp/한")))
	m.syncPathHints()
	m.applyPathOption(pathOption{text: "/tmp/한글.txt"})
	if m.input.Value() != "`/tmp/한글.txt\n다음 문장" {
		t.Fatal("completion consumed next line", m.input.Value())
	}
}

func TestHostPathWordDeleteRetainsNamespace(t *testing.T) {
	m := pathModel()
	m.input.SetValue("설명 `!/tmp/file.txt")
	for _, want := range []string{"설명 `!/tmp/", "설명 `!/", "설명 `!"} {
		m.Update(tea.KeyMsg{Type: tea.KeyCtrlW})
		if m.input.Value() != want {
			t.Fatal("host namespace lost", m.input.Value(), want)
		}
	}
}
