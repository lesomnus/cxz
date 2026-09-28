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
	source      string
	received    int64
	total       int64
	started     time.Time
	transferred bool
	cancel      context.CancelFunc
}
type downloadProgress struct {
	job         *fileDownload
	received    int64
	total       int64
	started     time.Time
	transferred bool
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
	job := &fileDownload{source: source, cancel: cancel, total: -1}
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
			w := &downloadWriter{dst: dst, total: -1, started: time.Now(), notify: func(p downloadProgress) {
				if program != nil && ctx.Err() == nil {
					p.job = job
					program.Send(p)
				}
			}}
			err := client.Download(ctx, project, source, w)
			if err == nil {
				w.emit(true)
			}
			return err
		})
		return downloadDone{job, target, n, err}
	}
}

type downloadWriter struct {
	dst           io.Writer
	notify        func(downloadProgress)
	count, total  int64
	started, last time.Time
}

func (w *downloadWriter) SetDownloadSize(size int64) error {
	w.total = size
	w.started = time.Now()
	w.emit(false)
	return nil
}
func (w *downloadWriter) emit(transferred bool) {
	w.last = time.Now()
	w.notify(downloadProgress{received: w.count, total: w.total, started: w.started, transferred: transferred})
}
func (w *downloadWriter) Write(b []byte) (int, error) {
	n, err := w.dst.Write(b)
	w.count += int64(n)
	if time.Since(w.last) >= 200*time.Millisecond {
		w.emit(false)
	}
	return n, err
}

func (d *fileDownload) progress(now time.Time) string {
	if d.started.IsZero() {
		return "Downloading · preparing… · /download --cancel"
	}
	elapsed := now.Sub(d.started).Seconds()
	speed := float64(0)
	if elapsed > 0 {
		speed = float64(d.received) / elapsed
	}
	rate, eta := "—/s", "—"
	if speed > 0 {
		rate = attachmentSize(int64(speed)) + "/s"
		if d.total >= 0 {
			seconds := min(float64(1<<31), max(0, float64(d.total-d.received)/speed))
			eta = (time.Duration(seconds+0.999) * time.Second).String()
		}
	}
	progress := "[??????????] —%"
	amount := attachmentSize(d.received)
	if d.total >= 0 {
		fraction := float64(1)
		if d.total > 0 {
			fraction = min(1, max(0, float64(d.received)/float64(d.total)))
		}
		filled := int(fraction * 10)
		progress = fmt.Sprintf("[%s%s] %.0f%%", strings.Repeat("━", filled), strings.Repeat("─", 10-filled), fraction*100)
		amount += "/" + attachmentSize(d.total)
	}
	if d.transferred {
		eta = "saving…"
	}
	return fmt.Sprintf("↓ %s %s · %s · ETA %s · /download --cancel", progress, amount, rate, eta)
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
