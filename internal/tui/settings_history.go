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
	if index == 5 {
		p := m.settingsPage.info.History
		if p == nil {
			return "Server history limit: unavailable"
		}
		n, _, _ := p.Limits()
		if n == 0 {
			return "Server history limit: unlimited"
		}
		return fmt.Sprintf("Server history limit: %d MiB/session", n/historypolicy.MiB)
	}
	b, t := m.windowPreference().Limits()
	if index == 6 {
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
	if index == 6 {
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
		m.limitHistory(id, m.historyWindow(id).detached)
	}
	m.render()
	m.settingsPage.message = "Client history window saved. Provider context is unchanged."
	return nil
}
