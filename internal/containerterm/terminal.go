package containerterm

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/vt"
	"github.com/creack/pty"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/dockerx"
	"github.com/lesomnus/cxz/internal/transport"
)

// Session belongs to one attached TUI, not to an agent's authentication HOME.
// Folding a panel never closes its PTY. No terminal bytes enter the agent journal.
type Session struct {
	Screen        *vt.SafeEmulator
	file          *os.File
	cmd           *exec.Cmd
	input         chan func()
	done          chan struct{}
	once          sync.Once
	dirty         atomic.Bool
	CursorVisible atomic.Bool
	mu            sync.Mutex
	viewMu        sync.Mutex
	err           error
	w, h          int
}

func Open(ctx context.Context, p *api.Project, width, height int, notify func()) (*Session, error) {
	if err := transport.LocalOnly(ctx, "container terminal"); err != nil {
		return nil, err
	}
	if p == nil || p.Id == "" || p.ContainerId == "" || p.RemoteUser == "" || p.RemoteWorkspace == "" {
		return nil, fmt.Errorf("project container information unavailable; refresh the project")
	}
	check, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	c, err := dockerx.Inspect(check, p.ContainerId)
	if err != nil {
		return nil, err
	}
	if !c.State.Running || c.Config.Labels["cxz.project"] != p.Id || c.Config.Labels["cxz.owner"] == "" {
		return nil, fmt.Errorf("refusing terminal in an unowned or stopped project container")
	}
	cmd := exec.CommandContext(ctx, "docker", "exec", "-it", "--user", p.RemoteUser, "--workdir", p.RemoteWorkspace, "--env", "TERM=xterm-256color", c.ID, "sh", "-c", `
`+utf8ShellLocale+`
if command -v getent >/dev/null 2>&1; then
  entry=$(getent passwd "$(id -u)")
  home=$(printf '%s' "$entry" | cut -d: -f6)
  login=$(printf '%s' "$entry" | cut -d: -f1)
  shell=$(printf '%s' "$entry" | cut -d: -f7)
  if [ -n "$home" ]; then export HOME="$home"; fi
  if [ -n "$login" ]; then export USER="$login" LOGNAME="$login"; fi
  case "$shell" in */nologin|*/false|'') ;; *) SHELL="$shell" ;; esac
fi
exec "${SHELL:-/bin/sh}" -l`)
	return Start(cmd, width, height, notify)
}

// Start is shared by Docker transport and fixture-only PTY tests.
func Start(cmd *exec.Cmd, width, height int, notify func()) (*Session, error) {
	width, height = max(1, min(width, 500)), max(1, min(height, 100))
	f, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: uint16(width), Rows: uint16(height)})
	if err != nil {
		return nil, err
	}
	s := &Session{Screen: vt.NewSafeEmulator(width, height), file: f, cmd: cmd, input: make(chan func(), 64), done: make(chan struct{}), w: width, h: height}
	s.CursorVisible.Store(true)
	s.Screen.SetCallbacks(vt.Callbacks{CursorVisibility: func(visible bool) { s.CursorVisible.Store(visible) }})
	s.Screen.SetScrollbackSize(2000)
	go func() { _, _ = io.Copy(f, s.Screen) }()
	go func() {
		for {
			select {
			case <-s.done:
				return
			case op := <-s.input:
				op()
			}
		}
	}()
	go func() {
		buf := make([]byte, 32768)
		for {
			n, e := f.Read(buf)
			if n > 0 {
				s.viewMu.Lock()
				_, _ = s.Screen.Write(buf[:n])
				s.viewMu.Unlock()
				s.dirty.Store(true)
			}
			if e != nil {
				break
			}
		}
		e := cmd.Wait()
		s.mu.Lock()
		s.err = e
		s.mu.Unlock()
		s.finish()
		if notify != nil {
			notify()
		}
	}()
	go func() {
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-s.done:
				return
			case <-ticker.C:
				if s.dirty.Swap(false) && notify != nil {
					notify()
				}
			}
		}
	}()
	return s, nil
}

func (s *Session) finish() {
	s.once.Do(func() { close(s.done); _ = s.file.Close(); _ = s.Screen.InputPipe().(io.Closer).Close() })
}
func (s *Session) Close() {
	s.finish()
	if s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
	}
}
func (s *Session) Exited() (bool, error) {
	select {
	case <-s.done:
		s.mu.Lock()
		defer s.mu.Unlock()
		return true, s.err
	default:
		return false, nil
	}
}
func (s *Session) Queue(op func()) bool {
	select {
	case <-s.done:
		return false
	default:
	}
	select {
	case s.input <- op:
		return true
	default:
		return false
	}
}
func (s *Session) Key(k uv.KeyPressEvent) bool { return s.Queue(func() { s.Screen.SendKey(k) }) }
func (s *Session) Text(text string, paste bool) bool {
	return s.Queue(func() {
		if paste {
			s.Screen.Paste(text)
		} else {
			s.Screen.SendText(text)
		}
	})
}
func (s *Session) Resize(w, h int) {
	w, h = max(1, min(w, 500)), max(1, min(h, 100))
	if s.w == w && s.h == h {
		return
	}
	if s.Queue(func() { s.viewMu.Lock(); defer s.viewMu.Unlock(); s.Screen.Resize(w, h) }) {
		s.w, s.h = w, h
		_ = pty.Setsize(s.file, &pty.Winsize{Cols: uint16(w), Rows: uint16(h)})
	}
}

// Only the new shell's environment changes; never rewrite user shell rc files.
// Preserve an already working UTF-8 locale, otherwise select an installed one.
const utf8ShellLocale = `
if command -v locale >/dev/null 2>&1; then
  charmap=$(locale charmap 2>/dev/null)
  case "$charmap" in UTF-8|utf8|UTF8) ;;
    *)
      for candidate in C.UTF-8 C.utf8 en_US.UTF-8; do
        charmap=$(LC_ALL="$candidate" locale charmap 2>/dev/null)
        case "$charmap" in UTF-8|utf8|UTF8) export LC_ALL="$candidate" LANG="$candidate"; break;; esac
      done
      ;;
  esac
else
  export LANG=C.UTF-8 LC_CTYPE=C.UTF-8
  unset LC_ALL
fi
`

type Snapshot struct {
	Lines         []uv.Line
	Cursor        uv.Position
	CursorVisible bool
}

// Copy cells while the PTY writer is fenced. Returned snapshots are immutable
// and safe to keep while browsing history, including during ring-buffer eviction.
func (s *Session) Capture(history bool) Snapshot {
	s.viewMu.Lock()
	defer s.viewMu.Unlock()
	w, h := s.Screen.Width(), s.Screen.Height()
	n := 0
	if history && !s.Screen.IsAltScreen() {
		n = s.Screen.ScrollbackLen()
	}
	out := Snapshot{Lines: make([]uv.Line, n+h), Cursor: s.Screen.CursorPosition(), CursorVisible: s.CursorVisible.Load()}
	for y := range out.Lines {
		row := make(uv.Line, w)
		for x := range row {
			var cell *uv.Cell
			if y < n {
				cell = s.Screen.ScrollbackCellAt(x, y)
			} else {
				cell = s.Screen.CellAt(x, y-n)
			}
			if cell != nil {
				row[x] = *cell
			} else {
				row[x] = uv.EmptyCell
			}
		}
		out.Lines[y] = row
	}
	return out
}
