package tui

import (
	runtimemetrics "runtime/metrics"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type performanceTick struct {
	recording time.Time
	ready     time.Time
	metrics   map[string]uint64
}

func (r *debugRecorder) generation() time.Time {
	if r == nil {
		return time.Time{}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.active {
		return time.Time{}
	}
	return r.started
}

// Sample on a command goroutine. The elapsed time from ready to Update includes
// scheduling and event-loop delivery delay, not terminal display latency.
func (m *model) performanceCommand() tea.Cmd {
	generation := m.debugRecorder.generation()
	if generation.IsZero() {
		return nil
	}
	return tea.Tick(time.Second, func(_ time.Time) tea.Msg {
		samples := []runtimemetrics.Sample{
			{Name: "/memory/classes/heap/objects:bytes"},
			{Name: "/gc/heap/allocs:bytes"},
			{Name: "/gc/cycles/total:gc-cycles"},
			{Name: "/sched/goroutines:goroutines"},
		}
		runtimemetrics.Read(samples)
		values := map[string]uint64{}
		for _, s := range samples {
			if s.Value.Kind() == runtimemetrics.KindUint64 {
				values[s.Name] = s.Value.Uint64()
			}
		}
		return performanceTick{recording: generation, ready: time.Now(), metrics: values}
	})
}

func (m *model) performanceUpdate(t performanceTick) tea.Cmd {
	if t.recording.IsZero() || t.recording != m.debugRecorder.generation() {
		return nil
	}
	m.debugRecorder.Add(debugEvent{Kind: "performance", Wait: time.Since(t.ready).Microseconds(), Metrics: t.metrics})
	return m.performanceCommand()
}
