package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/agentview"
	"github.com/muesli/termenv"
)

// The bright green marks what is live: where the keyboard is, and what is
// running. A list cursor, a footer hint under the pointer and a working spinner
// all qualify, so they carry it rather than the step the rest of the green
// chrome sits on.
func TestCursorAndHoverTakeTheFocusStep(t *testing.T) {
	profile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(profile) })
	bright, quiet := sgr(focus), sgr(accent)
	if bright == quiet {
		t.Fatal("the two greens render the same")
	}

	hint := hintSpec{key: "n", label: "new"}
	if hovered := hint.render(true); !strings.Contains(hovered, bright) || strings.Contains(hovered, quiet) {
		t.Fatalf("a hovered hint is not on the focus step: %q", hovered)
	}
	if idle := hint.render(false); !strings.Contains(idle, quiet) || strings.Contains(idle, bright) {
		t.Fatalf("an idle hint left the quiet step: %q", idle)
	}

	m := conversationModel()
	if spinner := m.sessionIndicator(&api.Session{Id: "s", State: "working"}); !strings.Contains(spinner, bright) {
		t.Fatalf("a working spinner is not on the focus step: %q", spinner)
	}

	m.modelPicker = &modelPicker{id: "s", run: "run", kind: "/model", catalog: &modelCatalog{
		Models: []agentview.ModelOption{{ID: "first"}, {ID: "second"}},
	}}
	m.modelPicker.selected = 1
	for _, row := range strings.Split(m.modelPickerOverlay(strings.Repeat("\n", 20)), "\n") {
		if !strings.Contains(row, "› second") {
			continue
		}
		if !strings.Contains(row, bright) {
			t.Fatalf("the row under the cursor is not on the focus step: %q", row)
		}
		return
	}
	t.Fatal("the picker did not draw a cursor")
}
