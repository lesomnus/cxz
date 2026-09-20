//go:build linux

package tui

import (
	"context"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/term"
	"github.com/lesomnus/cxz/api"
	"golang.org/x/sys/unix"
	"google.golang.org/grpc"
)

type sizeSnapshot struct {
	width, height, renderedWidth, renderedHeight int
}

type sizeObserver struct {
	inner *model
	sizes chan sizeSnapshot
}

func (m *sizeObserver) Init() tea.Cmd { return nil }
func (m *sizeObserver) View() string  { return m.inner.View() }
func (m *sizeObserver) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	_, cmd := m.inner.Update(msg)
	if _, ok := msg.(tea.WindowSizeMsg); ok {
		rows := strings.Split(m.View(), "\n")
		width := 0
		for _, row := range rows {
			width = max(width, ansi.StringWidth(row))
		}
		m.sizes <- sizeSnapshot{m.inner.width, m.inner.height, width, len(rows)}
	}
	return m, cmd
}

func TestCursorWriterTerminalResize(t *testing.T) {
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		t.Skipf("PTY unavailable: %v", err)
	}
	defer master.Close()
	if err := unix.IoctlSetPointerInt(int(master.Fd()), unix.TIOCSPTLCK, 0); err != nil {
		t.Fatal(err)
	}
	n, err := unix.IoctlGetInt(int(master.Fd()), unix.TIOCGPTN)
	if err != nil {
		t.Fatal(err)
	}
	slave, err := os.OpenFile(fmt.Sprintf("/dev/pts/%d", n), os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer slave.Close()
	setSize := func(width, height int) {
		t.Helper()
		if err := unix.IoctlSetWinsize(int(slave.Fd()), unix.TIOCSWINSZ, &unix.Winsize{Col: uint16(width), Row: uint16(height)}); err != nil {
			t.Fatal(err)
		}
	}
	setSize(117, 37) // Deliberately unlike the model's fallback dimensions.
	go io.Copy(io.Discard, master)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	m := &sizeObserver{inner: conversationModel(), sizes: make(chan sizeSnapshot, 16)}
	w := &cursorWriter{out: slave}
	if w.Fd() != slave.Fd() {
		t.Fatal("terminal descriptor not forwarded")
	}
	m.inner.cursorOutput = w
	p := tea.NewProgram(m, tea.WithContext(ctx), tea.WithInput(slave), tea.WithOutput(w), tea.WithAltScreen())
	done := make(chan error, 1)
	go func() { _, err := p.Run(); done <- err }()
	defer func() {
		p.Quit()
		if err := <-done; err != nil {
			t.Errorf("TUI shutdown: %v", err)
		}
	}()
	check := func(width, height int, resize bool) {
		t.Helper()
		tick := time.NewTicker(20 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case got := <-m.sizes:
				if got != (sizeSnapshot{min(width, maxViewWidth), height, width, height}) {
					t.Fatalf("want fullscreen %dx%d, got %+v", width, height, got)
				}
				return
			case <-tick.C:
				if resize {
					// The PTY is not our controlling terminal; notify as a real
					// terminal would. Retry until the startup watcher is ready.
					if err := unix.Kill(os.Getpid(), unix.SIGWINCH); err != nil {
						t.Fatal(err)
					}
				}
			case <-ctx.Done():
				t.Fatal("terminal size was not detected")
			}
		}
	}
	check(117, 37, false)
	setSize(143, 45)
	check(143, 45, true)
	setSize(200, 45)
	check(200, 45, true)
	setSize(63, 19)
	check(63, 19, true)
}

type screenClient struct{ projectClient }

func (*screenClient) List(context.Context, *api.Empty, ...grpc.CallOption) (*api.SessionList, error) {
	return &api.SessionList{Sessions: []*api.Session{{Id: "session", ProjectId: "p", Agent: "claude", State: "stopped"}}}, nil
}
func (*screenClient) Projects(context.Context, *api.Empty, ...grpc.CallOption) (*api.ProjectList, error) {
	return &api.ProjectList{Projects: []*api.Project{{Id: "p", Name: "Project", Workspace: "/work"}}}, nil
}
func (*screenClient) Watch(context.Context, *api.WatchRequest, ...grpc.CallOption) (grpc.ServerStreamingClient[api.Event], error) {
	return nil, fmt.Errorf("fixture has no event stream")
}

func TestProjectSessionTerminalNavigation(t *testing.T) {
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		t.Skipf("PTY unavailable: %v", err)
	}
	defer master.Close()
	if err = unix.IoctlSetPointerInt(int(master.Fd()), unix.TIOCSPTLCK, 0); err != nil {
		t.Fatal(err)
	}
	n, err := unix.IoctlGetInt(int(master.Fd()), unix.TIOCGPTN)
	if err != nil {
		t.Fatal(err)
	}
	slave, err := os.OpenFile(fmt.Sprintf("/dev/pts/%d", n), os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer slave.Close()
	if err = unix.IoctlSetWinsize(int(slave.Fd()), unix.TIOCSWINSZ, &unix.Winsize{Row: 30, Col: 100}); err != nil {
		t.Fatal(err)
	}
	before, err := term.GetState(slave.Fd())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	m := projectModel()
	m.ctx = ctx
	m.client = &screenClient{}
	m.cursorOutput = &cursorWriter{out: slave}
	p := tea.NewProgram(m, tea.WithContext(ctx), tea.WithInput(slave), tea.WithOutput(m.cursorOutput), tea.WithAltScreen(), tea.WithMouseCellMotion())
	m.program = p
	done := make(chan error, 1)
	go func() { _, err := p.Run(); done <- err }()
	navigated := make(chan error, 1)
	go func() {
		for _, stage := range []struct{ want, key string }{
			{"session  stopped", "a"}, {"No accounts yet", "n"},
			{"Create account", "\x1b"}, {"No accounts yet", "\x1b"},
			{"cxz · project", "\r"}, {"quota", "\x11"}, {"Projects", "\x03"},
		} {
			var output strings.Builder
			buf := make([]byte, 4096)
			for !strings.Contains(output.String(), stage.want) {
				n, err := master.Read(buf)
				if err != nil {
					navigated <- err
					return
				}
				output.Write(buf[:n])
			}
			if _, err := master.Write([]byte(stage.key)); err != nil {
				navigated <- err
				return
			}
		}
		navigated <- nil
	}()
	select {
	case err := <-navigated:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("screen navigation timed out")
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("TUI did not exit")
	}
	if m.watchCancel != nil {
		m.watchCancel()
	}
	after, err := term.GetState(slave.Fd())
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("terminal was not restored", err)
	}
	if !m.panelFocus || m.projectView {
		t.Fatal("Ctrl+Q did not open project navigator")
	}
}
