package tui

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/assets"
	"github.com/mattn/go-shellwords"
)

type fileUpload struct {
	session, run, source, name, label string
	size                              int64
	modified                          time.Time
	cancel                            context.CancelFunc
	received                          int64
	err                               error
	done                              bool
}
type fileUploadProgress struct {
	item     *pastedText
	upload   *fileUpload
	received int64
}
type fileUploadDone struct {
	item   *pastedText
	upload *fileUpload
	result *api.Attachment
	err    error
}

// Round before choosing precision, keeping the entire number + unit <= 5 cells.
func attachmentSize(size int64) string {
	value := float64(max(0, size))
	units := []string{"B", "K", "M", "G", "T", "P", "E"}
	unit := 0
	for value >= 1000 && unit < len(units)-1 {
		value /= 1024
		unit++
	}
	if unit == 0 {
		return fmt.Sprintf("%dB", size)
	}
	for {
		precision := 0
		if value < 9.995 {
			precision = 2
		} else if value < 99.95 {
			precision = 1
		}
		text := fmt.Sprintf("%.*f%s", precision, value, units[unit])
		if len(text) <= 5 {
			return text
		}
		value /= 1024
		unit++
	}
}

func (p *pastedText) fileChip() string {
	u := p.attachment
	progress := quotaBarWidth(0, 5)
	if u.size > 0 {
		progress = quotaBarWidth(100*float64(u.received)/float64(u.size), 5)
	}
	if u.err != nil {
		progress = "ERROR"
	} else if u.done {
		progress = fmt.Sprintf("%5s", attachmentSize(u.size))
	}
	return "[" + u.label + " " + progress + "]"
}

// The chip contains its visible text. Every progress replacement preserves its
// cell/rune count, keeping wrapping and cursor movement stable without hidden IDs.
func (m *model) updateFileChip(p *pastedText) {
	old, next := p.token, p.fileChip()
	if old == next {
		return
	}
	delete(m.pastes, old)
	p.token = next
	m.pastes[next] = p
	value := m.input.Value()
	li := m.input.LineInfo()
	pos := li.StartColumn + li.ColumnOffset
	lines := strings.Split(value, "\n")
	for _, line := range lines[:m.input.Line()] {
		pos += len([]rune(line)) + 1
	}
	if strings.Contains(value, old) {
		m.setPathInput(strings.ReplaceAll(value, old, next), pos)
	}
	for id, draft := range m.drafts {
		m.drafts[id] = strings.ReplaceAll(draft, old, next)
	}
	if d := m.pasteDialog; d != nil {
		for i, t := range d.tokens {
			if t == old {
				d.tokens[i] = next
			}
		}
	}
	if s := m.pasteSelection; s != nil && s.token == old {
		s.token = next
		s.value = strings.ReplaceAll(s.value, old, next)
	}
}

func filePaths(text string) []string {
	text = strings.TrimSpace(text)
	if text == "" || strings.ContainsAny(text, "\x00\n\r") {
		return nil
	}
	// A raw single path may contain spaces; prefer it before shell unquoting.
	raw, err := localHostPath(text)
	if err != nil {
		return nil
	}
	candidates := []string{raw}
	if _, err := os.Stat(raw); err != nil {
		var err error
		if runtime.GOOS == "windows" {
			candidates, err = windowsDropPaths(text)
		} else {
			parser := shellwords.NewParser()
			parser.ParseEnv, parser.ParseBacktick = false, false
			candidates, err = parser.Parse(text)
		}
		if err != nil {
			return nil
		}
	}
	if len(candidates) == 0 || len(candidates) > 16 {
		return nil
	}
	for i, path := range candidates {
		if strings.HasPrefix(path, "file://") {
			u, err := url.Parse(path)
			if err != nil || u.Host != "" && u.Host != "localhost" {
				return nil
			}
			path = u.Path
			if runtime.GOOS == "windows" && len(path) > 2 && path[0] == '/' && path[2] == ':' {
				path = path[1:]
			}
		}
		var err error
		path, err = localHostPath(path)
		if err != nil {
			return nil
		}
		if !filepath.IsAbs(path) {
			return nil
		}
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() {
			return nil
		}
		candidates[i] = path
	}
	return candidates
}

func windowsDropPaths(text string) ([]string, error) {
	var paths []string
	var part strings.Builder
	var quote rune
	for _, r := range text {
		if quote != 0 {
			if r == quote {
				quote = 0
			} else {
				part.WriteRune(r)
			}
			continue
		}
		if r == '"' || r == '\'' {
			quote = r
			continue
		}
		if unicode.IsSpace(r) {
			if part.Len() > 0 {
				paths = append(paths, part.String())
				part.Reset()
			}
			continue
		}
		part.WriteRune(r)
	}
	if quote != 0 {
		return nil, fmt.Errorf("unfinished filename quote")
	}
	if part.Len() > 0 {
		paths = append(paths, part.String())
	}
	return paths, nil
}

func (m *model) newFileChip(session, run, source string, info os.FileInfo) *pastedText {
	label := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || r == '[' || r == ']' || r == '`' {
			return '_'
		}
		return r
	}, info.Name())
	label = ansi.Truncate(label, 48, "…")
	base := label
	for n := 2; ; n++ {
		used := false
		for _, p := range m.pastes {
			if p.attachment != nil && p.attachment.label == label {
				used = true
				break
			}
		}
		if !used {
			break
		}
		label = fmt.Sprintf("%s (%d)", base, n)
	}

	u := &fileUpload{session: session, run: run, source: source, name: info.Name(), label: label, size: info.Size(), modified: info.ModTime()}
	p := &pastedText{file: true, owner: session, attachment: u}
	p.token = p.fileChip()
	return p
}

func (m *model) startFileUpload(p *pastedText) tea.Cmd {
	u := p.attachment
	parent := m.ctx
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	u.cancel = cancel
	client, program := m.client, m.program
	return func() tea.Msg {
		defer cancel()
		progress := func(n int64) {
			if program != nil {
				program.Send(fileUploadProgress{p, u, n})
			}
		}
		result, err := uploadAttachment(ctx, client, u, progress)
		return fileUploadDone{p, u, result, err}
	}
}

func uploadAttachment(ctx context.Context, client api.SessionsClient, u *fileUpload, progress func(int64)) (*api.Attachment, error) {
	f, err := os.Open(u.source)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() != u.size || !info.ModTime().Equal(u.modified) {
		return nil, fmt.Errorf("file changed before upload; attach it again")
	}
	uploader, ok := client.(assets.Uploader)
	if !ok {
		return nil, fmt.Errorf("connection does not support file uploads; update cxz")
	}
	reader := &uploadFileReader{file: f, upload: u, progress: progress}
	path, err := uploader.UploadAttachment(ctx, assets.Upload{SessionID: u.session, RunID: u.run, Name: u.name, Size: u.size}, reader)
	if err != nil {
		return nil, err
	}
	if path == "" {
		return nil, fmt.Errorf("upload completed without an attachment path")
	}
	return &api.Attachment{Path: path}, nil
}

// Count bytes as the stream reads the file. EOF also verifies that the source
// stayed unchanged, before the client closes the stream and publishes it.
type uploadFileReader struct {
	file     *os.File
	upload   *fileUpload
	progress func(int64)
	read     int64
}

func (r *uploadFileReader) Read(p []byte) (int, error) {
	n, err := r.file.Read(p)
	r.read += int64(n)
	if n > 0 && r.progress != nil {
		r.progress(r.read)
	}
	if err == io.EOF {
		info, statErr := r.file.Stat()
		if statErr != nil {
			return n, statErr
		}
		if r.read != r.upload.size || info.Size() != r.upload.size || !info.ModTime().Equal(r.upload.modified) {
			return n, fmt.Errorf("file changed during upload; attach it again")
		}
	}
	return n, err
}

func (m *model) receiveFileUpload(v fileUploadDone) {
	p, u := v.item, v.upload
	if p.attachment != u || m.pastes[p.token] != p {
		return
	}
	u.cancel = nil
	u.err = v.err
	if v.err == nil {
		u.done = true
		u.received = u.size
		p.path = v.result.Path
	}
	m.updateFileChip(p)
	if v.err != nil {
		m.showError("Attachment upload failed: " + v.err.Error())
	}
}

func (m *model) fileAttachmentsReady(text string) bool {
	if len(hostFileTokens(text)) > 0 {
		m.notice = "Finish a valid host file path and wait for its chip, or remove the ! marker."
		return false
	}
	for token, p := range m.pastes {
		if p.attachment == nil || !strings.Contains(text, token) {
			continue
		}
		if s := m.current(); s == nil || p.owner != s.Id {
			m.showError("Attachment belongs to another session.")
			return false
		}
		if !p.attachment.done {
			m.notice = "Wait for attachment uploads, or remove the failed chip."
			return false
		}
	}
	return true
}

func (m *model) pruneFileUploads() {
	for token, p := range m.pastes {
		u := p.attachment
		if u == nil || u.cancel == nil {
			continue
		}
		used := strings.Contains(m.drafts[p.owner], token)
		if s := m.current(); s != nil && s.Id == p.owner {
			used = used || strings.Contains(m.input.Value(), token)
		}
		if !used {
			u.cancel()
			u.cancel = nil
			delete(m.pastes, token)
		}
	}
}

func (m *model) fileChipKey(p *pastedText, key string) (bool, tea.Cmd) {
	switch key {
	case "t":
		m.notice = "File attachments are sent as paths; delete the chip to remove it."
		return true, nil
	case "f":
		if p.attachment.err == nil {
			return true, nil
		}
		next := *p.attachment
		next.err = nil
		next.received = 0
		p.attachment = &next
		m.updateFileChip(p)
		return true, m.startFileUpload(p)
	case "enter":
		u := p.attachment
		m.pasteDialog = nil
		m.openReport("Attachment", safeText(u.name)+"\n"+attachmentSize(u.size)+"\n"+safeText(p.path)+"\n\nf retries a failed upload · d removes the chip")
		return true, nil
	}
	return false, nil
}
