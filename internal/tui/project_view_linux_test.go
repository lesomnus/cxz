//go:build linux

package tui

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/term"
	"github.com/lesomnus/cxz/api"
	"golang.org/x/sys/unix"
	"google.golang.org/grpc"
)

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
	p := tea.NewProgram(m, tea.WithContext(ctx), tea.WithInput(slave), tea.WithOutput(slave), tea.WithAltScreen())
	m.program = p
	done := make(chan error, 1)
	go func() { _, err := p.Run(); done <- err }()
	navigated := make(chan error, 1)
	go func() {
		for _, stage := range []struct{ want, key string }{
			{"session  stopped", "a"}, {"No accounts yet", "n"},
			{"Create account", "\x1b"}, {"No accounts yet", "\x1b"},
			{"cxz · project", "\r"}, {"◉", "\x11"}, {"cxz · project", "\x03"},
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
	if !m.projectView {
		t.Fatal("Ctrl+Q did not return to project")
	}
}
