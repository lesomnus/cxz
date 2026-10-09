package auxiliary

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/core"
)

// Title metadata is durable, separate from the prunable summary context.
type TitleState struct {
	Text      string          `json:"text,omitempty"`
	Phase     string          `json:"phase,omitempty"`
	Status    string          `json:"status,omitempty"`
	Error     string          `json:"error,omitempty"`
	Seen      uint64          `json:"seen,omitempty"`
	Completed int             `json:"completed,omitempty"`
	First     string          `json:"first,omitempty"`
	Current   Turn            `json:"current,omitempty"`
	Recent    []Turn          `json:"recent,omitempty"`
	Job       string          `json:"job,omitempty"`
	Manual    bool            `json:"manual,omitempty"`
	Usage     json.RawMessage `json:"usage,omitempty"`
}

func (c *Controller) titlePath(id string) string {
	return filepath.Join(c.root, "titles", filepath.Base(c.path(id)))
}
func (c *Controller) loadTitle(id string) (TitleState, error) {
	var s TitleState
	b, e := os.ReadFile(c.titlePath(id))
	if os.IsNotExist(e) {
		return s, nil
	}
	if e == nil {
		e = json.Unmarshal(b, &s)
	}
	return s, e
}
func (c *Controller) saveTitle(id string, s TitleState) error {
	if e := os.MkdirAll(filepath.Dir(c.titlePath(id)), 0700); e != nil {
		return e
	}
	// A title is part of what a watcher reads, so saving one is news.
	defer c.wake(id)
	return core.WriteJSON(c.titlePath(id), s)
}
func cleanTitle(s string) string {
	s = strings.Join(strings.FieldsFunc(s, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }), " ")
	s = strings.Trim(s, "\"'` ")
	r := []rune(s)
	if len(r) > 120 {
		s = string(r[:117]) + "..."
	}
	return s
}
func (c *Controller) Title(id string) (TitleState, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	s, e := c.loadTitle(id)
	if (s.Status == "queued" || s.Status == "running") && c.titleActive[id] == nil {
		s.Status = "interrupted"
	}
	return s, e
}

// Seed preserves explicitly named existing sessions and never backfills an old
// session merely because the user configured a title profile today.
func (c *Controller) SeedTitle(v *api.Session) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.config.Title.Enabled {
		return
	}
	if _, e := os.Stat(c.titlePath(v.Id)); !os.IsNotExist(e) {
		return
	}
	s := TitleState{}
	if v.Title != "" && v.Title != "Untitled" {
		s.Text = cleanTitle(v.Title)
		s.Manual = true
		s.Phase = "manual"
	}
	if v.CreatedAt < c.config.TitleSince {
		s.Phase = "final"
	}
	_ = c.saveTitle(v.Id, s)
}
func (c *Controller) SetTitle(id, text string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	text = cleanTitle(text)
	if text == "" {
		return fmt.Errorf("title must not be empty")
	}
	s, e := c.loadTitle(id)
	if e != nil {
		return e
	}
	if f := c.titleActive[id]; f != nil {
		f()
	}
	delete(c.titleActive, id)
	s.Text, s.Phase, s.Status, s.Error, s.Job, s.Manual = text, "manual", "completed", "", "", true
	return c.saveTitle(id, s)
}
func (c *Controller) observeTitles(events []*api.Event) {
	if !c.config.Title.Enabled {
		return
	}
	for _, e := range events {
		if e.TimeMs < max(c.config.TitleSince, c.started) {
			continue
		}
		s, err := c.loadTitle(e.SessionId)
		if err != nil || e.Seq <= s.Seen || s.Manual || s.Phase == "final" {
			continue
		}
		old, err := c.load(e.SessionId)
		if err != nil || old.Deleted {
			continue
		}
		s.Seen = e.Seq
		stage := ""
		switch e.Kind {
		case "input":
			if s.Current.Run == e.RunId && s.Current.User != "" {
				s.Current.User = Clip(s.Current.User+"\n"+e.Text, 2048)
			} else {
				s.Current = Turn{Run: e.RunId, User: Clip(e.Text, 2048)}
			}
			if s.First == "" {
				s.First = s.Current.User
				stage = "draft"
			}
		case "tool_call":
			s.Current.Answer = ""
		case "assistant":
			if s.Current.Run == e.RunId {
				s.Current.Answer = Clip(s.Current.Answer+"\n"+e.Text, 1024)
			}
		case "turn_end":
			if e.RunId == s.Current.Run && e.Text == "completed" && s.Current.User != "" && s.Current.Answer != "" {
				s.Completed++
				s.Current.Seq = e.Seq
				s.Recent = append(s.Recent, s.Current)
				if len(s.Recent) > 3 {
					s.Recent = s.Recent[len(s.Recent)-3:]
				}
				if s.Completed >= 3 {
					stage = "final"
				}
			}
			s.Current = Turn{}
		default:
			continue
		}
		if stage != "" {
			if err := c.startTitle(e.SessionId, &s, stage); err != nil {
				s.Error = err.Error()
				s.Status = "failed"
			}
		}
		_ = c.saveTitle(e.SessionId, s)
	}
}
func (c *Controller) GenerateTitle(id string, events []*api.Event) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	s, e := c.loadTitle(id)
	if e != nil {
		return e
	}
	var turn Turn
	var recent []Turn
	for _, v := range events {
		if v.SessionId != id {
			continue
		}
		switch v.Kind {
		case "input":
			turn = Turn{Run: v.RunId, User: Clip(v.Text, 2048)}
		case "tool_call":
			turn.Answer = ""
		case "assistant":
			if turn.Run == v.RunId {
				turn.Answer = Clip(turn.Answer+"\n"+v.Text, 1024)
			}
		case "turn_end":
			if v.Text == "completed" && v.RunId == turn.Run && turn.User != "" && turn.Answer != "" {
				recent = append(recent, turn)
				if len(recent) > 3 {
					recent = recent[1:]
				}
			}
			turn = Turn{}
		}
	}
	if len(recent) == 0 {
		return fmt.Errorf("no completed conversation in retained history")
	}
	s.Recent = recent
	// Explicit regeneration also ends automatic naming, even before turn three.
	s.Manual = true
	return c.startTitle(id, &s, "final")
}
func (c *Controller) startTitle(id string, s *TitleState, phase string) error {
	if c.closed {
		return fmt.Errorf("Manager is closing")
	}
	p := c.config.Title
	p.Enabled = true
	if e := p.Validate(); e != nil {
		return fmt.Errorf("configure Session title in Settings → AI tasks first: %w", e)
	}
	if len(c.titleActive) >= 32 && c.titleActive[id] == nil {
		return fmt.Errorf("title queue is full")
	}
	if f := c.titleActive[id]; f != nil {
		f()
	}
	s.Phase, s.Status, s.Error, s.Job = phase, "queued", "", core.ID()
	if e := c.saveTitle(id, *s); e != nil {
		return e
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	c.titleActive[id] = cancel
	cfg := c.config
	copy := *s
	copy.Recent = append([]Turn(nil), s.Recent...)
	// Reuse available turn summaries; never wait or make another AI request.
	if summaries, err := c.store.summaries(id, 0, 0); err == nil {
		for i := range copy.Recent {
			for _, summary := range summaries {
				if summary.Turn == copy.Recent[i].Seq && summary.Text != "" {
					copy.Recent[i].Answer = Clip(summary.Text, 1024)
				}
			}
		}
	}
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		defer cancel()
		var out Output
		var err error
		select {
		case c.slots <- struct{}{}:
			contextData := struct {
				Stage, ExistingTitle, InitialPurpose string
				Turns                                []Turn
			}{phase, copy.Text, copy.First, copy.Recent}
			if copy.Manual {
				contextData.Stage = "regeneration"
			}
			if phase == "draft" {
				contextData.Turns = []Turn{{User: copy.First}}
			}
			b, _ := json.Marshal(contextData)
			if len(b) > MaxInput {
				err = fmt.Errorf("title context exceeds limit")
			} else {
				out, err = c.run(ctx, Input{Profile: p, Task: "title", Text: string(b)})
			}
			<-c.slots
		case <-ctx.Done():
			err = ctx.Err()
		}
		c.mu.Lock()
		defer c.mu.Unlock()
		current, e := c.loadTitle(id)
		if e != nil || current.Job != copy.Job {
			return
		}
		delete(c.titleActive, id)
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		if cfg.Revision != c.config.Revision {
			err = fmt.Errorf("AI task settings changed")
		}
		title := cleanTitle(out.Title)
		if err == nil && title == "" {
			err = fmt.Errorf("provider returned an empty title")
		}
		if err != nil {
			current.Status = "failed"
			current.Error = err.Error()
		} else {
			current.Text = title
			current.Status = "completed"
			current.Usage = out.Usage
		}
		_ = c.saveTitle(id, current)
	}()
	return nil
}
