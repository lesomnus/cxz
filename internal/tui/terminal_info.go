package tui

import (
	"context"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/term"
	"github.com/muesli/termenv"
)

// RunTerminalInfo is local-only and never connects to the manager.
func RunTerminalInfo(ctx context.Context, in io.Reader, out io.Writer, plain bool) error {
	m := newTerminalInfo(in, out)
	if plain || !terminalFile(in) || !terminalFile(out) {
		_, err := fmt.Fprintln(out, m.report())
		return err
	}
	if f, ok := in.(*os.File); ok {
		in = keyboardInput(f)
	}
	p := tea.NewProgram(m, tea.WithContext(ctx), tea.WithInput(in), tea.WithOutput(out), tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, err := runKeyboardProgram(ctx, p, in, nil)
	return err
}

func terminalFile(v any) bool {
	f, ok := v.(interface{ Fd() uintptr })
	return ok && term.IsTerminal(f.Fd())
}

type terminalInfo struct {
	width, height, selected, firstRow int
	rgb                               bool
	environment                       []string
}

func newTerminalInfo(in io.Reader, out io.Writer) *terminalInfo {
	m := &terminalInfo{width: 80, height: 24}
	m.environment = append(m.environment, fmt.Sprintf("OS: %s/%s · stdin TTY: %t · stdout TTY: %t", runtime.GOOS, runtime.GOARCH, terminalFile(in), terminalFile(out)))
	for _, names := range [][]string{{"TERM", "COLORTERM"}, {"TERM_PROGRAM", "TERM_PROGRAM_VERSION"}} {
		var values []string
		for _, name := range names {
			value := strings.Join(strings.Fields(safeText(os.Getenv(name))), " ")
			if value == "" {
				value = "(unset)"
			}
			values = append(values, name+"="+value)
		}
		m.environment = append(m.environment, strings.Join(values, " · "))
	}
	profile := termenv.NewOutput(out).Profile
	label := map[termenv.Profile]string{termenv.Ascii: "none", termenv.ANSI: "ANSI 16", termenv.ANSI256: "ANSI 256", termenv.TrueColor: "truecolor"}[profile]
	m.environment = append(m.environment, fmt.Sprintf("Detected color: %s · NO_COLOR: %t · tmux: %t · SSH: %t", label, os.Getenv("NO_COLOR") != "", os.Getenv("TMUX") != "", os.Getenv("SSH_CONNECTION") != ""))
	if f, ok := out.(interface{ Fd() uintptr }); ok {
		if w, h, err := term.GetSize(f.Fd()); err == nil {
			m.width, m.height = w, h
		}
	}
	return m
}

// These are xterm reference values, not queried terminal theme colors.
func paletteRGB(index int) (int, int, int) {
	base := [16][3]int{{0, 0, 0}, {128, 0, 0}, {0, 128, 0}, {128, 128, 0}, {0, 0, 128}, {128, 0, 128}, {0, 128, 128}, {192, 192, 192}, {128, 128, 128}, {255, 0, 0}, {0, 255, 0}, {255, 255, 0}, {0, 0, 255}, {255, 0, 255}, {0, 255, 255}, {255, 255, 255}}
	if index < 16 {
		c := base[index]
		return c[0], c[1], c[2]
	}
	if index >= 232 {
		v := 8 + (index-232)*10
		return v, v, v
	}
	i := index - 16
	levels := [6]int{0, 95, 135, 175, 215, 255}
	return levels[i/36], levels[i/6%6], levels[i%6]
}
func paletteHex(index int) string {
	r, g, b := paletteRGB(index)
	return fmt.Sprintf("#%02X%02X%02X", r, g, b)
}

const paletteTop = 8
const paletteCell = 6

func (m *terminalInfo) layout() (columns, rows int) {
	return max(1, min(16, m.width/paletteCell)), max(1, m.height-paletteTop-4)
}
func (m *terminalInfo) reveal() {
	cols, rows := m.layout()
	row := m.selected / cols
	if row < m.firstRow {
		m.firstRow = row
	}
	if row >= m.firstRow+rows {
		m.firstRow = row - rows + 1
	}
	m.firstRow = max(0, min(m.firstRow, max(0, (255/cols)+1-rows)))
}
func (m *terminalInfo) Init() tea.Cmd { return nil }
func (m *terminalInfo) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	cols, rows := m.layout()
	switch v := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = v.Width, v.Height
	case tea.KeyMsg:
		if v.Paste {
			return m, nil
		}
		switch v.String() {
		case "q", "esc", "ctrl+c":
			return m, tea.Quit
		case "left":
			m.selected--
		case "right":
			m.selected++
		case "up":
			m.selected -= cols
		case "down":
			m.selected += cols
		case "pgup":
			m.selected -= cols * rows
		case "pgdown":
			m.selected += cols * rows
		case "home":
			m.selected = 0
		case "end":
			m.selected = 255
		case "tab", "shift+tab":
			m.rgb = !m.rgb
		}
	case tea.MouseMsg:
		if v.Button == tea.MouseButtonWheelUp {
			m.selected -= cols
		}
		if v.Button == tea.MouseButtonWheelDown {
			m.selected += cols
		}
		if v.Action == tea.MouseActionPress && v.Button == tea.MouseButtonLeft && v.X >= 0 && v.X < cols*paletteCell && v.Y >= paletteTop && v.Y < paletteTop+rows {
			index := (m.firstRow+v.Y-paletteTop)*cols + v.X/paletteCell
			if index < 256 {
				m.selected = index
			}
		}
	}
	m.selected = max(0, min(255, m.selected))
	m.reveal()
	return m, nil
}
func (m *terminalInfo) header() []string {
	lines := []string{"Terminal environment & palette"}
	lines = append(lines, m.environment...)
	lines = append(lines, fmt.Sprintf("Size: %d × %d", m.width, m.height))
	mode := "ANSI indexed · terminal theme colors"
	if m.rgb {
		mode = "RGB · explicit reference HEX colors"
	}
	return append(lines, mode, "←↑↓→ / click select · Tab ANSI/RGB · PgUp/PgDn · Esc exit")
}
func (m *terminalInfo) selection() string {
	if m.rgb {
		return fmt.Sprintf("HEX %s · RGB", paletteHex(m.selected))
	}
	return fmt.Sprintf("ANSI %d · %s ref", m.selected, paletteHex(m.selected))
}
func (m *terminalInfo) View() string {
	if m.width < 24 || m.height < 14 {
		return ansi.Truncate("Enlarge terminal (24×14 minimum) · Esc exit", max(1, m.width), "")
	}
	m.reveal()
	cols, rows := m.layout()
	lines := m.header()
	for row := m.firstRow; row < min(m.firstRow+rows, 255/cols+1); row++ {
		var line strings.Builder
		for col := 0; col < cols; col++ {
			i := row*cols + col
			if i >= 256 {
				break
			}
			r, g, b := paletteRGB(i)
			bg := fmt.Sprintf("48;5;%d", i)
			fg := "38;5;15"
			if r*299+g*587+b*114 > 128000 {
				fg = "38;5;0"
			}
			if m.rgb {
				bg = fmt.Sprintf("48;2;%d;%d;%d", r, g, b)
				fg = "38;2;255;255;255"
				if r*299+g*587+b*114 > 128000 {
					fg = "38;2;0;0;0"
				}
			}
			left, right := " ", " "
			if i == m.selected {
				left, right = "[", "]"
			}
			fmt.Fprintf(&line, "%s\x1b[%s;%sm%3d \x1b[0m%s", left, bg, fg, i, right)
		}
		lines = append(lines, line.String())
	}
	lines = append(lines, "", m.selection(), "ANSI colors may be customized; HEX is a reference, not a theme measurement.", "Share ANSI <number> or switch to RGB and share HEX #RRGGBB.")
	for i, line := range lines {
		lines[i] = ansi.Truncate(line, m.width, "")
	}
	return strings.Join(lines, "\n")
}
func (m *terminalInfo) report() string {
	lines := m.header()
	lines = append(lines, "ANSI index / reference HEX (actual ANSI colors depend on terminal theme):")
	for i := 0; i < 256; i++ {
		lines = append(lines, fmt.Sprintf("%3d %s", i, paletteHex(i)))
	}
	return strings.Join(lines, "\n")
}
