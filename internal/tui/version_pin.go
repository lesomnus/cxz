package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/lesomnus/cxz/internal/versionpin"
	"time"
)

func (m *model) pinnedRestart() tea.Cmd {
	if time.Since(m.pinChecked) < time.Second {
		return nil
	}
	m.pinChecked = time.Now()
	c, ok := versionpin.ClientFrom(m.ctx)
	if !ok {
		return nil
	}
	p, e := versionpin.Load(c.Root)
	if e != nil || p.Version == "" || !p.Ready || p.Generation == m.pinGeneration {
		return nil
	}
	// cxz use explicitly forces the local frontend restart. No draft or secret is
	// written to disk and no conversation mutation is repeated.
	m.pinRestart = &versionpin.Restart{Executable: c.Executable}
	return tea.Quit
}

func (m *model) releaseSelectionText() string {
	c, ok := versionpin.ClientFrom(m.ctx)
	if !ok {
		return ""
	}
	p, e := versionpin.Load(c.Root)
	if e != nil {
		return "cxz release selection unavailable: " + e.Error()
	}
	if p.Version != "" && !p.Ready {
		return "cxz release switching: " + p.Selection()
	}
	if p.Pinned() {
		return "cxz release pinned: " + p.Version
	}
	channel, e := versionpin.Channel(c.Root)
	if e != nil {
		return "cxz channel unavailable: " + e.Error()
	}
	return "cxz update channel: @" + channel
}
