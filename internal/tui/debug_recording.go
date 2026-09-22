package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"google.golang.org/grpc/status"
)

const debugEventLimit = 12000

// Explicit fields only: never serialize tea messages, drafts, errors or frames.
type debugState struct {
	Focus        string `json:"focus"`
	Session      int    `json:"session"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	InputFocused bool   `json:"input_focused"`
	Line         int    `json:"line"`
	Column       int    `json:"column"`
	InputRunes   int    `json:"input_runes"`
	Hint         bool   `json:"path_hint"`
	Chip         bool   `json:"chip_selected"`
}
type debugEvent struct {
	Bytes      int               `json:"bytes,omitempty"`
	Wait       int64             `json:"wait_us,omitempty"`
	IO         int64             `json:"io_us,omitempty"`
	Metrics    map[string]uint64 `json:"metrics,omitempty"`
	ErrorCode  string            `json:"error_code,omitempty"`
	At         int64             `json:"elapsed_us"`
	Kind       string            `json:"kind"`
	Type       string            `json:"type,omitempty"`
	Key        string            `json:"key,omitempty"`
	Binding    string            `json:"binding,omitempty"`
	Sequence   string            `json:"sequence,omitempty"`
	Translated string            `json:"translated,omitempty"`
	Before     *debugState       `json:"before,omitempty"`
	After      *debugState       `json:"after,omitempty"`
	Duration   int64             `json:"duration_us,omitempty"`
	Count      int               `json:"count,omitempty"`
}
type debugArchive struct {
	Started     time.Time         `json:"started"`
	Ended       time.Time         `json:"ended"`
	Format      int               `json:"format"`
	Dropped     int               `json:"dropped"`
	Environment map[string]string `json:"environment"`
	Events      []debugEvent      `json:"-"`
}
type debugRecorder struct {
	mu            sync.Mutex
	active        bool
	started       time.Time
	events        []debugEvent
	next, dropped int
	sessions      map[string]int
}

func (r *debugRecorder) Active() bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.active
}
func (r *debugRecorder) Start() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.active = true
	r.started = time.Now()
	r.events = make([]debugEvent, 0, debugEventLimit)
	r.next = 0
	r.dropped = 0
	r.sessions = map[string]int{}
}
func (r *debugRecorder) Add(e debugEvent) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.active {
		return
	}
	e.At = time.Since(r.started).Microseconds()
	if len(r.events) < debugEventLimit {
		r.events = append(r.events, e)
	} else {
		r.events[r.next] = e
		r.next = (r.next + 1) % debugEventLimit
		r.dropped++
	}
}
func (r *debugRecorder) session(id string) int {
	if r == nil || id == "" {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.active {
		return 0
	}
	if r.sessions[id] == 0 {
		r.sessions[id] = len(r.sessions) + 1
	}
	return r.sessions[id]
}

var safeTerminalValue = regexp.MustCompile(`^[a-zA-Z0-9_.+ -]{0,100}$`)

func (r *debugRecorder) Stop() *debugArchive {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.active {
		return nil
	}
	r.active = false
	a := &debugArchive{Started: r.started, Ended: time.Now(), Format: 1, Dropped: r.dropped, Environment: map[string]string{"os": runtime.GOOS, "arch": runtime.GOARCH, "go": runtime.Version(), "tmux": fmt.Sprint(os.Getenv("TMUX") != ""), "ssh": fmt.Sprint(os.Getenv("SSH_CONNECTION") != "")}}
	for _, name := range []string{"TERM", "COLORTERM", "TERM_PROGRAM", "TERM_PROGRAM_VERSION"} {
		if value := os.Getenv(name); safeTerminalValue.MatchString(value) {
			a.Environment[name] = value
		}
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, v := range info.Settings {
			if v.Key == "vcs.revision" || v.Key == "vcs.modified" {
				a.Environment[v.Key] = v.Value
			}
		}
	}
	a.Events = append(a.Events, r.events[r.next:]...)
	a.Events = append(a.Events, r.events[:r.next]...)
	r.events = nil
	r.sessions = nil
	return a
}

// Only navigation escape sequences are retained. CSI-u for printable characters,
// terminal replies, pasted bytes and all other input are deliberately omitted.
var debugNavigation = regexp.MustCompile("^(?:\\x1b\\[(?:[0-9]{1,2}(?:;[0-9]{1,3})?)?[ABCDHF]|\\x1bO[ABCDHF]|\\x1b\\[(?:5735[0-7]|574(?:1[7-9]|2[0-4]))(?:;[0-9]{1,3})?u)$")

func (r *debugRecorder) navigation(raw, translated []byte, paste bool) {
	if r == nil || paste || !debugNavigation.Match(raw) {
		return
	}
	out := "omitted"
	if debugNavigation.Match(translated) {
		out = fmt.Sprintf("%q", translated)
	}
	r.Add(debugEvent{Kind: "terminal_navigation", Sequence: fmt.Sprintf("%q", raw), Translated: out})
}
func (m *model) debugState() *debugState {
	focus := "composer"
	switch {
	case m.redactDialog != nil:
		focus = "secret"
	case m.workflow != nil:
		focus = "account_workflow"
	case m.settingsPage != nil:
		focus = "settings"
	case m.memoryPage != nil:
		focus = "memory"
	case m.questionDialog != nil:
		focus = "question"
	case m.pasteDialog != nil:
		focus = "paste"
	case m.terminalFocused():
		focus = "terminal"
	case m.filePreview != nil && m.filePreview.focused:
		focus = "tool_preview"
	case m.report != nil:
		focus = "report"
	case m.modelPicker != nil:
		focus = "model_picker"
	case m.accountView:
		focus = "accounts"
	case m.panelFocus || m.projectView:
		focus = "projects"
	case m.focusApproval:
		focus = "approval"
	case m.focusList:
		focus = "sessions"
	case m.renaming:
		focus = "rename"
	}
	s := &debugState{Focus: focus, Width: m.terminalWidth, Height: m.height, InputFocused: m.input.Focused(), Hint: m.pathHints != nil, Chip: m.pasteSelection != nil}
	if current := m.current(); current != nil {
		s.Session = m.debugRecorder.session(current.Id)
	}
	if focus == "composer" {
		li := m.input.LineInfo()
		s.Line = m.input.Line()
		s.Column = li.StartColumn + li.ColumnOffset
		s.InputRunes = len([]rune(m.input.Value()))
	}
	return s
}
func (m *model) debugUpdate(msg tea.Msg) func() {
	r := m.debugRecorder
	if !r.Active() {
		return func() {}
	}
	start := time.Now()
	e := debugEvent{Kind: "update", Before: m.debugState()}
	if t := reflect.TypeOf(msg); t != nil {
		e.Type = t.String()
	}
	switch v := msg.(type) {
	case result:
		if v.err != nil {
			e.ErrorCode = status.Code(v.err).String()
		}
	case settingsResult:
		if v.err != nil {
			e.ErrorCode = status.Code(v.err).String()
		} else if v.infoErr != nil {
			e.ErrorCode = status.Code(v.infoErr).String()
		}
	case memoryResult:
		if v.err != nil {
			e.ErrorCode = status.Code(v.err).String()
		}
	}
	if k, ok := msg.(tea.KeyMsg); ok {
		switch {
		case k.Paste:
			e.Key = "paste"
		case k.Type == tea.KeyRunes:
			e.Key = "text"
		default:
			e.Key = k.String()
		}
		if !k.Paste {
			switch {
			case key.Matches(k, m.input.KeyMap.WordBackward):
				e.Binding = "word_backward"
			case key.Matches(k, m.input.KeyMap.WordForward):
				e.Binding = "word_forward"
			}
		}
	}
	return func() { e.After = m.debugState(); e.Duration = time.Since(start).Microseconds(); r.Add(e) }
}

type recordingDirectoryKey struct{}

func WithRecordingDirectory(ctx context.Context, dir string) context.Context {
	return context.WithValue(ctx, recordingDirectoryKey{}, dir)
}
func saveDebugArchive(ctx context.Context, a *debugArchive) (string, error) {
	dir, _ := ctx.Value(recordingDirectoryKey{}).(string)
	if dir == "" {
		base := os.Getenv("XDG_STATE_HOME")
		if base == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", err
			}
			base = filepath.Join(home, ".local", "state")
		}
		dir = filepath.Join(base, "cxz", "recordings")
	}
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	if err = os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	f, err := os.CreateTemp(dir, "cxz-debug-"+a.Started.UTC().Format("20060102T150405Z")+"-*.jsonl")
	if err != nil {
		return "", err
	}
	success := false
	defer func() {
		f.Close()
		if !success {
			os.Remove(f.Name())
		}
	}()
	enc := json.NewEncoder(f)
	if err = enc.Encode(a); err != nil {
		return "", err
	}
	for _, e := range a.Events {
		if err = enc.Encode(e); err != nil {
			return "", err
		}
	}
	if err = f.Sync(); err != nil {
		return "", err
	}
	if err = f.Close(); err != nil {
		return "", err
	}
	success = true
	return f.Name(), nil
}

type recordingSaved struct {
	archive *debugArchive
	path    string
	err     error
}

func (m *model) toggleRecording() tea.Cmd {
	if m.recordingSaving {
		return nil
	}
	if m.debugRecorder == nil {
		m.debugRecorder = &debugRecorder{}
	}
	if m.debugRecorder.Active() {
		m.recordingPending = m.debugRecorder.Stop()
	}
	if m.recordingPending == nil {
		m.debugRecorder.Start()
		m.recordingError = ""
		m.notice = "Debug recording started · F9 stops and saves"
		return m.performanceCommand()
	}
	m.recordingSaving = true
	a, ctx := m.recordingPending, m.ctx
	task := &debugSave{done: make(chan struct{})}
	m.recordingTask = task
	go func() {
		path, err := saveDebugArchive(ctx, a)
		task.result = recordingSaved{a, path, err}
		close(task.done)
	}()
	return func() tea.Msg { <-task.done; return task.result }
}
func (m *model) receiveRecording(r recordingSaved) {
	if r.archive != m.recordingPending {
		return
	}
	m.recordingSaving = false
	m.recordingTask = nil
	if r.err != nil {
		m.recordingError = r.err.Error()
		m.notice = "Recording save failed: " + r.err.Error() + " · F9 retries"
		return
	}
	m.recordingPending = nil
	m.recordingError = ""
	m.lastRecording = r.path
	m.notice = "Debug recording saved: " + r.path
}
func (m *model) recordingLabel(now time.Time) string {
	switch {
	case m.debugRecorder.Active():
		dot := "⬤"
		if int(now.Sub(m.debugRecorder.generation())/time.Second)%2 != 0 {
			dot = strings.Repeat(" ", ansi.StringWidth(dot))
		}
		return failure.Render(dot+" REC") + " "
	case m.recordingSaving:
		return warning.Render("Saving debug recording…")
	case m.recordingPending != nil:
		return warning.Render("Recording unsaved · F9 retry")
	}
	return ""
}

func (m *model) recordingStatusRow(status string, width int) string {
	label := m.recordingLabel(time.Now())
	if label == "" {
		return clip(status, width)
	}
	label = clip(label, width)
	available := max(0, width-ansi.StringWidth(label)-1)
	line := ansi.Truncate(status, available, "…")
	return line + strings.Repeat(" ", max(0, width-ansi.StringWidth(line)-ansi.StringWidth(label))) + label
}

func (m *model) recordingBadge(view string) string {
	// The conversation uses its status row above the input. Other screens have
	// no composer, so keep the recording state visible in their final row.
	if (!m.projectView || m.creating) && (!m.panelFocus || m.panelVisible() || m.creating) && !m.accountView && m.workflow == nil && m.settingsPage == nil && m.memoryPage == nil && m.width >= 40 && m.height >= 14 {
		return view
	}
	rows := strings.Split(view, "\n")
	rows[len(rows)-1] = m.recordingStatusRow(rows[len(rows)-1], m.settingsWidth())
	return strings.Join(rows, "\n")
}

type debugSave struct {
	done   chan struct{}
	result recordingSaved
}

func (m *model) finishRecording() (string, error) {
	if task := m.recordingTask; task != nil {
		<-task.done
		m.receiveRecording(task.result)
		if task.result.err != nil {
			return "", task.result.err
		}
		return task.result.path, nil
	}
	if m.debugRecorder.Active() {
		m.recordingPending = m.debugRecorder.Stop()
	}
	if m.recordingPending == nil {
		return "", nil
	}
	path, err := saveDebugArchive(m.ctx, m.recordingPending)
	if err == nil {
		m.recordingPending = nil
		m.lastRecording = path
	}
	return path, err
}

func (m *model) recordCommand(text string) tea.Cmd {
	if text == "/record" {
		return m.toggleRecording()
	}
	if text == "/record status" {
		state := "Recording is off."
		if m.debugRecorder.Active() {
			state = "Recording is on · F9 stops and saves."
		}
		if m.recordingSaving {
			state = "Saving recording…"
		}
		if m.recordingError != "" {
			state += "\nSave failed: " + m.recordingError + "\nF9 retries."
		}
		if m.lastRecording != "" {
			state += "\n\nLast recording:\n" + m.lastRecording
		}
		m.openReport("/record", state)
		return nil
	}
	m.openReport("/record", "Usage: /record | /record status · F9 toggles recording")
	return nil
}
