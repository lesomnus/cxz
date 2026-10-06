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
	"github.com/lesomnus/cxz/internal/core"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type modelCatalog struct {
	Models          []agentview.ModelOption
	Model, Effort   string
	EffectiveModel  string  `json:"effective_model"`
	EffectiveEffort *string `json:"effective_effort"`
	Source          string  `json:"source"`
	// AppliedReported is whether the provider confirmed the applied values. Codex
	// reports none, so what is shown for it is what cxz asked for -- worth saying
	// rather than letting a request read as a fact.
	AppliedReported bool `json:"applied_reported"`
	// seq and ms are where and when this record was published, for a picker that
	// has to say how old its answer is.
	seq uint64
	ms  int64
}

func (c *modelCatalog) selectedModel() (agentview.ModelOption, bool) {
	return agentview.SelectedModel(c.Models, c.Model, c.EffectiveModel)
}

func (c *modelCatalog) currentEffort() string {
	if c.EffectiveEffort != nil {
		return *c.EffectiveEffort
	}
	if c.Effort != "" {
		return c.Effort
	}
	if option, ok := c.selectedModel(); ok {
		return option.DefaultEffort
	}
	return ""
}

type modelPicker struct {
	id, run, kind, query, requested, message string
	epoch                                    uint64
	// lastSeq anchors the fallback scan, which reads backwards.
	lastSeq    uint64
	selected   int
	loading    bool
	refreshing bool
	catalog    *modelCatalog
	cancel     context.CancelFunc
}
type modelCatalogLoaded struct {
	epoch      uint64
	catalog    *modelCatalog
	refreshing bool
	status     string
	err        error
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
	p := &modelPicker{id: s.Id, run: s.RunId, kind: f[0], epoch: m.modelPickerEpoch, loading: true, lastSeq: max(s.LastSeq, m.cursor[s.Id])}
	if len(f) > 1 {
		p.requested = command
	}
	m.modelPicker = p
	// A record already read for this run describes the same agent, so /model
	// followed by /effort asks nothing twice.
	if v, ok := m.modelCatalogs[s.Id+"/"+s.RunId]; ok && v != nil && p.requested == "" {
		p.loading, p.catalog = false, v
		m.selectCurrentOption(p)
		return nil
	}
	return m.loadModelCatalog(p, false)
}

// loadModelCatalog asks the server for the capability record instead of
// rebuilding it here. The record is one event in a journal that can hold an
// entire conversation, and a client that reads the journal to find it pays for
// the conversation: every page, in order, over whatever link it is on.
//
// refresh additionally asks the agent. That answer is published as a journal
// event, so it arrives on the stream this client already watches -- the picker
// shows what is known meanwhile instead of blocking on a provider.
func (m *model) loadModelCatalog(p *modelPicker, refresh bool) tea.Cmd {
	client := m.client
	ctx, cancel := context.WithTimeout(m.ctx, 20*time.Second)
	p.cancel = cancel
	p.loading = true
	return func() tea.Msg {
		defer cancel()
		reply, err := client.Models(ctx, &api.ModelsRequest{SessionId: p.id, Refresh: refresh, RunId: p.run, ClientId: core.ID()})
		if status.Code(err) == codes.Unimplemented {
			return scanModelCatalog(ctx, client, p)
		}
		if err != nil {
			return modelCatalogLoaded{epoch: p.epoch, err: err}
		}
		out := modelCatalogLoaded{epoch: p.epoch, refreshing: reply.GetRefreshing(), status: reply.GetStatus()}
		if len(reply.GetData()) > 0 && reply.GetRunId() == p.run {
			var v modelCatalog
			if json.Unmarshal(reply.GetData(), &v) == nil {
				v.seq, v.ms = reply.GetCatalogSeq(), reply.GetCatalogMs()
				out.catalog = &v
			}
		}
		return out
	}
}

// scanModelCatalog is the fallback for a manager that predates the capability
// RPC. It reads backwards from the end, because the record it wants belongs to
// the current run and a run's events are the newest in the journal -- the
// forward scan this replaced read the whole history to reach them.
//
// An unknown end leaves nothing to read backwards from, so that case reads
// forward and keeps the last match.
func scanModelCatalog(ctx context.Context, client api.SessionsClient, p *modelPicker) tea.Msg {
	latest := func(batch *api.EventBatch, limit uint64) *modelCatalog {
		var out *modelCatalog
		for _, e := range batch.GetEvents() {
			if e.Kind != "models" || p.run == "" || e.RunId != p.run || (limit > 0 && e.Seq > limit) {
				continue
			}
			var v modelCatalog
			if json.Unmarshal(e.Payload, &v) == nil {
				v.seq, v.ms = e.Seq, e.TimeMs
				out = &v
			}
		}
		return out
	}
	if p.lastSeq == 0 {
		var catalog *modelCatalog
		for after := uint64(0); ; {
			batch, err := client.History(ctx, &api.WatchRequest{SessionId: p.id, AfterSeq: after})
			if err != nil {
				return modelCatalogLoaded{epoch: p.epoch, err: err}
			}
			if len(batch.GetEvents()) == 0 {
				return modelCatalogLoaded{epoch: p.epoch, catalog: catalog}
			}
			if v := latest(batch, 0); v != nil {
				catalog = v
			}
			previous := after
			for _, e := range batch.GetEvents() {
				after = max(after, e.Seq)
			}
			if after == previous {
				return modelCatalogLoaded{epoch: p.epoch, err: fmt.Errorf("history cursor did not advance")}
			}
		}
	}
	for end := p.lastSeq; end > 0; {
		after := uint64(0)
		if end > historyPageSize {
			after = end - historyPageSize
		}
		batch, err := client.History(ctx, &api.WatchRequest{SessionId: p.id, AfterSeq: after})
		if err != nil {
			return modelCatalogLoaded{epoch: p.epoch, err: err}
		}
		if v := latest(batch, end); v != nil {
			return modelCatalogLoaded{epoch: p.epoch, catalog: v}
		}
		if after == 0 {
			break
		}
		end = after
	}
	return modelCatalogLoaded{epoch: p.epoch}
}

func (m *model) acceptModelCatalog(v modelCatalogLoaded) tea.Cmd {
	p := m.modelPicker
	if p == nil || p.epoch != v.epoch {
		return nil
	}
	p.loading = false
	p.refreshing = v.refreshing
	if v.err != nil {
		p.message = "Catalog lookup failed; no command sent. " + v.err.Error()
		return nil
	}
	if v.catalog != nil {
		p.catalog = v.catalog
		m.rememberCatalog(p.id, p.run, v.catalog)
	}
	p.message = v.status
	if p.catalog == nil {
		if p.message == "" {
			p.message = "This run has no model-control capability record. No command was sent.\nPress r to ask the provider, or close with Esc and submit /restart. If the runtime is outdated, update manager and project runtime first."
		}
		return nil
	}
	if p.requested != "" {
		return m.submitModelChoice(p.requested)
	}
	m.selectCurrentOption(p)
	return nil
}

// rememberCatalog keeps one record per session: a previous run's catalog
// describes an agent that is gone, so it is replaced rather than accumulated.
func (m *model) rememberCatalog(id, run string, catalog *modelCatalog) {
	if m.modelCatalogs == nil {
		m.modelCatalogs = map[string]*modelCatalog{}
	}
	for key := range m.modelCatalogs {
		if session, _, ok := strings.Cut(key, "/"); ok && session == id && key != id+"/"+run {
			delete(m.modelCatalogs, key)
		}
	}
	m.modelCatalogs[id+"/"+run] = catalog
}

// selectCurrentOption puts the cursor on what is in effect now, so Enter without
// moving changes nothing.
func (m *model) selectCurrentOption(p *modelPicker) {
	if p.catalog == nil {
		return
	}
	current := p.catalog.Model
	if p.kind == "/effort" {
		current = p.catalog.currentEffort()
	}
	for i, option := range p.options() {
		if option == current {
			p.selected = i
			break
		}
	}
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
		if v, ok := p.catalog.selectedModel(); ok && len(v.Efforts) > 0 {
			options = append(options, v.Efforts...)
			options = append(options, "default")
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
	case "ctrl+d":
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
	case "alt+r":
		// Asking the provider is the only way to learn that it changed something
		// by itself, such as falling back to a smaller model. Not r, because every
		// printable key here types into the search, and not Ctrl+R, which resumes
		// a session everywhere else in this TUI.
		if !p.loading {
			p.message = ""
			return m.loadModelCatalog(p, true)
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

// catalogNote says where the answer came from, how old it is, and whether the
// provider confirmed the applied value. A picker that shows a record without
// saying it is a record is how a stale answer passes for the agent's state.
func (p *modelPicker) catalogNote(now time.Time) string {
	if p.catalog == nil || p.loading {
		return ""
	}
	note := p.catalog.Source
	if note == "" {
		note = "provider report"
	}
	if p.catalog.ms > 0 {
		if age := now.Sub(time.UnixMilli(p.catalog.ms)); age < time.Minute {
			note += " · just now"
		} else {
			note += " · " + minutesLabel(int(age.Minutes())) + " ago"
		}
	}
	if !p.catalog.AppliedReported {
		note += " · applied value not confirmed by the provider"
	}
	return note
}

func (m *model) modelPickerOverlay(view string) string {
	p := m.modelPicker
	if p == nil {
		return view
	}
	rows := strings.Split(view, "\n")
	width := max(1, m.width-4)
	lines := []string{accent.Bold(true).Render(p.kind + " · select"), muted.Render("↑/↓ choose · Enter apply · Alt+R ask the provider · Esc cancel")}
	if p.kind == "/effort" {
		lines[0] = accent.Bold(true).Render("/effort · reasoning level")
		if p.catalog != nil {
			current := p.catalog.currentEffort()
			if current == "" {
				current = "not reported"
			}
			if p.catalog.Effort == "" {
				current += " · model default"
			}
			lines = append(lines, "Current: "+safeText(current))
		}
	}
	if p.loading {
		lines = append(lines, "Reading provider capabilities…")
	}
	if p.refreshing {
		lines = append(lines, muted.Render("Asked the provider; this updates when it answers."))
	}
	if note := p.catalogNote(time.Now()); note != "" {
		lines = append(lines, muted.Render(safeText(note)))
	}
	if p.message != "" {
		lines = append(lines, strings.Split(ansi.Hardwrap(safeText(p.message), width, true), "\n")...)
	}
	options := p.options()
	count := min(7, max(0, len(rows)-8))
	selected := min(p.selected, max(0, len(options)-1))
	start := max(0, min(selected-max(0, count-3), len(options)-count))
	for i := start; i < min(len(options), start+count); i++ {
		label := options[i]
		if p.kind == "/effort" && label == "default" {
			label = "Model default (reset)"
		}
		line := "  " + safeText(label)
		if i == selected {
			line = focus.Render("› " + safeText(label))
		}
		lines = append(lines, line)
	}
	if !p.loading && p.catalog != nil && len(options) == 0 {
		if p.kind == "/effort" && p.query == "" {
			lines = append(lines, "No reasoning levels reported for the current model.")
		} else {
			lines = append(lines, "No matching provider choices.")
		}
	}
	caret := " "
	if m.pulse%10 < 5 {
		caret = inputCursorStyle.Render("▏")
	}
	lines = append(lines, "", "Search: "+safeText(p.query)+caret)
	return overlayBox(view, lines, m.width, true)
}
