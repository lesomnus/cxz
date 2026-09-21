//go:build !windows

package tui

import (
	"context"
	"io"

	tea "github.com/charmbracelet/bubbletea"
)

func runKeyboardProgram(_ context.Context, p *tea.Program, _ io.Reader, _ *debugRecorder) (tea.Model, error) {
	return p.Run()
}
