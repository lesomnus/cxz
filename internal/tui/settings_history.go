package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/historypolicy"
	"github.com/lesomnus/cxz/internal/settings"
	"github.com/lesomnus/cxz/internal/versionpin"
	"path/filepath"
)

func (m *model) windowPreference() historypolicy.Window {
	if m.windowPolicy != nil {
		return *m.windowPolicy
	}
	if m.ctx == nil {
		return historypolicy.Window{}
	}
	return settings.From(m.ctx).History
}
func (m *model) historySettingLabel(index int) string {
	if index == 5 || index == 6 {
		p := m.settingsPage.info.History
		if p == nil {
			return []string{"Server history limit: unavailable", "Vendor stream limit: unavailable"}[index-5]
		}
		if index == 6 {
			// What the provider actually sent, kept verbatim. It is most of a
			// journal's bytes, so it is bounded apart from the conversation it
			// came with.
			if raw := p.RawLimit(); raw > 0 {
				return fmt.Sprintf("Vendor stream limit: %d MiB/session", raw/historypolicy.MiB)
			}
			return "Vendor stream limit: unlimited"
		}
		n, _, _ := p.Limits()
		if n == 0 {
			return "Server history limit: unlimited"
		}
		return fmt.Sprintf("Server history limit: %d MiB/session", n/historypolicy.MiB)
	}
	b, t := m.windowPreference().Limits()
	if index == 7 {
		return fmt.Sprintf("Client scroll window: %d MiB", b/historypolicy.MiB)
	}
	return fmt.Sprintf("Client scroll window: %d turns", t)
}
func nextHistoryChoice(v int, values []int) int {
	for i, n := range values {
		if n == v {
			return values[(i+1)%len(values)]
		}
	}
	return values[0]
}
func (m *model) changeHistoryWindow(index int) tea.Cmd {
	c, ok := versionpin.ClientFrom(m.ctx)
	if !ok {
		return nil
	}
	p := m.windowPreference()
	b, t := p.Limits()
	if index == 7 {
		p.MiB = nextHistoryChoice(b/historypolicy.MiB, []int{5, 10, 20, 50})
	} else {
		p.Turns = nextHistoryChoice(t, []int{100, 200, 500, 1000})
	}
	lock, err := core.Lock(filepath.Join(c.Root, "settings.lock"))
	if err == nil {
		defer lock.Close()
		cfg, e := settings.Load(c.Root)
		err = e
		if err == nil {
			cfg.History = p
			err = settings.Save(c.Root, cfg)
		}
	}
	if err != nil {
		m.settingsPage.message = err.Error()
		return nil
	}
	m.windowPolicy = &p
	for id := range m.events {
		m.limitHistory(id, m.historyWindow(id).detached, m.readerAnchor(id))
	}
	m.render()
	m.settingsPage.message = "Client history window saved. Provider context is unchanged."
	return nil
}
