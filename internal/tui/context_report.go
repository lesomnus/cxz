package tui

import (
	"fmt"
	"math"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lesomnus/cxz/internal/agentview"
)

type contextReportView struct {
	snapshot agentview.ContextReport
	raw      string
	rawMode  bool
	offsets  [2]int
	timeMS   int64
	headerY  int
}

func contextBar(percent float64, width int) string {
	width = max(1, min(36, width))
	filled := int(math.Round(math.Max(0, math.Min(100, percent)) * float64(width) / 100))
	return "[" + strings.Repeat("█", filled) + strings.Repeat("░", width-filled) + "]"
}
func (c *contextReportView) summary(width int) string {
	r := c.snapshot
	value := func(n *float64) string {
		if n == nil {
			return "Not reported"
		}
		return humanCount(*n) + " tokens"
	}
	model := r.Model
	if model == "" {
		model = "Not reported"
	}
	lines := []string{"Context · " + pickerLabel(r.Provider), "Model       " + model, "Source      " + r.Basis}
	if c.timeMS > 0 {
		lines = append(lines, "Snapshot    "+time.UnixMilli(c.timeMS).Local().Format("01-02 15:04:05"))
	}
	lines = append(lines, "", "Used        "+value(r.Used), "Limit       "+value(r.Window))
	if p, ok := r.Utilization(); ok {
		free := math.Max(0, *r.Window-*r.Used)
		lines = append(lines, "Available   "+value(&free), fmt.Sprintf("Utilization %.1f%%", p), contextBar(p, width-2))
	} else {
		lines = append(lines, "Available   Not reported", "Utilization Not reported")
	}
	lines = append(lines, "", "Context composition")
	if len(r.Categories) == 0 {
		lines = append(lines, "Not reported by this provider.")
	}
	for _, m := range r.Categories {
		line := m.Name + "  " + humanCount(m.Tokens) + " tokens"
		if r.Window != nil {
			line += fmt.Sprintf("  (%.1f%%)", m.Tokens / *r.Window * 100)
		}
		lines = append(lines, line)
	}
	lines = append(lines, "", "Token metrics")
	if len(r.Metrics) == 0 {
		lines = append(lines, "Not reported by this provider.")
	}
	for _, m := range r.Metrics {
		lines = append(lines, m.Name+"  "+humanCount(m.Tokens)+" tokens")
	}
	if r.Note != "" {
		lines = append(lines, "", r.Note)
	}
	return strings.Join(lines, "\n")
}
func (p *reportOverlay) setContext(r agentview.ContextReport, raw string, at int64) {
	if p.context == nil {
		p.context = &contextReportView{headerY: -1}
	}
	p.context.snapshot, p.context.raw, p.context.timeMS = r, raw, at
	p.text = p.context.summary(40)
}
func (p *reportOverlay) contextTab(raw bool) {
	c := p.context
	if c == nil || c.rawMode == raw {
		return
	}
	old, next := 0, 0
	if c.rawMode {
		old = 1
	}
	if raw {
		next = 1
	}
	c.offsets[old] = p.offset
	c.rawMode = raw
	p.offset = c.offsets[next]
}

// Mouse coordinates are relative to the conversation pane, like other modals.
func (m *model) contextReportMouse(v tea.MouseMsg) bool {
	p := m.report
	if p == nil || p.context == nil || v.Y != p.context.headerY || v.Button != tea.MouseButtonLeft || v.Action != tea.MouseActionPress {
		return false
	}
	if v.X >= 2 && v.X < 11 {
		p.contextTab(false)
		return true
	}
	if v.X >= 12 && v.X < 17 {
		p.contextTab(true)
		return true
	}
	return false
}

func (p *reportOverlay) contextNote(note string) {
	if p.context == nil {
		p.text = note
		return
	}
	p.context.snapshot.Note = note
	p.text = p.context.summary(40)
}
