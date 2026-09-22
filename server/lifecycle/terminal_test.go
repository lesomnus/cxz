package lifecycle

import (
	"context"
	"io"
	"net"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/containerterm"
	"github.com/lesomnus/cxz/internal/resourceclient"
	"github.com/lesomnus/cxz/resource"
	"github.com/lesomnus/payday/config"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

type terminalRuntime struct {
	*countedRuntime
	opened chan *trackedTerminal
}
type trackedTerminal struct {
	containerterm.Terminal
	reaped chan struct{}
}

func (t *trackedTerminal) Wait() error {
	err := t.Terminal.Wait()
	close(t.reaped)
	return err
}
func (f *terminalRuntime) OpenTerminal(ctx context.Context, project string, w, h int) (containerterm.Terminal, error) {
	if project != f.p.Id {
		return nil, status.Error(codes.NotFound, "wrong project")
	}
	p, err := containerterm.StartPTY(exec.CommandContext(ctx, "sh", "-c", "stty -echo; printf 'shell-ready\\n'; exec sh"), w, h)
	if err != nil {
		return nil, err
	}
	t := &trackedTerminal{Terminal: p, reaped: make(chan struct{})}
	f.opened <- t
	return t, nil
}

func TestProjectTerminalRPC(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, _, err := (config.DbConfig{Driver: "sqlite3", Dsn: "file:" + filepath.Join(t.TempDir(), "db"), MaxOpenConns: 1}).Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	f := &terminalRuntime{countedRuntime: &countedRuntime{fixture: &fixture{p: &api.Project{Id: "project", Name: "workspace", Workspace: "/workspace", State: "running"}}}, opened: make(chan *trackedTerminal, 1)}
	stack, err := Build(ctx, db, f)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stack.Project().Add(ctx, resource.ProjectAddRequest_builder{Workspace: "/workspace"}.Build()); err != nil {
		t.Fatal(err)
	}
	ln := bufconn.Listen(1 << 20)
	g := grpc.NewServer()
	resource.RegisterServer(g, stack)
	go g.Serve(ln)
	defer g.Stop()
	conn, err := grpc.NewClient("passthrough:///terminal", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return ln.Dial() }))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	c := resourceclient.New(conn)
	var before, after int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM audit").Scan(&before); err != nil {
		t.Fatal(err)
	}
	reaped := func(p *trackedTerminal) {
		t.Helper()
		select {
		case <-p.reaped:
		case <-time.After(3 * time.Second):
			t.Fatal("remote PTY not reaped")
		}
	}
	waitText := func(s *containerterm.Session, text string) {
		t.Helper()
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			if strings.Contains(ansi.Strip(s.Screen.Render()), text) {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatalf("missing %q: %q", text, s.Screen.Render())
	}
	t.Run("interactive", func(t *testing.T) {
		terminal, err := c.OpenTerminal(ctx, "project", 80, 24)
		if err != nil {
			t.Fatal(err)
		}
		server := <-f.opened
		s := containerterm.Attach(terminal, 80, 24, nil)
		defer s.Close()
		waitText(s, "shell-ready")
		s.Text("printf 'unicode-%s\\n' 한글\r", false)
		waitText(s, "unicode-한글")
		s.Resize(73, 19)
		s.Text("stty size\r", false)
		waitText(s, "19 73")
		s.Text("sleep 30\r", false)
		time.Sleep(100 * time.Millisecond)
		s.Key(uv.KeyPressEvent{Code: 'c', Mod: uv.ModCtrl})
		s.Text("printf 'interrupted-ok\\n'\r", false)
		waitText(s, "interrupted-ok")
		s.Text("exit\r", false)
		reaped(server)
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			if done, err := s.Exited(); done {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatal("exit status not delivered")
	})
	t.Run("nonzero", func(t *testing.T) {
		terminal, err := c.OpenTerminal(ctx, "project", 80, 24)
		if err != nil {
			t.Fatal(err)
		}
		defer terminal.Close()
		server := <-f.opened
		if _, err := terminal.Write([]byte("exit 7\r")); err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, terminal)
		if err := terminal.Wait(); err == nil || !strings.Contains(err.Error(), "7") {
			t.Fatal("nonzero exit lost", err)
		}
		reaped(server)
	})
	t.Run("disconnect", func(t *testing.T) {
		terminal, err := c.OpenTerminal(ctx, "project", 80, 24)
		if err != nil {
			t.Fatal(err)
		}
		server := <-f.opened
		terminal.Close()
		_, _ = io.Copy(io.Discard, terminal)
		if terminal.Wait() == nil {
			t.Fatal("disconnect became successful shell exit")
		}
		reaped(server)
	})
	t.Run("validate", func(t *testing.T) {
		for _, first := range []*resource.ProjectTerminalRequest{
			resource.ProjectTerminalRequest_builder{Columns: ptr(uint32(80)), Rows: ptr(uint32(24))}.Build(),
			resource.ProjectTerminalRequest_builder{Ref: projectRef("project"), Columns: ptr(uint32(501)), Rows: ptr(uint32(24))}.Build(),
			resource.ProjectTerminalRequest_builder{Ref: projectRef("project"), Columns: ptr(uint32(80)), Rows: ptr(uint32(0))}.Build(),
			resource.ProjectTerminalRequest_builder{Ref: projectRef("project"), Columns: ptr(uint32(80)), Rows: ptr(uint32(24)), Input: []byte("ls")}.Build(),
		} {
			stream, err := resource.NewProjectServiceClient(conn).Terminal(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if err := stream.Send(first); err != nil {
				t.Fatal(err)
			}
			if _, err := stream.Recv(); status.Code(err) != codes.InvalidArgument {
				t.Fatal("invalid open accepted", err)
			}
		}
		for _, frame := range []*resource.ProjectTerminalRequest{
			resource.ProjectTerminalRequest_builder{Ref: projectRef("other"), Input: []byte("ls")}.Build(),
			resource.ProjectTerminalRequest_builder{Input: make([]byte, 32769)}.Build(),
			resource.ProjectTerminalRequest_builder{Columns: ptr(uint32(0)), Rows: ptr(uint32(10))}.Build(),
			resource.ProjectTerminalRequest_builder{Columns: ptr(uint32(80)), Rows: ptr(uint32(24)), Input: []byte("ls")}.Build(),
		} {
			stream, err := resource.NewProjectServiceClient(conn).Terminal(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if err := stream.Send(resource.ProjectTerminalRequest_builder{Ref: projectRef("project"), Columns: ptr(uint32(80)), Rows: ptr(uint32(24))}.Build()); err != nil {
				t.Fatal(err)
			}
			if ready, err := stream.Recv(); err != nil || !ready.GetReady() {
				t.Fatal("not ready", err)
			}
			server := <-f.opened
			if err := stream.Send(frame); err != nil {
				t.Fatal(err)
			}
			for {
				if _, err := stream.Recv(); err != nil {
					if status.Code(err) != codes.InvalidArgument {
						t.Fatal("invalid frame accepted", err)
					}
					break
				}
			}
			reaped(server)
		}
		if _, err := c.OpenTerminal(ctx, "missing", 80, 24); status.Code(err) != codes.NotFound {
			t.Fatal("unknown project accepted", err)
		}
	})
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM audit").Scan(&after); err != nil || before != after {
		t.Fatal("terminal input entered audit", before, after, err)
	}
	if f.snapshots.Load() != 0 {
		t.Fatal("terminal refreshed full inventory")
	}
	layer, _ := resource.Find[Layer](stack)
	if _, err := layer.Next().Project().Patch(ctx, resource.ProjectPatchRequest_builder{Ref: projectRef("project"), Listed: ptr(false), DateUpdatedForce: ptr(true)}.Build()); err != nil {
		t.Fatal(err)
	}
	if _, err := c.OpenTerminal(ctx, "project", 80, 24); status.Code(err) != codes.NotFound {
		t.Fatal("deleted project accepted", err)
	}
}
