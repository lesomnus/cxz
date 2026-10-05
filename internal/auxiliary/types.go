// Package auxiliary owns bounded text-only AI tasks, independently of agent sessions.
package auxiliary

import (
	"encoding/json"
	"fmt"
	"github.com/lesomnus/cxz/internal/accounts"
	"github.com/lesomnus/cxz/internal/agentview"
	"unicode/utf8"
)

type Profile struct {
	Enabled bool   `json:"enabled"`
	Account string `json:"account"`
	Agent   string `json:"agent"`
	Backend string `json:"backend"`
	Model   string `json:"model"`
	Effort  string `json:"effort,omitempty"`
}
type Config struct {
	Title      Profile `json:"title"`
	TitleSince int64   `json:"title_since"`
	Since      int64   `json:"since"`
	Revision   string  `json:"revision"`
	Summary    Profile `json:"summary"`
	Suggestion Profile `json:"suggestion"`
}

func (c Config) Configured() bool {
	return c.Summary.Account != "" || c.Suggestion.Account != "" || c.Title.Account != ""
}
func (c Config) Active() bool { return c.Summary.Enabled || c.Suggestion.Enabled }
func (p Profile) Validate() error {
	if !p.Enabled && p.Account == "" {
		return nil
	}
	if err := accounts.Validate(p.Account, p.Agent); err != nil {
		return err
	}
	if _, err := accounts.Resolve(p.Agent, p.Backend); err != nil {
		return err
	}
	if p.Model == "" || len(p.Model) > 150 || len(p.Effort) > 30 {
		return fmt.Errorf("select a model and supported effort")
	}
	return nil
}
func ValidateModel(p Profile, models []agentview.ModelOption) error {
	m, ok := agentview.SelectedModel(models, p.Model, "")
	if !ok {
		return fmt.Errorf("model %q is unavailable for this account", p.Model)
	}
	if p.Effort != "" {
		for _, e := range m.Efforts {
			if e == p.Effort {
				return nil
			}
		}
		return fmt.Errorf("effort %q is not supported by %s", p.Effort, p.Model)
	}
	return nil
}

type Request struct {
	Text    string  `json:"text,omitempty"`
	Action  string  `json:"action"`
	Enabled *bool   `json:"enabled,omitempty"`
	Task    string  `json:"task,omitempty"`
	Profile Profile `json:"profile"`
	Session string  `json:"session,omitempty"`
}
type Reply struct {
	Title         *TitleState             `json:"title,omitempty"`
	NeedsLogin    bool                    `json:"needs_login,omitempty"`
	Summaries     []Summary               `json:"summaries,omitempty"`
	SessionConfig *SessionConfig          `json:"session_config,omitempty"`
	Owner         string                  `json:"owner,omitempty"`
	Profile       *Profile                `json:"profile,omitempty"`
	Config        Config                  `json:"config"`
	Job           *Job                    `json:"job,omitempty"`
	Models        []agentview.ModelOption `json:"models,omitempty"`
	Message       string                  `json:"message,omitempty"`
}
type Turn struct {
	Run    string `json:"run"`
	Seq    uint64 `json:"seq"`
	User   string `json:"user"`
	Answer string `json:"answer"`
}
type Summary struct {
	Run  string `json:"run"`
	Turn uint64 `json:"turn"`
	Text string `json:"text"`
}

type State struct {
	PendingTask string    `json:"pending_task,omitempty"`
	Summaries   []Summary `json:"summaries,omitempty"`
	Deleted     bool      `json:"deleted,omitempty"`
	Gap         bool      `json:"gap,omitempty"`
	Seen        uint64    `json:"seen"`
	Current     Turn      `json:"current"`
	Checkpoint  string    `json:"checkpoint,omitempty"`
	Through     uint64    `json:"through"`
	Recent      []Turn    `json:"recent"`
	Job         *Job      `json:"job,omitempty"`
}
type SessionConfig struct {
	Summary    bool `json:"summary"`
	Suggestion bool `json:"suggestion"`
}

type Job struct {
	SummaryRequested    bool    `json:"summary_requested,omitempty"`
	SuggestionRequested bool    `json:"suggestion_requested,omitempty"`
	ID                  string  `json:"id"`
	Session             string  `json:"session"`
	Run                 string  `json:"run"`
	Turn                uint64  `json:"turn"`
	Revision            string  `json:"revision"`
	Status              string  `json:"status"`
	Summary             string  `json:"summary,omitempty"`
	Suggestion          string  `json:"suggestion,omitempty"`
	Error               string  `json:"error,omitempty"`
	Usage               []Usage `json:"usage,omitempty"`
}
type Usage struct {
	Account string          `json:"account"`
	Model   string          `json:"model"`
	Task    string          `json:"task"`
	Data    json.RawMessage `json:"data,omitempty"`
}
type Output struct {
	Title      string                  `json:"title"`
	NeedsLogin bool                    `json:"needs_login,omitempty"`
	Summary    string                  `json:"summary"`
	Suggestion string                  `json:"suggestion"`
	Checkpoint string                  `json:"checkpoint"`
	Usage      json.RawMessage         `json:"usage,omitempty"`
	Models     []agentview.ModelOption `json:"models,omitempty"`
}
type Input struct {
	Profile Profile `json:"profile"`
	Task    string  `json:"task"`
	Text    string  `json:"text"`
}

const MaxInput = 32 << 10
const RecentLimit = 20 << 10

// MaxOutput bounds one structured reply. A reply this large is a malfunction
// rather than a long answer, so the bound is deliberately far above anything the
// prompt asks for: overshooting a budget by a line must never cost the reply.
// It is the one limit that has to refuse rather than clip, because the reply is
// a single JSON object and half of one cannot be read.
const MaxOutput = 1 << 20

// Hard limits, in UTF-8 bytes. Summary and suggestion are clipped and marked at
// theirs, so what arrived is still shown. The checkpoint is refused at its own,
// because it is fed back as context for every later turn instead of displayed,
// and the retained one bounds the copy of a summary kept in session history for
// a later suggestion to read. The prompt is told to stay well inside all of
// them; see instructions.go.
const CheckpointLimit = 6 << 10
const SummaryLimit = MaxOutput
const SuggestionLimit = MaxOutput
const RetainedSummaryLimit = 4 << 10

func Clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	s = s[:max(0, n-32)]
	for !utf8.ValidString(s) && len(s) > 0 {
		s = s[:len(s)-1]
	}
	return s + "\n[truncated]"
}
