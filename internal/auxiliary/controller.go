package auxiliary

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/core"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

type Runner func(context.Context, Input) (Output, error)
type Controller struct {
	wg      sync.WaitGroup
	closed  bool
	started int64
	mu      sync.Mutex
	root    string
	run     Runner
	config  Config
	active  map[string]context.CancelFunc
	slots   chan struct{}
}

func New(root string, run Runner) (*Controller, error) {
	c := &Controller{root: filepath.Join(root, "auxiliary"), started: time.Now().UnixMilli(), run: run, active: map[string]context.CancelFunc{}, slots: make(chan struct{}, 4)}
	if err := os.MkdirAll(c.root, 0700); err != nil {
		return nil, err
	}
	b, e := os.ReadFile(filepath.Join(c.root, "config.json"))
	if e == nil {
		e = json.Unmarshal(b, &c.config)
	}
	if e != nil && !os.IsNotExist(e) {
		return nil, e
	}
	return c, nil
}
func (c *Controller) Config() Config { c.mu.Lock(); defer c.mu.Unlock(); return c.config }
func (c *Controller) Save(task string, p Profile) (Config, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e := p.Validate(); e != nil {
		return c.config, e
	}
	n := c.config
	switch task {
	case "summary":
		n.Summary = p
	case "suggestion":
		n.Suggestion = p
	default:
		return n, fmt.Errorf("unknown auxiliary task")
	}
	n.Revision = core.ID()
	n.Since = time.Now().UnixMilli()
	if e := core.WriteJSON(filepath.Join(c.root, "config.json"), n); e != nil {
		return c.config, e
	}
	c.config = n
	for _, cancel := range c.active {
		cancel()
	}
	return n, nil
}
func (c *Controller) path(id string) string {
	return filepath.Join(c.root, fmt.Sprintf("%x.json", sha256.Sum256([]byte(id))))
}
func (c *Controller) load(id string) (State, error) {
	var s State
	b, e := os.ReadFile(c.path(id))
	if os.IsNotExist(e) {
		return s, nil
	}
	if e != nil {
		return s, e
	}
	e = json.Unmarshal(b, &s)
	return s, e
}
func (c *Controller) save(id string, s State) error {
	if e := core.WriteJSON(c.path(id), s); e != nil {
		return e
	}
	// Bound derived storage independently of conversation retention.
	activeFiles := map[string]bool{}
	for id := range c.active {
		activeFiles[filepath.Base(c.path(id))] = true
	}
	entries, _ := os.ReadDir(c.root)
	type file struct {
		name string
		t    time.Time
	}
	var files []file
	for _, v := range entries {
		if v.Name() == "config.json" || activeFiles[v.Name()] || v.IsDir() {
			continue
		}
		i, e := v.Info()
		if e == nil {
			files = append(files, file{v.Name(), i.ModTime()})
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].t.Before(files[j].t) })
	for len(files) > 256-len(activeFiles) {
		_ = os.Remove(filepath.Join(c.root, files[0].name))
		files = files[1:]
	}
	return nil
}
func (c *Controller) Status(id string) (*Job, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	s, e := c.load(id)
	if e != nil {
		return nil, e
	}
	if s.Job != nil && (s.Job.Status == "running" || s.Job.Status == "queued") && c.active[id] == nil {
		s.Job.Status = "interrupted"
		s.Job.Error = "Manager restarted; generation was not retried"
	}
	if s.Job != nil && s.Job.Revision != c.config.Revision {
		s.Job.Status = "stale"
		s.Job.Suggestion = ""
	}
	return s.Job, nil
}
func (c *Controller) Cancel(id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if f := c.active[id]; f != nil {
		f()
	}
	s, e := c.load(id)
	if e != nil {
		return e
	}
	if s.Job != nil {
		s.Job.Status = "canceled"
		s.Job.Suggestion = ""
	}
	return c.save(id, s)
}

// Observe accepts committed live events only. Historical page reads never schedule jobs.
func (c *Controller) Observe(events []*api.Event) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || !c.config.Configured() {
		return
	}
	for _, e := range events {
		if e.TimeMs < max(c.config.Since, c.started) {
			continue
		}
		if e.Kind != "input" && e.Kind != "assistant" && e.Kind != "tool_call" && e.Kind != "turn_end" {
			continue
		}
		s, err := c.load(e.SessionId)
		if err != nil || s.Deleted || e.Seq <= s.Seen {
			continue
		}
		cfg, err := c.sessionConfig(e.SessionId)
		if err != nil {
			continue
		}
		s.Seen = e.Seq
		switch e.Kind {
		case "input":
			if f := c.active[e.SessionId]; f != nil {
				f()
			}
			if s.Job != nil {
				s.Job.Status = "stale"
				s.Job.Suggestion = ""
			}
			s.Current = Turn{Run: e.RunId, Seq: e.Seq, User: Clip(e.Text, 8<<10)}
		case "tool_call":
			s.Current.Answer = "" // discard commentary preceding a tool
		case "assistant":
			s.Current.Answer = Clip(s.Current.Answer+"\n"+e.Text, 8<<10)
		case "turn_end":
			if e.Text == "completed" && s.Current.User != "" && s.Current.Answer != "" && s.Current.Run == e.RunId {
				s.Current.Seq = e.Seq
				s.Recent = append(s.Recent, s.Current)
				for len(s.Recent) > 1 {
					raw, _ := json.Marshal(s.Recent)
					if len(raw) <= 48<<10 && len(s.Recent) <= 64 {
						break
					}
					s.Recent = s.Recent[1:]
					s.Gap = true
				}
				s.Current = Turn{}
				if cfg.Active() && e.TimeMs >= cfg.Since {
					if err := c.start(e.SessionId, &s, cfg); err != nil {
						s.Job = &Job{Run: e.RunId, Turn: e.Seq, Revision: cfg.Revision, Status: "failed", Error: err.Error(), SummaryRequested: cfg.Summary.Enabled, SuggestionRequested: cfg.Suggestion.Enabled}
					}
				}
			}
		}
		_ = c.save(e.SessionId, s)
	}
}
func (c *Controller) execute(ctx context.Context, cancel context.CancelFunc, id string, s State, cfg Config) {
	defer cancel()
	var err error
	var usage []Usage
	defer func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		current, e := c.load(id)
		if e != nil || current.Job == nil || current.Job.ID != s.Job.ID {
			return
		}
		delete(c.active, id)
		if current.Job.Status == "canceled" || current.Job.Status == "stale" {
			return
		}
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		if cfg.Revision != c.config.Revision {
			s.Job.Status = "stale"
			s.Job.Suggestion = ""
		} else if err != nil {
			s.Job.Status = "failed"
			s.Job.Error = err.Error()
			s.Job.Suggestion = ""
		} else {
			s.Job.Status = "completed"
		}
		s.Job.Usage = usage
		current.Job = s.Job
		if cfg.Revision == c.config.Revision {
			rememberSummary(&current, s.Job)
		}
		if err == nil {
			current.Checkpoint = s.Checkpoint
			current.Through = s.Through
			current.Recent = s.Recent
		}
		pending := current.PendingTask
		current.PendingTask = ""
		_ = c.save(id, current)
		if pending != "" && ctx.Err() == nil && cfg.Revision == c.config.Revision && !c.closed {
			next := c.config
			next.Summary.Enabled = pending == "summary"
			next.Suggestion.Enabled = pending == "suggestion"
			if e := c.start(id, &current, next); e != nil {
				current.Job.Status = "failed"
				current.Job.Error = e.Error()
				_ = c.save(id, current)
			}
		}
	}()
	select {
	case c.slots <- struct{}{}:
		defer func() { <-c.slots }()
	case <-ctx.Done():
		err = ctx.Err()
		return
	}
	c.mu.Lock()
	current, e := c.load(id)
	ready := e == nil && current.Job != nil && current.Job.ID == s.Job.ID && current.Job.Status == "queued" && ctx.Err() == nil
	if ready {
		current.Job.Status = "running"
		err = c.save(id, current)
	}
	c.mu.Unlock()
	if !ready || err != nil {
		return
	}

	call := func(p Profile, task, text string) (Output, error) {
		if err := ctx.Err(); err != nil {
			return Output{}, err
		}
		if len(text) > MaxInput {
			return Output{}, fmt.Errorf("auxiliary input exceeds budget")
		}
		o, e := c.run(ctx, Input{p, task, text})
		if e == nil && (task == "summary" || task == "combined") && o.Summary == "" {
			e = fmt.Errorf("provider returned an empty summary")
		}
		if len(o.Usage) > 0 {
			usage = append(usage, Usage{p.Account, p.Model, task, o.Usage})
		}
		return o, e
	}
	b, _ := json.Marshal(s.Recent)
	if (len(b) > RecentLimit || len(s.Recent) > 32) && len(s.Recent) > 1 {
		p := cfg.Summary
		if !p.Enabled {
			p = cfg.Suggestion
		}
		older := s.Recent[:len(s.Recent)-1]
		b, _ = json.Marshal(older)
		o, e := call(p, "checkpoint", Clip("Previous checkpoint:\n"+s.Checkpoint+"\nOlder turns:\n"+string(b), MaxInput))
		if e != nil {
			err = e
			return
		}
		if o.Checkpoint == "" || len(o.Checkpoint) > CheckpointLimit {
			err = fmt.Errorf("invalid checkpoint output")
			return
		}
		s.Checkpoint = o.Checkpoint
		s.Through = older[len(older)-1].Seq
		s.Recent = s.Recent[len(s.Recent)-1:]
	}
	b, _ = json.Marshal(s.Recent)
	gap := ""
	if s.Gap {
		gap = "Some earlier turns are unavailable due to the context storage bound. Do not infer missing facts.\n"
	}
	text := gap + fmt.Sprintf("Checkpoint through turn %d:\n%s\nRecent completed turns:\n%s", s.Through, s.Checkpoint, b)
	if cfg.Summary.Enabled && cfg.Suggestion.Enabled && cfg.Summary == cfg.Suggestion {
		o, e := call(cfg.Summary, "combined", text)
		err = e
		s.Job.Summary = o.Summary
		s.Job.Suggestion = o.Suggestion
		return
	}
	if cfg.Summary.Enabled {
		o, e := call(cfg.Summary, "summary", text)
		if e != nil {
			err = e
			return
		}
		s.Job.Summary = o.Summary
		// Publish the summary immediately, even while a separate suggestion runs.
		c.mu.Lock()
		current, e := c.load(id)
		if e == nil && current.Job != nil && current.Job.ID == s.Job.ID && current.Job.Status == "running" {
			current.Job.Summary = o.Summary
			rememberSummary(&current, current.Job)
			_ = c.save(id, current)
		}
		c.mu.Unlock()
	}
	if cfg.Suggestion.Enabled {
		if s.Job.Summary != "" {
			context := "\nSummary of this turn (generated, not authoritative):\n" + Clip(s.Job.Summary, 4<<10)
			text = Clip(text, MaxInput-len(context)) + context
		}
		o, e := call(cfg.Suggestion, "suggestion", text)
		err = e
		s.Job.Suggestion = o.Suggestion
	}
}

func (c *Controller) Forget(id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if f := c.active[id]; f != nil {
		f()
	}
	delete(c.active, id)
	_ = os.Remove(c.preferencePath(id))
	return c.save(id, State{Deleted: true})
}
func (c *Controller) Close() {
	c.mu.Lock()
	c.closed = true
	for _, f := range c.active {
		f()
	}
	c.mu.Unlock()
	c.wg.Wait()
}
func (c *Controller) Cursor(id string) uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	s, _ := c.load(id)
	return s.Seen
}
