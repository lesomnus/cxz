package auxiliary

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/core"
)

// Preferences are user settings, separate from the prunable derived context.
type sessionPreferences struct {
	Since      int64 `json:"since,omitempty"`
	Summary    *bool `json:"summary,omitempty"`
	Suggestion *bool `json:"suggestion,omitempty"`
}

func (c *Controller) preferencePath(id string) string {
	return filepath.Join(c.root, "sessions", filepath.Base(c.path(id)))
}
func (c *Controller) preferences(id string) (sessionPreferences, error) {
	var p sessionPreferences
	b, err := os.ReadFile(c.preferencePath(id))
	if os.IsNotExist(err) {
		return p, nil
	}
	if err == nil {
		err = json.Unmarshal(b, &p)
	}
	return p, err
}
func (c *Controller) sessionConfig(id string) (Config, error) {
	p, err := c.preferences(id)
	cfg := c.config
	cfg.Since = max(cfg.Since, p.Since)
	if p.Summary != nil {
		cfg.Summary.Enabled = *p.Summary
	}
	if p.Suggestion != nil {
		cfg.Suggestion.Enabled = *p.Suggestion
	}
	return cfg, err
}
func (c *Controller) SessionConfig(id string) (SessionConfig, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	cfg, err := c.sessionConfig(id)
	return SessionConfig{cfg.Summary.Enabled, cfg.Suggestion.Enabled}, err
}
func (c *Controller) SetSession(id, task string, enabled bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if id == "" {
		return fmt.Errorf("select a session")
	}
	p, err := c.preferences(id)
	if err != nil {
		return err
	}
	profile := c.config.Summary
	switch task {
	case "summary":
		p.Summary = &enabled
	case "suggestion":
		p.Suggestion = &enabled
		profile = c.config.Suggestion
	default:
		return fmt.Errorf("unknown auxiliary task")
	}
	if enabled {
		profile.Enabled = true
		if err := profile.Validate(); err != nil {
			return fmt.Errorf("configure %s in Settings → AI tasks first: %w", task, err)
		}
	}
	p.Since = time.Now().UnixMilli()
	if err := os.MkdirAll(filepath.Dir(c.preferencePath(id)), 0700); err != nil {
		return err
	}
	if err := core.WriteJSON(c.preferencePath(id), p); err != nil {
		return err
	}
	// Do not leave a disabled task running. Completed results remain readable.
	s, err := c.load(id)
	if err != nil {
		return err
	}
	if !enabled && s.Job != nil {
		wanted := task == "summary" && s.Job.SummaryRequested || task == "suggestion" && s.Job.SuggestionRequested
		if wanted && c.active[id] != nil {
			c.active[id]()
			s.Job.Status = "canceled"
		}
		if task == "suggestion" {
			s.Job.Suggestion = ""
			s.Job.SuggestionRequested = false
		}
		return c.save(id, s)
	}
	return nil
}

// Generate is explicit opt-in to use the last completed turn from bounded server
// history. The caller verifies that the source session is idle. It never schedules
// historical turns individually, and automatic tasks make a bare command a no-op.
func (c *Controller) Generate(id, task string, events []*api.Event) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return fmt.Errorf("Manager is closing")
	}
	cfg, err := c.sessionConfig(id)
	if err != nil {
		return err
	}
	switch task {
	case "summary":
		if cfg.Summary.Enabled {
			return nil
		}
		cfg.Summary.Enabled = true
		cfg.Suggestion.Enabled = false
	case "suggestion":
		if cfg.Suggestion.Enabled {
			return nil
		}
		cfg.Suggestion.Enabled = true
		cfg.Summary.Enabled = false
	default:
		return fmt.Errorf("unknown auxiliary task")
	}
	p := cfg.Summary
	if task == "suggestion" {
		p = cfg.Suggestion
	}
	if err := p.Validate(); err != nil {
		return fmt.Errorf("configure %s in Settings → AI tasks first: %w", task, err)
	}
	s, err := c.load(id)
	if err != nil {
		return err
	}
	if s.Deleted {
		return fmt.Errorf("session was deleted")
	}

	var turn, last Turn
	var recent []Turn
	for _, e := range events {
		if e.SessionId != id {
			continue
		}
		switch e.Kind {
		case "input":
			turn = Turn{Run: e.RunId, User: Clip(e.Text, 8<<10)}
			last = Turn{}
		case "tool_call":
			turn.Answer = ""
		case "assistant":
			if e.RunId == turn.Run {
				turn.Answer = Clip(turn.Answer+"\n"+e.Text, 8<<10)
			}
		case "turn_end":
			if e.Text == "completed" && turn.Run == e.RunId && turn.User != "" && turn.Answer != "" {
				turn.Seq = e.Seq
				last = turn
				recent = append(recent, turn)
				for len(recent) > 1 {
					b, _ := json.Marshal(recent)
					if len(b) <= 48<<10 && len(recent) <= 64 {
						break
					}
					recent = recent[1:]
				}
			}
		}
	}
	if last.Seq == 0 {
		return fmt.Errorf("no complete final response in recent retained history")
	}
	if s.Seen > last.Seq && s.Current.User != "" || len(s.Recent) > 0 && s.Recent[len(s.Recent)-1].Seq > last.Seq {
		return fmt.Errorf("a newer turn has started")
	}
	if c.active[id] != nil && s.Job != nil && s.Job.Turn == last.Seq && s.Job.Run == last.Run && (s.Job.Status == "running" || s.Job.Status == "queued") {
		if task == "summary" && s.Job.SummaryRequested || task == "suggestion" && s.Job.SuggestionRequested {
			return nil
		}
		// Reuse the active source and run the other requested task immediately
		// afterward, without canceling/repeating the provider call in flight.
		s.PendingTask = task
		if task == "summary" {
			s.Job.SummaryRequested = true
		} else {
			s.Job.SuggestionRequested = true
		}
		return c.save(id, s)
	}
	if len(s.Recent) == 0 || s.Recent[len(s.Recent)-1].Seq != last.Seq {
		// The bounded history window may have skipped turns: don't attach an old
		// checkpoint as though it directly preceded this response.
		s.Recent = recent
		s.Checkpoint = ""
		s.Through = 0
		s.Gap = true
	}
	s.Seen = max(s.Seen, last.Seq)
	s.Current = Turn{}
	return c.start(id, &s, cfg)
}

// start requires c.mu. Save the queued source identity before launching work.
func (c *Controller) start(id string, s *State, cfg Config) error {
	if len(c.active) >= 32 && c.active[id] == nil {
		return fmt.Errorf("Auxiliary queue is full")
	}
	last := s.Recent[len(s.Recent)-1]
	j := &Job{ID: core.ID(), Session: id, Run: last.Run, Turn: last.Seq, Revision: cfg.Revision, Status: "queued", SummaryRequested: cfg.Summary.Enabled, SuggestionRequested: cfg.Suggestion.Enabled}
	if old := s.Job; old != nil && old.Turn == j.Turn && old.Run == j.Run && old.Revision == j.Revision {
		// A separate one-shot suggestion can use the existing same-turn summary.
		if !cfg.Summary.Enabled {
			j.Summary = old.Summary
		}
		if !cfg.Suggestion.Enabled {
			j.Suggestion = old.Suggestion
		}
	}
	s.Job = j
	s.PendingTask = ""
	if err := c.save(id, *s); err != nil {
		return err
	}
	if f := c.active[id]; f != nil {
		f()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	c.active[id] = cancel
	c.wg.Add(1)
	go func(state State) { defer c.wg.Done(); c.execute(ctx, cancel, id, state, cfg) }(*s)
	return nil
}

// Keep only a bounded set of inline summaries, independent of the agent journal.
func rememberSummary(s *State, j *Job) {
	if j.Summary == "" {
		return
	}
	v := Summary{Run: j.Run, Turn: j.Turn, Text: Clip(j.Summary, RetainedSummaryLimit)}
	for i := range s.Summaries {
		if s.Summaries[i].Run == v.Run && s.Summaries[i].Turn == v.Turn {
			s.Summaries[i] = v
			return
		}
	}
	s.Summaries = append(s.Summaries, v)
	if len(s.Summaries) > 32 {
		s.Summaries = s.Summaries[len(s.Summaries)-32:]
	}
}
func (c *Controller) Summaries(id string) ([]Summary, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	s, err := c.load(id)
	return s.Summaries, err
}
