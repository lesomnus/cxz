package containerterm

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/dockerx"
)

func waitText(t *testing.T, s *Session, text string) {
	t.Helper()
	until := time.Now().Add(8 * time.Second)
	for time.Now().Before(until) {
		if strings.Contains(ansi.Strip(s.Screen.Render()), text) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("missing %q: %q", text, s.Screen.Render())
}
func exercise(t *testing.T, s *Session) {
	t.Helper()
	if !s.Text("printf 'pty-%s\\n' ready\r", false) {
		t.Fatal("input rejected")
	}
	waitText(t, s, "pty-ready")
	s.Text("printf 'unicode-%s\\n' '한글입력'\r", false)
	waitText(t, s, "unicode-한글입력")
	s.Resize(73, 19)
	// Docker forwards SIGWINCH asynchronously; a command already in flight can
	// still observe the old size. Wait for the remote PTY, not just the emulator.
	resizeDeadline := time.Now().Add(4 * time.Second)
	for !strings.Contains(ansi.Strip(s.Screen.Render()), "19 73") && time.Now().Before(resizeDeadline) {
		s.Text("stty size\r", false)
		time.Sleep(100 * time.Millisecond)
	}
	waitText(t, s, "19 73")
	s.Text("sleep 30\r", false)
	time.Sleep(100 * time.Millisecond)
	s.Key(uv.KeyPressEvent{Code: 'c', Mod: uv.ModCtrl})
	s.Text("printf 'interrupt-%s\\n' ok\r", false)
	waitText(t, s, "interrupt-ok")
	s.Text("exit\r", false)
	until := time.Now().Add(5 * time.Second)
	for time.Now().Before(until) {
		if done, _ := s.Exited(); done {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("shell did not exit")
}
func TestPTYInteraction(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	s, err := Start(exec.CommandContext(ctx, "sh"), 80, 24, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	exercise(t, s)
}

func TestZshUTF8ShellLocale(t *testing.T) {
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh not installed")
	}
	for _, fixed := range []bool{false, true} {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		script := "exec zsh -f"
		if fixed {
			script = utf8ShellLocale + script
		}
		cmd := exec.CommandContext(ctx, "sh", "-c", script)
		cmd.Env = append(os.Environ(), "LC_ALL=not_an_installed_locale", "LANG=C", "TERM=xterm-256color", "PS1=READY> ")
		s, err := Start(cmd, 100, 24, nil)
		if err != nil {
			cancel()
			t.Fatal(err)
		}
		waitText(t, s, "READY>")
		s.Text("echo 한글입력", false)
		if fixed {
			waitText(t, s, "한글입력")
			before := s.Screen.CursorPosition()
			s.Key(uv.KeyPressEvent{Code: uv.KeyLeft})
			until := time.Now().Add(2 * time.Second)
			for s.Screen.CursorPosition() == before && time.Now().Before(until) {
				time.Sleep(10 * time.Millisecond)
			}
			if s.Screen.CursorPosition().X >= before.X {
				t.Fatal("zsh left arrow did not move Unicode cursor")
			}
			s.Text("\rlocale charmap\r", false)
			waitText(t, s, "UTF-8")
		} else {
			time.Sleep(150 * time.Millisecond)
			t.Logf("invalid locale observation (vendor symptom not guaranteed): %q", ansi.Strip(s.Screen.Render()))
		}
		if fixed && strings.Contains(s.Screen.Render(), "<ffffffff>") {
			t.Fatal("invalid Unicode remains")
		}
		s.Close()
		cancel()
	}
}
func TestDockerTerminal(t *testing.T) {
	if os.Getenv("CXZ_TERMINAL_DOCKER_TEST") != "1" {
		t.Skip("opt-in owned Docker fixture")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	owner, project := core.ID(), core.ID()
	image := os.Getenv("CXZ_TERMINAL_TEST_IMAGE")
	if image == "" {
		image = "alpine:3.22"
	}
	b, err := dockerx.Run(ctx, "run", "-d", "--label", "cxz.owner="+owner, "--label", "cxz.project="+project, image, "sleep", "60")
	if err != nil {
		t.Fatal(err)
	}
	id := strings.TrimSpace(string(b))
	t.Cleanup(func() {
		cleanup, c := context.WithTimeout(context.Background(), 10*time.Second)
		defer c()
		if _, err := dockerx.Owned(cleanup, id, owner, project); err == nil {
			_, _ = dockerx.Run(cleanup, "rm", "-f", id)
		}
	})
	p := &api.Project{Id: project, ContainerId: id, RemoteUser: "root", RemoteWorkspace: "/tmp"}
	paths, err := ListPaths(ctx, p, "/")
	if err != nil {
		t.Fatal(err)
	}
	foundTmp := false
	for _, entry := range paths.Entries {
		if entry.Name == "tmp" && entry.Directory {
			foundTmp = true
		}
	}
	if !foundTmp {
		t.Fatal("container root listing missing tmp")
	}
	if _, err := ListPaths(ctx, p, "~/"); err != nil {
		t.Fatal("container home expansion failed", err)
	}
	s, err := Open(ctx, p, 80, 24, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.Text("pwd\r", false)
	waitText(t, s, "/tmp")
	exercise(t, s)
	p.RemoteUser = "65534"
	nonroot, err := Open(ctx, p, 80, 24, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer nonroot.Close()
	nonroot.Text("id -u\r", false)
	waitText(t, nonroot, "65534")
	p.Id = "unowned"
	if _, err := ListPaths(ctx, p, "/"); err == nil {
		t.Fatal("path lookup accepted wrong project")
	}
	if s, err := Open(ctx, p, 80, 24, nil); err == nil {
		s.Close()
		t.Fatal("wrong project accepted")
	}
}
