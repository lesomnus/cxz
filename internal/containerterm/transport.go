package containerterm

import (
	"context"
	"io"
	"os"
	"os/exec"
	"sync"

	"github.com/creack/pty"
)

// Terminal transports raw terminal bytes, with a single reader and serialized
// writes/resizes. Close must unblock all IO. Call Wait once, after draining Read.
type Terminal interface {
	io.ReadWriteCloser
	Resize(columns, rows int) error
	Wait() error
}

type TerminalClient interface {
	OpenTerminal(context.Context, string, int, int) (Terminal, error)
}

func Dimensions(columns, rows int) (int, int) {
	return max(1, min(columns, 500)), max(1, min(rows, 100))
}

type localPTY struct {
	*os.File
	cmd    *exec.Cmd
	mu     sync.Mutex
	closed bool
}

func StartPTY(cmd *exec.Cmd, columns, rows int) (Terminal, error) {
	columns, rows = Dimensions(columns, rows)
	f, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: uint16(columns), Rows: uint16(rows)})
	if err != nil {
		return nil, err
	}
	return &localPTY{File: f, cmd: cmd}, nil
}

func (p *localPTY) Resize(columns, rows int) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return os.ErrClosed
	}
	columns, rows = Dimensions(columns, rows)
	return pty.Setsize(p.File, &pty.Winsize{Cols: uint16(columns), Rows: uint16(rows)})
}
func (p *localPTY) Wait() error { return p.cmd.Wait() }
func (p *localPTY) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.closed {
		p.closed = true
		_ = p.File.Close()
		_ = p.cmd.Process.Kill()
	}
	return nil
}
