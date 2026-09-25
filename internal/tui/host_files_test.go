package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lesomnus/cxz/api"
)

func resolveHostFiles(t *testing.T, m *model) tea.Cmd {
	t.Helper()
	m.syncHostFiles()
	if m.hostFileCheck == nil {
		t.Fatal("host attachment check not scheduled")
	}
	cmd := m.checkHostFiles(hostFileDue{m.hostFileCheck})
	if cmd == nil {
		t.Fatal("host check discarded")
	}
	return m.receiveHostFiles(cmd().(hostFileChecked))
}
func testHostFile(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestHostFilesRequireExplicitMarker(t *testing.T) {
	path := testHostFile(t, "file.txt")
	for _, text := range []string{path, "`" + path + "`", "!" + path, "`word !" + path + "`"} {
		m := conversationModel()
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(text), Paste: true})
		if m.hostFileCheck != nil || len(m.pastes) != 0 || m.input.Value() != text {
			t.Fatalf("ordinary input became attachment: %q", text)
		}
	}
}

func TestHostFileSplitInputDiscardsStaleChecks(t *testing.T) {
	m := conversationModel()
	path := testHostFile(t, "한글 report.txt")
	m.input.InsertString("앞 문장 `!")
	for _, r := range "'" + path + "'" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	if len(m.pastes) != 0 || m.hostFileCheck == nil {
		t.Fatal("split input bypassed debounce")
	}
	old := m.hostFileCheck
	pending := m.checkHostFiles(hostFileDue{old})
	// A later character invalidates both a queued timer and an in-flight result.
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	if m.checkHostFiles(hostFileDue{old}) != nil {
		t.Fatal("stale timer checked partial input")
	}
	m.receiveHostFiles(pending().(hostFileChecked))
	if len(m.pastes) != 0 {
		t.Fatal("stale result replaced newer text")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	_ = resolveHostFiles(t, m)
	if len(m.pastes) != 1 || !strings.HasPrefix(m.input.Value(), "앞 문장 [한글 report.txt ") || strings.Contains(m.input.Value(), "`!") {
		t.Fatal("complete split path not attached", m.input.Value())
	}
	// Starting an upload is enough for this test; no client I/O is run.
	for _, p := range m.pastes {
		p.attachment.cancel()
	}
}

func TestInlineHostAttachmentsPreserveProseAndCursor(t *testing.T) {
	m := conversationModel()
	a, b := testHostFile(t, "첫 파일.txt"), testHostFile(t, "second.txt")
	prefix := "앞 문장\n"
	marker := "`!'" + a + "' '" + b + "'`"
	suffix := " 뒤 문장\n다음 줄"
	m.setPathInput(prefix+marker+suffix, len([]rune(prefix+marker+suffix)))
	_ = resolveHostFiles(t, m)
	if len(m.pastes) != 2 || !strings.HasPrefix(m.input.Value(), prefix+"[첫 파일.txt ") || !strings.Contains(m.input.Value(), "] [second.txt ") || !strings.HasSuffix(m.input.Value(), suffix) {
		t.Fatal("prose or attachments changed", m.input.Value())
	}
	_, pos, _, _, _ := m.chipInput()
	if pos != len([]rune(m.input.Value())) {
		t.Fatal("cursor moved", pos)
	}
	for _, p := range m.pastes {
		p.attachment.cancel()
	}
}

func TestHostAttachmentKeepsTextAfterCursor(t *testing.T) {
	m := conversationModel()
	path := testHostFile(t, "image.png")
	before := "설명 `!" + path
	m.setPathInput(before+"` 뒤 문장", len([]rune(before)))
	_ = resolveHostFiles(t, m)
	value, pos, _, _, _ := m.chipInput()
	if !strings.HasSuffix(value, " 뒤 문장") || string([]rune(value)[pos:]) != " 뒤 문장" {
		t.Fatal("cursor/suffix not preserved", value, pos)
	}
	for _, p := range m.pastes {
		p.attachment.cancel()
	}
}

func TestHostFilesStayEditableUntilValid(t *testing.T) {
	for _, input := range []string{"`!", "`!/no/such/file", "`!'unfinished", "`!" + t.TempDir()} {
		m := conversationModel()
		m.input.InsertString(input)
		if cmd := resolveHostFiles(t, m); cmd != nil || len(m.pastes) != 0 || m.input.Value() != input {
			t.Fatal("incomplete path lost", input)
		}
		m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
		if len(m.client.(*recordingClient).inputs) != 0 || m.input.Value() != input {
			t.Fatal("unresolved host path sent", input)
		}
	}
}

func TestHostFilesDiscardResultsAfterSessionOrRunChanges(t *testing.T) {
	for _, change := range []func(*model){
		func(m *model) { m.current().Id = "other" },
		func(m *model) { m.current().RunId = "other" },
		func(m *model) { m.input.Reset() },
		func(m *model) { m.input.Blur() },
	} {
		m := conversationModel()
		m.input.InsertString("`!" + testHostFile(t, "x.txt"))
		m.syncHostFiles()
		cmd := m.checkHostFiles(hostFileDue{m.hostFileCheck})
		change(m)
		m.receiveHostFiles(cmd().(hostFileChecked))
		if len(m.pastes) != 0 {
			t.Fatal("attachment crossed session/draft boundary")
		}
	}
}

func TestHostFilePasteRemainsInlineWhenLong(t *testing.T) {
	m := conversationModel()
	m.input.InsertString("앞 `!")
	text := "/" + strings.Repeat("a", 900)
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(text), Paste: true})
	if len(m.pastes) != 0 || m.input.Value() != "앞 `!"+text {
		t.Fatal("host path collapsed into a text paste")
	}
}

func TestHostFileHomePathWithSpaces(t *testing.T) {
	path := testHostFile(t, "한글 report.txt")
	t.Setenv("HOME", filepath.Dir(path))
	t.Setenv("USERPROFILE", filepath.Dir(path))
	m := conversationModel()
	m.input.InsertString("`!~/한글 report.txt`")
	_ = resolveHostFiles(t, m)
	if len(m.pastes) != 1 {
		t.Fatal("home path with spaces did not become a chip", m.input.Value())
	}
	for _, p := range m.pastes {
		p.attachment.cancel()
		if p.attachment.source != path {
			t.Fatal("wrong host path", p.attachment.source)
		}
	}
}

func TestHostFileCheckDoesNotRetargetUpload(t *testing.T) {
	m := conversationModel()
	m.client = &fileAttachmentClient{root: t.TempDir()}
	m.input.InsertString("`!" + testHostFile(t, "x.txt"))
	cmd := resolveHostFiles(t, m)
	m.sessions = []*api.Session{{Id: "other", RunId: "other"}}
	result := cmd().(fileUploadDone)
	if result.upload.session != "s" || result.upload.run != "run" || result.err != nil {
		t.Fatal("upload changed target", result)
	}
}
