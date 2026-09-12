package tui

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/resource"
)

func pickerAccounts() []*resource.Account {
	return []*resource.Account{
		resource.Account_builder{Alias: "main", Name: "Personal", Agent: "claude"}.Build(),
		resource.Account_builder{Alias: "work", Name: "Company Codex", Agent: "codex"}.Build(),
		resource.Account_builder{Alias: "backup", Name: "Company Codex", Agent: "codex"}.Build(),
		resource.Account_builder{Alias: "local", Name: "개인 계정", Agent: "claude"}.Build(),
	}
}

func TestAccountPickerSearch(t *testing.T) {
	for _, tc := range []struct {
		query string
		want  []int
	}{
		{"", []int{0, 1, 2, 3}}, {"1", []int{0}}, {"3", []int{2}},
		{"MAIN", []int{0}}, {"personal", []int{0}}, {"  work  ", []int{1}},
		{"Company Codex", []int{1, 2}}, {"comp", []int{1, 2}},
		{"codex company", []int{1, 2}}, {"개인", []int{3}},
		{"claude", []int{0, 3}}, {"missing", nil}, {"99", nil},
	} {
		t.Run(tc.query, func(t *testing.T) {
			m := newAccountPicker(pickerAccounts())
			m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(tc.query)})
			if !reflect.DeepEqual(m.matches, tc.want) {
				t.Fatalf("%q: %v", tc.query, m.matches)
			}
		})
	}
}

func TestAccountPickerNavigationAndEmptySearch(t *testing.T) {
	m := newAccountPicker(pickerAccounts())
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("company")})
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.selected != 1 {
		t.Fatal(m.selected)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.chosen != "work" {
		t.Fatal(m.chosen)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.chosen != "work" {
		t.Fatal("selection changed after quit")
	}

	m = newAccountPicker(pickerAccounts())
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("z")})
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil || m.chosen != "" || !strings.Contains(m.View(), "No matching") {
		t.Fatal("empty result accepted")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if len(m.matches) != 4 || m.selected != 0 {
		t.Fatal("search not reset")
	}
	for _, key := range []tea.KeyType{tea.KeyEsc, tea.KeyCtrlC} {
		m = newAccountPicker(pickerAccounts())
		_, cmd = m.Update(tea.KeyMsg{Type: key})
		if cmd == nil || !m.finished || m.chosen != "" {
			t.Fatal("cancel selected an account")
		}
	}
}

func TestAccountPickerViewport(t *testing.T) {
	var accounts []*resource.Account
	for i := 0; i < 250; i++ {
		accounts = append(accounts, resource.Account_builder{Alias: fmt.Sprintf("account-%d", i), Name: strings.Repeat("긴이름", 50)}.Build())
	}
	m := newAccountPicker(accounts)
	m.Update(tea.WindowSizeMsg{Width: 40, Height: 12})
	for i := 0; i < 249; i++ {
		m.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "> 250) account-249") || !strings.Contains(view, "Search:") {
		t.Fatal(view)
	}
	lines := strings.Split(view, "\n")
	if len(lines) > 12 || !strings.HasPrefix(lines[len(lines)-2], "Search:") {
		t.Fatal(view)
	}
	for _, line := range lines {
		if ansi.StringWidth(line) > 40 {
			t.Fatal("overflow", line)
		}
	}
}

// Exercise Bubble Tea's input decoder and program lifecycle, not only Update.
// In particular a typed alias must be consumed whole, unlike the old Fscanln.
func TestSelectAccountProgram(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"main\r", "main"}, {"2\r", "work"}, {"Personal\r", "main"},
		{"\x1b[B\r", "work"}, {"company\x1b[B\r", "backup"},
		{"missing\x03", ""}, {"\x1b", ""},
	} {
		t.Run(fmt.Sprintf("%q", tc.input), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			input := strings.NewReader(tc.input)
			var output bytes.Buffer
			got, err := SelectAccount(ctx, pickerAccounts(), input, &output)
			if tc.want == "" {
				if !errors.Is(err, ErrAccountSelectionCanceled) {
					t.Fatal(err)
				}
			} else if err != nil || got != tc.want {
				t.Fatalf("%q: %v", got, err)
			}
			if input.Len() != 0 {
				t.Fatal("unread input", input.Len())
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := SelectAccount(ctx, pickerAccounts(), strings.NewReader(""), &bytes.Buffer{}); err == nil {
		t.Fatal("ignored canceled context")
	}
	if _, err := SelectAccount(context.Background(), nil, strings.NewReader(""), &bytes.Buffer{}); err == nil {
		t.Fatal("accepted empty list")
	}
}
