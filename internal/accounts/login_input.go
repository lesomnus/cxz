package accounts

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/term"
)

func terminalInput(input io.Reader) bool {
	f, ok := input.(term.File)
	return ok && term.IsTerminal(f.Fd())
}

type loginOutput string
type loginFinished struct{ err error }
type loginSubmitted struct{ err error }

type loginInput struct {
	input                                     textinput.Model
	stdin                                     io.Writer
	pending                                   string
	submitting, submitted, canceled, finished bool
	err                                       error
}

func newLoginInput(stdin io.Writer) *loginInput {
	input := textinput.New()
	input.EchoMode = textinput.EchoNone
	input.CharLimit = 16384
	input.Focus()
	return &loginInput{input: input, stdin: stdin}
}

func (m *loginInput) Init() tea.Cmd { return textinput.Blink }

func (m *loginInput) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.finished {
		return m, nil
	}
	switch msg := msg.(type) {
	case loginFinished:
		m.err, m.finished = msg.err, true
		m.input.Reset()
		return m, tea.Quit
	case loginSubmitted:
		m.submitting = false
		if msg.err != nil {
			m.err, m.finished = fmt.Errorf("cannot submit login code"), true
			return m, tea.Quit
		}
		m.submitted = true
		return m, nil
	case loginOutput:
		// The subprocess has pipes, not a terminal: readline must not echo the
		// secret. Keep vendor URL/instructions above our single-line indicator.
		m.pending += strings.ReplaceAll(string(msg), "\r\n", "\n")
		if i := strings.LastIndex(m.pending, "\n"); i >= 0 {
			lines := ansi.Strip(m.pending[:i])
			m.pending = m.pending[i+1:]
			return m, tea.Println(lines)
		}
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			m.canceled, m.finished = true, true
			m.input.Reset()
			return m, tea.Quit
		case "ctrl+x":
			m.input.Reset()
			m.submitted = false
			return m, nil
		case "enter":
			if m.input.Value() == "" || m.submitting {
				return m, nil
			}
			code := m.input.Value()
			m.input.Reset()
			m.submitting = true
			return m, func() tea.Msg {
				_, err := io.WriteString(m.stdin, code+"\n")
				return loginSubmitted{err: err}
			}
		}
		if m.submitting {
			return m, nil
		}
		m.submitted = false
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m *loginInput) View() string {
	if m.finished {
		return ""
	}
	n := utf8.RuneCountInString(m.input.Value())
	digits := fmt.Sprintf("%03d", n)
	padding := len(digits) - len(fmt.Sprint(n))
	digits = lipgloss.NewStyle().Foreground(lipgloss.Color("#30343B")).Render(digits[:padding]) + digits[padding:]
	m.input.Cursor.SetChar(" ")
	indicator := "[" + strings.Repeat("*", min(3, n)) + strings.Repeat(" ", max(0, 3-n)) + "]" + m.input.Cursor.View() + digits + "; ctrl+x to clear."
	if m.submitting || m.submitted {
		indicator = "[   ] submitted; waiting for Claude."
	}
	return ansi.Strip(m.pending) + "\n" + indicator + "\nEnter to submit; Esc/Ctrl-C to cancel.\n"
}

type loginWriter struct{ program *tea.Program }

func (w loginWriter) Write(p []byte) (int, error) {
	w.program.Send(loginOutput(string(p)))
	return len(p), nil
}

// OAuth remains entirely in the official CLI. Only interactive code input is
// owned here; pipes prevent the child from echoing it or changing terminal mode.
func runClaudeLogin(ctx context.Context, cmd *exec.Cmd, input io.Reader, output io.Writer) error {
	uiCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd.Stdin = nil
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	defer stdin.Close()
	m := newLoginInput(stdin)
	p := tea.NewProgram(m, tea.WithContext(uiCtx), tea.WithInput(input), tea.WithOutput(output))
	cmd.Stdout, cmd.Stderr = loginWriter{p}, loginWriter{p}
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() {
		err := cmd.Wait()
		done <- err
		p.Send(loginFinished{err})
	}()
	_, runErr := p.Run()
	cancel() // Unblock output writers even if terminal initialization failed.
	// Also unblock a pending stdin write if the user cancels before submission.
	stdin.Close()
	_ = cmd.Process.Kill()
	processErr := <-done
	m.input.Reset()
	if runErr != nil {
		return runErr
	}
	if m.canceled {
		return fmt.Errorf("Claude login canceled")
	}
	if m.err != nil {
		return m.err
	}
	return processErr
}
