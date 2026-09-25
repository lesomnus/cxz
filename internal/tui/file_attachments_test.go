package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/assets"
	"io"
)

type fileAttachmentClient struct {
	api.SessionsClient
	root string
	fail bool
}

func (c *fileAttachmentClient) UploadAttachment(ctx context.Context, r assets.Upload, src io.Reader) (string, error) {
	if c.fail {
		return "", fmt.Errorf("connection lost")
	}
	return assets.Add(ctx, c.root, "project", r.SessionID, r.Name, r.Size, src)
}

func TestFileChipUploadAndExpansion(t *testing.T) {
	m := conversationModel()
	m.ctx = context.Background()
	m.client = &fileAttachmentClient{root: t.TempDir()}
	filename := filepath.Join(t.TempDir(), "한글 report.txt")
	body := strings.Repeat("x", 256*1024+31)
	if err := os.WriteFile(filename, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	m.input.InsertString("`!")
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("'" + filename + "'"), Paste: true})
	cmd := resolveHostFiles(t, m)
	if cmd == nil {
		t.Fatal("drop not captured")
	}
	var p *pastedText
	for _, value := range m.pastes {
		p = value
	}
	if p == nil || !strings.Contains(p.token, "⣀⣀⣀⣀⣀") || m.fileAttachmentsReady(m.input.Value()) {
		t.Fatal("incomplete chip was sent")
	}
	// Progress retains the cursor and supports Unicode/wrapped chips.
	m.input.InsertString("suffix")
	value, pos, _, _, _ := m.chipInput()
	m.Update(fileUploadProgress{p, p.attachment, int64(len(body)) / 2})
	_, nextPos, _, _, _ := m.chipInput()
	if pos != nextPos || !strings.HasSuffix(m.input.Value(), "suffix") || len([]rune(value)) != len([]rune(m.input.Value())) {
		t.Fatal("progress moved input cursor")
	}
	m.Update(cmd())
	if !p.attachment.done || !m.fileAttachmentsReady(m.input.Value()) || strings.Contains(p.token, "⣿") {
		t.Fatal("upload did not finish", p.attachment.err)
	}
	if data, err := os.ReadFile(p.path); err != nil || string(data) != body {
		t.Fatal("uploaded bytes changed", err)
	}
	if !strings.Contains(expandPastes(m.input.Value(), m.pastes), p.path) {
		t.Fatal("chip did not resolve to path")
	}
	if got := ansi.Strip(m.decorateInputPastes(m.input.View())); strings.Contains(got, "Paste") {
		t.Fatal("file chip changed into paste label")
	}
}

func TestAttachmentFailureAndRemoval(t *testing.T) {
	m := conversationModel()
	m.ctx = context.Background()
	client := &fileAttachmentClient{root: t.TempDir(), fail: true}
	m.client = client
	filename := filepath.Join(t.TempDir(), "file.txt")
	os.WriteFile(filename, []byte("data"), 0600)
	m.input.InsertString("`!" + filename)
	cmd := resolveHostFiles(t, m)
	m.Update(cmd())
	var p *pastedText
	for _, value := range m.pastes {
		p = value
	}
	if p.attachment.err == nil || !strings.Contains(p.token, "ERROR") {
		t.Fatal("failed upload was not retained")
	}
	if m.fileAttachmentsReady(m.input.Value()) {
		t.Fatal("failed upload sent")
	}
	client.fail = false
	_, cmd = m.fileChipKey(p, "f")
	m.Update(cmd())
	if !p.attachment.done {
		t.Fatal("retry failed", p.attachment.err)
	}
	m.input.InsertString("`!" + filename)
	cmd = resolveHostFiles(t, m)
	_ = cmd
	var pending *pastedText
	for _, p := range m.pastes {
		if !p.attachment.done {
			pending = p
		}
	}
	m.input.SetValue("")
	m.pruneFileUploads()
	if pending.attachment.cancel != nil || m.pastes[pending.token] != nil {
		t.Fatal("removed upload not cancelled")
	}
}

func TestAttachmentSizeAndDropParsing(t *testing.T) {
	for _, size := range []int64{0, 1, 999, 1000, 1024, 10235, 10240, 102348, 102400, 1023999, 1048575, 1048576, 1 << 30} {
		text := attachmentSize(size)
		if len(text) > 5 || text == "" {
			t.Fatalf("%d rendered as %q", size, text)
		}
	}
	if attachmentSize(1024) != "1.00K" || attachmentSize(10240) != "10.0K" {
		t.Fatal("wrong precision")
	}
	dir := t.TempDir()
	a := filepath.Join(dir, "a b.txt")
	b := filepath.Join(dir, "c.txt")
	os.WriteFile(a, nil, 0600)
	os.WriteFile(b, nil, 0600)
	for _, input := range []string{a, "'" + a + "'", strings.ReplaceAll(a, " ", `\ `)} {
		if got := filePaths(input); len(got) != 1 || got[0] != a {
			t.Fatal("path not recognized", input, got)
		}
	}
	if got := filePaths("'" + a + "' '" + b + "'"); len(got) != 2 {
		t.Fatal("multiple files lost", got)
	}
	if filePaths("please read "+a) != nil || filePaths(dir) != nil {
		t.Fatal("ordinary text or folder captured")
	}
	got, err := windowsDropPaths(`"C:\Users\a b\file.txt" C:\temp\x.png`)
	if err != nil || len(got) != 2 || got[0] != `C:\Users\a b\file.txt` {
		t.Fatal("Windows paths changed", got, err)
	}
}
