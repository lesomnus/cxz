package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lesomnus/cxz/api"
)

type usageLoaded struct {
	id         string
	generation uint64
	text       string
	err        error
}

func (m *model) loadUsage() tea.Cmd {
	s := m.current()
	if s == nil {
		m.notice = "Select a session first"
		return nil
	}
	id := s.Id
	provider, run := s.Agent, s.RunId
	if m.localHelp == nil {
		m.localHelp = map[string]uint64{}
	}
	if m.localOutput == nil {
		m.localOutput = map[string]string{}
	}
	if m.usageReports == nil {
		m.usageReports = map[string]string{}
	}
	if m.usageGeneration == nil {
		m.usageGeneration = map[string]uint64{}
	}
	m.localHelp[id] = m.cursor[id]
	m.localOutput[id] = "/usage"
	m.usageReports[id] = "Loading session usage…"
	m.usageGeneration[id]++
	generation := m.usageGeneration[id]
	m.resize()
	m.render()
	m.view.GotoBottom()
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 20*time.Second)
		defer cancel()
		var events []*api.Event
		var after uint64
		for {
			batch, err := m.client.History(ctx, &api.WatchRequest{SessionId: id, AfterSeq: after})
			if err != nil {
				return usageLoaded{id: id, generation: generation, err: err}
			}
			if len(batch.Events) == 0 {
				break
			}
			previous := after
			for _, e := range batch.Events {
				if e.Seq > after {
					events = append(events, e)
					after = e.Seq
				}
			}
			if after == previous {
				return usageLoaded{id: id, generation: generation, err: fmt.Errorf("history cursor did not advance")}
			}
		}
		return usageLoaded{id: id, generation: generation, text: usageReport(events) + "\n\n" + quotaHistoryReport(provider, run, events)}
	}
}

func usageReport(events []*api.Event) string {
	var turns, costTurns, durationTurns int
	var cost, duration float64
	cumulativeCost := map[string]float64{}
	sums := map[string]float64{}
	counts := map[string]int{}
	var pending metrics
	var started int64
	for _, e := range events {
		switch e.Kind {
		case "input":
			pending = nil
			started = e.TimeMs
		case "usage":
			if e.Text == "thread/tokenUsage/updated" {
				pending = fields(e.Payload).object("tokenUsage").object("last")
			}
		case "turn_end":
			turns++
			root := fields(e.Payload)
			u := root.object("usage")
			if len(u) == 0 {
				u = root.object("turn").object("usage")
			}
			if len(u) == 0 {
				u = pending
			}
			for _, f := range []struct {
				name string
				keys []string
			}{
				{"Input", []string{"input_tokens", "inputTokens"}},
				{"Output", []string{"output_tokens", "outputTokens"}},
				{"Cache read", []string{"cache_read_input_tokens", "cachedInputTokens"}},
				{"Cache write", []string{"cache_creation_input_tokens"}},
				{"Total", []string{"total_tokens", "totalTokens"}},
			} {
				if n, ok := u.number(f.keys...); ok {
					sums[f.name] += n
					counts[f.name]++
				}
			}
			if n, ok := root.number("total_cost_usd"); ok {
				previous := cumulativeCost[e.RunId]
				if n >= previous {
					cost += n - previous
				} else {
					cost += n
				}
				cumulativeCost[e.RunId] = n
				costTurns++
			} else if n, ok := root.number("costUSD", "costUsd"); ok {
				cost += n
				costTurns++
			}
			ms, ok := root.number("duration_ms", "durationMs")
			if !ok {
				ms, ok = root.object("turn").number("duration_ms", "durationMs")
			}
			if !ok && started > 0 && e.TimeMs >= started {
				ms = float64(e.TimeMs - started)
				ok = true
			}
			if ok {
				duration += ms
				durationTurns++
			}
			pending = nil
			started = 0
		}
	}
	lines := []string{"Session /usage", fmt.Sprintf("%d finished turns · journal snapshot", turns)}
	for _, name := range []string{"Input", "Output", "Cache read", "Cache write", "Total"} {
		if counts[name] > 0 {
			lines = append(lines, fmt.Sprintf("%-12s %s  (%d/%d turns reported)", name, humanCount(sums[name]), counts[name], turns))
		}
	}
	if costTurns > 0 {
		lines = append(lines, fmt.Sprintf("Cost         $%.4f  (%d/%d turns reported)", cost, costTurns, turns))
	} else {
		lines = append(lines, "Cost         not reported")
	}
	if durationTurns > 0 {
		seconds := int64(duration / 1000)
		lines = append(lines, fmt.Sprintf("Time         %02d:%02d:%02d  (%d/%d turns; provider duration or measured elapsed)", seconds/3600, seconds/60%60, seconds%60, durationTurns, turns))
	}
	lines = append(lines, "Session only; not account quota or billing. Missing metrics are not estimated.")
	return strings.Join(lines, "\n")
}
