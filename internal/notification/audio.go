// Package notification plays short local session alerts using OS audio tools.
package notification

import (
	"context"
	_ "embed"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

type Sound bool

const (
	Complete  Sound = false
	Attention Sound = true
)

// These original PCM tones require no codec or downloaded assets.
//
//go:embed sounds/complete.wav
var complete []byte

//go:embed sounds/attention.wav
var attention []byte

// Player serializes alerts so simultaneous sessions do not overlap audio.
// False means the caller should ring its terminal bell instead.
type Player struct {
	mu sync.Mutex
}

func (p *Player) Play(ctx context.Context, sound Sound) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return play(ctx, sound, runtime.GOOS, os.Getenv, exec.LookPath, run)
}

func run(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.WaitDelay = 100 * time.Millisecond
	return cmd.Run()
}

func play(ctx context.Context, sound Sound, platform string, getenv func(string) string, lookup func(string) (string, error), execute func(context.Context, string, ...string) error) bool {
	if ctx.Err() != nil {
		return false
	}
	for _, key := range []string{"SSH_CONNECTION", "SSH_CLIENT", "SSH_TTY"} {
		if getenv(key) != "" {
			return false
		}
	}
	var names []string
	switch platform {
	case "darwin":
		names = []string{"afplay"}
	case "linux":
		names = []string{"paplay", "aplay"}
	case "windows":
		names = []string{"powershell.exe"}
	default:
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	var path string
	defer func() {
		if path != "" {
			_ = os.Remove(path)
		}
	}()
	for _, name := range names {
		tool, err := lookup(name)
		if err != nil {
			continue
		}
		if path == "" {
			file, err := os.CreateTemp("", "cxz-alert-*.wav")
			if err != nil {
				return false
			}
			path = file.Name()
			data := complete
			if sound == Attention {
				data = attention
			}
			_, writeErr := file.Write(data)
			closeErr := file.Close()
			if writeErr != nil || closeErr != nil {
				return false
			}
		}
		args := []string{path}
		if platform == "windows" {
			// PowerShell single-quoted literals escape a quote by doubling it.
			literal := "'" + strings.ReplaceAll(path, "'", "''") + "'"
			args = []string{"-NoProfile", "-NonInteractive", "-Command", "$ErrorActionPreference = 'Stop'; $s = New-Object System.Media.SoundPlayer; $s.SoundLocation = " + literal + "; try { $s.Load(); $s.PlaySync() } finally { $s.Dispose() }"}
		}
		if execute(ctx, tool, args...) == nil {
			return true
		}
		if ctx.Err() != nil {
			return false
		}
	}
	return false
}
