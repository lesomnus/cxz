package tui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/agentview"
)

type backgroundHistory struct {
	id     string
	epoch  uint64
	events []*api.Event
	err    error
}

// Backfill telemetry only: old transcript rows remain lazily loaded.
func (m *model) loadBackgroundHistory(p historyPage) tea.Cmd {
	if !p.initial || p.start == 0 || p.err != nil || (p.epoch != 0 && p.epoch != m.watchEpoch) {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		out := backgroundHistory{id: p.id, epoch: p.epoch}
		for end := p.start; end > 0; {
			start := uint64(0)
			if end > historyPageSize {
				start = end - historyPageSize
			}
			batch, err := m.client.History(ctx, &api.WatchRequest{SessionId: p.id, AfterSeq: start})
			if err != nil {
				out.err = err
				return out
			}
			if batch == nil || len(batch.Events) == 0 {
				out.err = fmt.Errorf("journal page unavailable")
				return out
			}
			for _, e := range batch.Events {
				if e.Seq > start && e.Seq <= end && e.Kind == "background" {
					out.events = append(out.events, e)
				}
			}
			end = start
		}
		return out
	}
}

func (m *model) backgroundStates() map[string]*agentview.BackgroundState {
	out := map[string]*agentview.BackgroundState{}
	s := m.current()
	if s == nil {
		return out
	}
	events := append([]*api.Event{}, m.backgroundHistory[s.Id]...)
	for _, e := range m.events[s.Id] {
		if e.Kind == "background" {
			events = append(events, e)
		}
	}
	sort.SliceStable(events, func(i, j int) bool { return events[i].Seq < events[j].Seq })
	seen := map[uint64]bool{}
	for _, e := range events {
		if e.Seq != 0 && seen[e.Seq] {
			continue
		}
		seen[e.Seq] = true
		if out[e.RunId] == nil {
			out[e.RunId] = &agentview.BackgroundState{}
		}
		out[e.RunId].Apply(e.Payload)
	}
	return out
}

func (m *model) backgroundStatus() string {
	s := m.current()
	if s == nil {
		return ""
	}
	state := m.backgroundStates()[s.RunId]
	if state == nil {
		if m.backgroundLoading[s.Id] {
			return muted.Render("background · loading history · /background")
		}
		return ""
	}
	n := 0
	for _, t := range state.Tasks {
		if t.Active {
			n++
		}
	}
	if n == 0 {
		return ""
	}
	frames := []rune("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏")
	return accent.Render(fmt.Sprintf("%c background %d · /background", frames[m.pulse%len(frames)], n))
}

func (m *model) backgroundReport() string {
	s := m.current()
	if s == nil {
		return "No session."
	}
	state := m.backgroundStates()[s.RunId]
	lines := []string{"Provider-reported background tasks · current run", ""}
	if state == nil || len(state.Tasks) == 0 {
		lines = append(lines, "No background task telemetry recorded for this run.")
	} else {
		ids := make([]string, 0, len(state.Tasks))
		for id := range state.Tasks {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			t := state.Tasks[id]
			status := t.Status
			if !t.Active && (status == "working" || status == "running") {
				status = "awaiting final status"
			}
			lines = append(lines, fmt.Sprintf("%s · %s · %s", id, status, t.Description))
			if t.Summary != "" {
				lines = append(lines, "  "+t.Summary)
			}
			if t.OutputFile != "" {
				lines = append(lines, "  Output: "+t.OutputFile)
			}
		}
	}
	if m.backgroundLoading[s.Id] {
		lines = append(lines, "", "Loading older task telemetry…")
	}
	if err := m.backgroundErrors[s.Id]; err != "" {
		lines = append(lines, "", "Older telemetry incomplete: "+err)
	}
	return strings.Join(append(lines, "", "Output paths are metadata only; files are not fetched. Raw events remain in the journal."), "\n")
}
