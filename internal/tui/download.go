package tui

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lesomnus/cxz/internal/containerterm"
	"github.com/lesomnus/cxz/internal/download"
)

type fileDownload struct {
	source   string
	received int64
	cancel   context.CancelFunc
}
type downloadProgress struct {
	job      *fileDownload
	received int64
}
type downloadDone struct {
	job  *fileDownload
	path string
	size int64
	err  error
}

func downloadArgument(text string) string {
	arg := strings.TrimSpace(strings.TrimPrefix(text, "/download"))
	if len(arg) >= 2 && (arg[0] == '`' || arg[0] == '"' || arg[0] == '\'') && arg[len(arg)-1] == arg[0] {
		arg = arg[1 : len(arg)-1]
	}
	return arg
}

func (m *model) downloadCommand(text string) tea.Cmd {
	source := downloadArgument(text)
	if source == "--cancel" {
		if m.download != nil {
			m.download.cancel()
			m.notice = "Cancelling download…"
		} else {
			m.notice = "No download in progress"
		}
		return nil
	}
	if m.download != nil {
		m.notice = "A download is already in progress; /download --cancel stops it"
		return nil
	}
	if !containerterm.ValidDownloadPath(source) {
		m.input.SetValue("/download ")
		m.input.CursorEnd()
		m.notice = "Choose a container file · /download <path>"
		return nil
	}
	session := m.current()
	if session == nil || session.ProjectId == "" {
		m.showError("Select a project session first")
		return nil
	}
	client, ok := m.client.(containerterm.DownloadClient)
	if !ok {
		m.showError("Container downloads unavailable; update the manager")
		return nil
	}
	ctx, cancel := context.WithTimeout(m.contextFor(session.ProjectId), 30*time.Minute)
	job := &fileDownload{source: source, cancel: cancel}
	m.download = job
	project := session.ProjectId
	program := m.program
	return func() tea.Msg {
		defer cancel()
		dir, err := download.Directory()
		if err != nil {
			return downloadDone{job: job, err: err}
		}
		target, n, err := download.Save(ctx, dir, source, func(dst io.Writer) error {
			w := &downloadWriter{dst: dst, notify: func(n int64) {
				if program != nil && ctx.Err() == nil {
					program.Send(downloadProgress{job, n})
				}
			}}
			return client.Download(ctx, project, source, w)
		})
		return downloadDone{job, target, n, err}
	}
}

type downloadWriter struct {
	dst    io.Writer
	notify func(int64)
	count  int64
	last   time.Time
}

func (w *downloadWriter) Write(b []byte) (int, error) {
	n, err := w.dst.Write(b)
	w.count += int64(n)
	if time.Since(w.last) >= 200*time.Millisecond {
		w.last = time.Now()
		w.notify(w.count)
	}
	return n, err
}
func (m *model) receiveDownload(done downloadDone) {
	if m.download != done.job {
		return
	}
	m.download = nil
	if done.err != nil {
		m.showError("Download: " + done.err.Error())
		return
	}
	m.notice = fmt.Sprintf("Downloaded %s · %s", attachmentSize(done.size), done.path)
}
