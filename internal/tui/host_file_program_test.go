package tui

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/assets"
	"github.com/lesomnus/cxz/internal/transport"
)

type hostDropClient struct {
	api.SessionsClient
	upload assets.Upload
	body   string
}

func (c *hostDropClient) UploadAttachment(_ context.Context, upload assets.Upload, src io.Reader) (string, error) {
	c.upload = upload
	data, err := io.ReadAll(src)
	c.body = string(data)
	return "/cxz/assets/session/asset/" + upload.Name, err
}

type hostDropProgram struct {
	*model
	ready chan struct{}
}

func (p *hostDropProgram) Init() tea.Cmd { close(p.ready); return nil }
func (p *hostDropProgram) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	_, cmd := p.model.Update(msg)
	if _, ok := msg.(fileUploadDone); ok {
		return p, tea.Quit
	}
	return p, cmd
}

// Exercise decoded Windows input through the actual event loop and debounce,
// rather than injecting a successful filesystem reply. On Windows this also
// checks drive-letter paths against the native filesystem (including in CI).
func TestWindowsHostFileDropProgram(t *testing.T) {
	for _, bracketed := range []bool{false, true} {
		for _, quoted := range []bool{false, true} {
			t.Run(fmt.Sprintf("bracketed=%t/quoted=%t", bracketed, quoted), func(t *testing.T) {
				path := testHostFile(t, "한글 report.txt")
				drop := path
				if quoted {
					drop = `"` + drop + `"`
				}
				if bracketed {
					drop = "\x1b[200~" + drop + "\x1b[201~"
				}
				// The marker is typed with native keys; the drop arrives as text,
				// with focus moving to Explorer and back. No closing backtick.
				wire := "앞 문장 " + win32Key(192, '`', 0, 1, 1) + win32Key(49, '!', 16, 1, 1)
				wire += "\x1b[O" + drop + "\x1b[I"
				var serialized strings.Builder
				for _, char := range utf16.Encode([]rune(wire)) {
					serialized.WriteString(win32Key(0, rune(char), 0, 1, 1))
				}
				var decoder consoleVTDecoder
				var normalized bytes.Buffer
				data := []byte(serialized.String())
				for len(data) > 0 {
					n := min(7, len(data))
					decoder.write(&normalized, data[:n], false)
					data = data[n:]
				}
				decoder.write(&normalized, nil, true)
				messages := decodedTerminalMessages(t, &normalized)

				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				m := conversationModel()
				m.ctx = transport.WithRemote(ctx)
				client := &hostDropClient{}
				m.client = client
				probe := &hostDropProgram{model: m, ready: make(chan struct{})}
				program := tea.NewProgram(probe, tea.WithContext(ctx), tea.WithInput(nil), tea.WithOutput(io.Discard), tea.WithoutRenderer(), tea.WithoutSignalHandler())
				m.program = program
				done := make(chan error, 1)
				go func() { _, err := program.Run(); done <- err }()
				select {
				case <-probe.ready:
				case <-ctx.Done():
					<-done
					t.Fatal("input program did not start")
				}
				for _, msg := range messages {
					program.Send(msg)
				}
				if err := <-done; err != nil {
					t.Fatalf("drop did not become a chip: %v; draft=%q notice=%q", err, m.input.Value(), m.notice)
				}
				if client.upload.SessionID != "s" || client.upload.RunID != "run" || client.body != "data" {
					t.Fatal("wrong upload", client.upload, client.body)
				}
				if !strings.HasPrefix(m.input.Value(), "앞 문장 [한글 report.txt ") || !m.fileAttachmentsReady(m.input.Value()) {
					t.Fatal("drop did not become a completed inline chip", m.input.Value(), m.notice)
				}
			})
		}
	}
}
