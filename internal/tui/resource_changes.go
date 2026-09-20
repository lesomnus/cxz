package tui

import (
	"context"
	tea "github.com/charmbracelet/bubbletea"
	"sort"
	"strings"
	"time"
)

type resourceChangeSource interface {
	WatchChanges(context.Context, []string, []string, func()) error
}
type resourcesChanged struct{ generation uint64 }
type resourcesRefreshDue struct{}
type resourcesWatchEnded struct {
	err        error
	generation uint64
}
type resourcesWatchRetry struct{}

func (m *model) watchResources() tea.Cmd {
	source, ok := m.client.(resourceChangeSource)
	if !ok || m.program == nil {
		return nil
	}
	projects, sessions := []string{}, []string{}
	seen := map[string]bool{}
	all := m.allSessions
	if len(all) == 0 {
		all = m.sessions
	}
	for _, s := range all {
		sessions = append(sessions, s.Id)
		if s.ProjectId != "" && !seen[s.ProjectId] {
			projects = append(projects, s.ProjectId)
			seen[s.ProjectId] = true
		}
	}
	for _, p := range m.panelProjects {
		if p.Id != "" && !seen[p.Id] {
			projects = append(projects, p.Id)
			seen[p.Id] = true
		}
	}
	if m.project != nil && m.project.Id != "" && !seen[m.project.Id] {
		projects = append(projects, m.project.Id)
	}
	sort.Strings(projects)
	sort.Strings(sessions)
	signature := strings.Join(projects, ",") + ":" + strings.Join(sessions, ",")
	if m.resourcesWatching && signature == m.resourceWatchSignature {
		return nil
	}
	if m.resourceWatchCancel != nil {
		m.resourceWatchCancel()
	}
	m.resourceWatchGeneration++
	generation := m.resourceWatchGeneration
	m.resourceWatchSignature = signature
	empty, _ := m.client.(interface{ WatchEmpty() bool })
	if len(projects)+len(sessions) == 0 && (empty == nil || !empty.WatchEmpty()) {
		m.resourcesWatching = false
		return nil
	}
	m.resourcesWatching = true
	m.resourceWatchStarted = time.Now()
	ctx, cancel := context.WithCancel(m.ctx)
	m.resourceWatchCancel = cancel
	program := m.program
	return func() tea.Msg {
		return resourcesWatchEnded{generation: generation, err: source.WatchChanges(ctx, projects, sessions, func() { program.Send(resourcesChanged{generation: generation}) })}
	}
}
func (m *model) periodicRefresh() tea.Cmd {
	if time.Since(m.lastResourceRefresh) < 30*time.Second {
		return nil
	}
	return m.refresh()
}
