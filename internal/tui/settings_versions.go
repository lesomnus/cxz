package tui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lesomnus/cxz/internal/cxzupdate"
	"github.com/lesomnus/cxz/internal/releasechannel"
	"github.com/lesomnus/cxz/internal/versionpin"
)

type settingsUpstreamResult struct {
	page     *settingsPage
	versions map[string]string
}

func (p *settingsPage) upstreamVersion(channel string) string {
	if v := p.upstream[channel]; v != "" {
		return v
	}
	return "Loading…"
}

// Metadata lookup does not change update policy, stage a build or restart anything.
func (m *model) settingsUpstream() tea.Cmd {
	p := m.settingsPage
	if p == nil || p.upstreamLoading || time.Since(p.upstreamAt) < time.Minute {
		return nil
	}
	p.upstreamLoading = true
	p.upstreamAt = time.Now()
	ctx := m.ctx
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		type result struct{ channel, text string }
		results := make(chan result, 2)
		for _, channel := range []string{"edge", "stable"} {
			go func() {
				r, err := releasechannel.Resolve(ctx, channel)
				text := r.Version
				if err != nil {
					text = "Unavailable: " + err.Error()
				}
				results <- result{channel, text}
			}()
		}
		versions := map[string]string{}
		for range 2 {
			r := <-results
			versions[r.channel] = r.text
		}
		return settingsUpstreamResult{p, versions}
	}
}

func settingsBuild(version, revision string) string {
	if version == "" {
		version = "unknown"
	}
	if revision != "" {
		if len(revision) > 12 {
			revision = revision[:12]
		}
		version += " (" + revision + ")"
	}
	return version
}
func settingsRemoteVersion(p *settingsPage) string {
	label := "Connected Manager: "
	if !p.loaded {
		return label + "Unavailable"
	}
	i := p.info
	if i.CXZVersion == "" && i.CXZRevision == "" {
		return label + "Unavailable (update Manager)"
	}
	channel := "unknown"
	if i.CXZChannel != "" {
		channel = "@" + i.CXZChannel
	}
	text := label + settingsBuild(i.CXZVersion, i.CXZRevision) + " · " + channel
	if i.CXZPin != "" {
		text += " · pinned " + i.CXZPin
	}
	if i.CXZError != "" {
		text += " · " + i.CXZError
	}
	if p.statusError != "" {
		text += " (last known)"
	}
	return text
}
func (m *model) settingsClientVersion() string {
	b := cxzupdate.Current()
	text := "Client: " + settingsBuild(b.Version, b.Revision)
	c, ok := versionpin.ClientFrom(m.ctx)
	if !ok {
		return text + " · channel unavailable"
	}
	channel, err := versionpin.Channel(c.Root)
	if err != nil {
		return text + " · channel unavailable: " + err.Error()
	}
	text += " · @" + channel
	p, err := versionpin.Load(c.Root)
	if err != nil {
		return text + " · selection unavailable: " + err.Error()
	}
	if p.Pinned() {
		text += " · pinned " + p.Version
	}
	return text
}
