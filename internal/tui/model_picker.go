package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/agentview"
)

type modelCatalog struct {
	Models        []agentview.ModelOption
	Model, Effort string
}
type modelPicker struct {
	id, run, kind, query, requested, message string
	epoch                                    uint64
	selected                                 int
	loading                                  bool
	catalog                                  *modelCatalog
	cancel                                   context.CancelFunc
}
type modelCatalogLoaded struct {
	epoch   uint64
	catalog *modelCatalog
	err     error
}

func (m *model) openModelPicker(command string) tea.Cmd {
	s := m.current()
	if s == nil {
		return nil
	}
	m.modelPickerEpoch++
	m.report = nil
	if m.modelPicker != nil && m.modelPicker.cancel != nil {
		m.modelPicker.cancel()
	}
	f := strings.Fields(command)
	p := &modelPicker{id: s.Id, run: s.RunId, kind: f[0], epoch: m.modelPickerEpoch, loading: true}
	if len(f) > 1 {
		p.requested = command
	}
	m.modelPicker = p
	// Read-only history lookup: a missing capability must NEVER fall back to
	// sending a slash command as an agent prompt, including on older servers.
	client := m.client
	ctx, cancel := context.WithTimeout(m.ctx, 20*time.Second)
	p.cancel = cancel
	return func() tea.Msg {
		defer cancel()
		var catalog *modelCatalog
		var after uint64
		for {
			batch, err := client.History(ctx, &api.WatchRequest{SessionId: p.id, AfterSeq: after})
			if err != nil {
				return modelCatalogLoaded{epoch: p.epoch, err: err}
			}
			if len(batch.Events) == 0 {
				break
			}
			previous := after
			for _, e := range batch.Events {
				after = max(after, e.Seq)
				if e.Kind == "models" && e.RunId == p.run && p.run != "" {
					var v modelCatalog
					if json.Unmarshal(e.Payload, &v) == nil {
						catalog = &v
					}
				}
			}
			if after == previous {
				return modelCatalogLoaded{epoch: p.epoch, err: fmt.Errorf("history cursor did not advance")}
			}
		}
		return modelCatalogLoaded{epoch: p.epoch, catalog: catalog}
	}
}

func (m *model) acceptModelCatalog(v modelCatalogLoaded) tea.Cmd {
	p := m.modelPicker
	if p == nil || p.epoch != v.epoch {
		return nil
	}
	p.loading = false
	if v.err != nil {
		p.message = "Catalog lookup failed; no command sent. " + v.err.Error()
		return nil
	}
	p.catalog = v.catalog
	if p.catalog == nil {
		p.message = "This run has no model-control capability record. Update manager and project runtime; restart the agent. No command was sent."
		return nil
	}
	if p.requested != "" {
		return m.submitModelChoice(p.requested)
	}
	return nil
}

func (p *modelPicker) options() []string {
	if p.catalog == nil {
		return nil
	}
	var options []string
	if p.kind == "/model" {
		for _, v := range p.catalog.Models {
			options = append(options, v.ID)
		}
	} else {
		options = append(options, "default")
		for _, v := range p.catalog.Models {
			if v.ID == p.catalog.Model || p.catalog.Model == "" && v.Default {
				options = append(options, v.Efforts...)
				break
			}
		}
	}
	var matches []string
	for _, v := range options {
		if fuzzyScore(v, p.query) >= 0 {
			matches = append(matches, v)
		}
	}
	return matches
}

func (m *model) submitModelChoice(command string) tea.Cmd {
	p, s := m.modelPicker, m.current()
	if p == nil || p.catalog == nil || s == nil {
		return nil
	}
	if s.Id != p.id || s.RunId != p.run {
		p.message = "Session run changed. Close and reopen the selector."
		return nil
	}
	if s.State != "idle" {
		p.message = "Wait until the session is idle."
		return nil
	}
	f := strings.Fields(command)
	if len(f) != 2 {
		p.message = "Usage: " + p.kind + " <value>"
		return nil
	}
	// The runtime validates model-specific effort and policy again.
	m.modelPicker = nil
	return m.action("send", command)
}

func (m *model) modelPickerKey(key tea.KeyMsg) tea.Cmd {
	p := m.modelPicker
	switch key.String() {
	case "esc", "ctrl+q":
		if p.cancel != nil {
			p.cancel()
		}
		m.modelPicker = nil
		return nil
	case "ctrl+c":
		if p.cancel != nil {
			p.cancel()
		}
		return tea.Quit
	case "up":
		p.selected = max(0, p.selected-1)
	case "down":
		p.selected = min(max(0, len(p.options())-1), p.selected+1)
	case "enter", "ctrl+s":
		options := p.options()
		if !p.loading && len(options) > 0 {
			return m.submitModelChoice(p.kind + " " + options[min(p.selected, len(options)-1)])
		}
	case "backspace":
		q := []rune(p.query)
		if len(q) > 0 {
			p.query = string(q[:len(q)-1])
		}
		p.selected = 0
	case "ctrl+x":
		p.query = ""
		p.selected = 0
	default:
		if key.Type == tea.KeyRunes {
			p.query += string(key.Runes)
			p.selected = 0
		}
	}
	return nil
}

func (m *model) modelPickerOverlay(view string) string {
	p := m.modelPicker
	if p == nil {
		return view
	}
	rows := strings.Split(view, "\n")
	width := max(1, m.width-4)
	lines := []string{accent.Bold(true).Render(p.kind + " · select"), muted.Render("↑/↓ choose · Enter apply · Esc cancel")}
	if p.loading {
		lines = append(lines, "Reading provider capabilities…")
	}
	if p.message != "" {
		lines = append(lines, strings.Split(ansi.Hardwrap(safeText(p.message), width, true), "\n")...)
	}
	options := p.options()
	count := min(7, max(0, len(rows)-8))
	selected := min(p.selected, max(0, len(options)-1))
	start := max(0, min(selected-max(0, count-3), len(options)-count))
	for i := start; i < min(len(options), start+count); i++ {
		line := "  " + safeText(options[i])
		if i == selected {
			line = accent.Render("› " + safeText(options[i]))
		}
		lines = append(lines, line)
	}
	if !p.loading && p.catalog != nil && len(options) == 0 {
		lines = append(lines, "No matching provider choices.")
	}
	lines = append(lines, "", "Search: "+safeText(p.query)+"▏")
	return overlayBox(view, lines, m.width, true)
}
