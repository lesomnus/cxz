package workspace

import (
	"context"
	"fmt"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/accounts"
	"github.com/lesomnus/cxz/internal/agentview"
	"github.com/lesomnus/cxz/internal/auxiliary"
)

// Aux is one task done beside a session, and these are the only ways to ask for
// one. The controller behind them is the one the manager's own observation loop
// drives, so a caller never starts a task the manager would not have started by
// itself -- it asks for one sooner, or reads what the last one produced.

// AuxKinds is every kind a caller may name.
var AuxKinds = []string{"summary", "suggestion", "title"}

// maxAuxText bounds the one text a caller supplies, which is a manual title.
const maxAuxText = 4 << 10

func auxKind(kind string) error {
	for _, k := range AuxKinds {
		if k == kind {
			return nil
		}
	}
	return fmt.Errorf("unknown aux kind %q", kind)
}

func (m *Manager) AuxConfig(ctx context.Context) (*api.AuxConfigReply, error) {
	c, err := m.auxiliaryController()
	if err != nil {
		return nil, err
	}
	return m.auxConfigReply(c.Config(), ""), nil
}

func (m *Manager) AuxSetConfig(ctx context.Context, in *api.AuxSetConfigInput) (*api.AuxConfigReply, error) {
	c, err := m.auxiliaryController()
	if err != nil {
		return nil, err
	}
	if len(in.Profiles) == 0 {
		return nil, fmt.Errorf("name a kind to configure")
	}
	before := c.Config()
	cfg := before
	message := ""
	for _, p := range in.Profiles {
		if err = auxKind(p.Kind); err != nil {
			return nil, err
		}
		profile := auxProfile(p)
		// An enabled profile is proven before it is saved: the catalog call is
		// what says the account can reach the model it names.
		if profile.Enabled {
			out, err := m.runAuxiliary(ctx, auxiliary.Input{Profile: profile, Task: "models"})
			if err != nil {
				return nil, err
			}
			if out.NeedsLogin {
				return nil, fmt.Errorf("auxiliary account needs login")
			}
			if err = auxiliary.ValidateModel(profile, out.Models); err != nil {
				return nil, err
			}
		}
		if cfg, err = c.Save(p.Kind, profile); err != nil {
			return nil, err
		}
	}
	// A brokered grant outlives the profile that needed it unless something
	// takes it back, and nothing else will.
	for _, p := range []auxiliary.Profile{before.Summary, before.Suggestion, before.Title} {
		if !p.Enabled || p.Backend != accounts.BrokeredAccessToken {
			continue
		}
		needed := false
		for _, next := range []auxiliary.Profile{cfg.Summary, cfg.Suggestion, cfg.Title} {
			needed = needed || (next.Enabled && next.Account == p.Account)
		}
		if !needed {
			if err := accounts.RevokeAuxiliaryGrant(m.Root, p.Account); err != nil {
				message = "Settings saved; auxiliary grant cleanup failed: " + err.Error()
			}
		}
	}
	return m.auxConfigReply(cfg, message), nil
}

func (m *Manager) AuxModels(ctx context.Context, in *api.AuxModelsInput) (*api.AuxModelsReply, error) {
	if in.Account == "" {
		return nil, fmt.Errorf("select a registered account")
	}
	profile := auxiliary.Profile{Account: in.Account, Agent: in.Agent, Backend: in.Backend}
	out, err := m.runAuxiliary(ctx, auxiliary.Input{Profile: profile, Task: "models"})
	if err != nil {
		return nil, err
	}
	return &api.AuxModelsReply{Models: auxModels(out.Models), NeedsLogin: out.NeedsLogin}, nil
}

// AuxLoginInfo says where a login has to happen and under which profile. It
// carries no credential, which is why it is a separate call from the stream
// that performs one.
func (m *Manager) AuxLoginInfo(ctx context.Context, in *api.AuxLoginInfoInput) (*api.AuxLoginInfoReply, error) {
	if in.Account == "" {
		return nil, fmt.Errorf("select a registered account")
	}
	if _, err := m.auxiliaryController(); err != nil {
		return nil, err
	}
	return &api.AuxLoginInfoReply{
		Owner: m.Owner, Account: in.Account, Agent: in.Agent, Backend: in.Backend,
	}, nil
}

func (m *Manager) AuxStatus(ctx context.Context, in *api.AuxStatusInput) (*api.AuxState, error) {
	return m.auxState(in.SessionId, "", in.AfterTurn, in.Limit)
}

// AuxRun asks for one task now. It does not change what runs automatically, and
// a kind that is already automatic is left alone rather than run twice.
func (m *Manager) AuxRun(ctx context.Context, in *api.AuxRunInput) (*api.AuxState, error) {
	if in.SessionId == "" {
		return nil, fmt.Errorf("select a session")
	}
	if len(in.Kinds) == 0 {
		return nil, fmt.Errorf("name a kind to run")
	}
	if len(in.Text) > maxAuxText {
		return nil, fmt.Errorf("aux text exceeds %d bytes", maxAuxText)
	}
	c, err := m.auxiliaryController()
	if err != nil {
		return nil, err
	}
	message := ""
	for _, kind := range in.Kinds {
		if err = auxKind(kind); err != nil {
			return nil, err
		}
		if kind == "title" {
			if _, err = m.Get(ctx, in.SessionId); err != nil {
				return nil, err
			}
			if in.Text != "" {
				err = c.SetTitle(in.SessionId, in.Text)
			} else {
				err = m.generateAuxiliaryTitle(ctx, c, in.SessionId)
			}
			if err != nil {
				return nil, err
			}
			message = "Session title updated or queued"
			continue
		}
		cfg, err := c.SessionConfig(in.SessionId)
		if err != nil {
			return nil, err
		}
		automatic := kind == "summary" && cfg.Summary || kind == "suggestion" && cfg.Suggestion
		if automatic {
			continue
		}
		if err = m.generateAuxiliary(ctx, c, in.SessionId, kind); err != nil {
			return nil, err
		}
	}
	return m.auxState(in.SessionId, message, 0, 0)
}

func (m *Manager) AuxPrefer(ctx context.Context, in *api.AuxPreferInput) (*api.AuxState, error) {
	if len(in.Preferences) == 0 {
		return nil, fmt.Errorf("name a kind to set")
	}
	c, err := m.auxiliaryController()
	if err != nil {
		return nil, err
	}
	message := ""
	for _, p := range in.Preferences {
		if err = auxKind(p.Kind); err != nil {
			return nil, err
		}
		if p.Kind == "title" {
			return nil, fmt.Errorf("titles are not preferred per session")
		}
		if err = c.SetSession(in.SessionId, p.Kind, p.Enabled); err != nil {
			return nil, err
		}
		message = fmt.Sprintf("%s %s · this session", p.Kind, map[bool]string{true: "on", false: "off"}[p.Enabled])
	}
	return m.auxState(in.SessionId, message, 0, 0)
}

func (m *Manager) AuxCancel(ctx context.Context, in *api.AuxCancelInput) (*api.AuxState, error) {
	c, err := m.auxiliaryController()
	if err != nil {
		return nil, err
	}
	if err = c.Cancel(in.SessionId); err != nil {
		return nil, err
	}
	return m.auxState(in.SessionId, "", 0, 0)
}

func (m *Manager) AuxForget(ctx context.Context, in *api.AuxForgetInput) (*api.Receipt, error) {
	c, err := m.auxiliaryController()
	if err != nil {
		return nil, err
	}
	return &api.Receipt{Status: "aux state forgotten"}, c.Forget(in.SessionId)
}

func (m *Manager) auxConfigReply(cfg auxiliary.Config, message string) *api.AuxConfigReply {
	return &api.AuxConfigReply{
		Revision: cfg.Revision,
		Owner:    m.Owner,
		Message:  message,
		Profiles: []*api.AuxProfile{
			auxProfileOf("summary", cfg.Summary, cfg.Since),
			auxProfileOf("suggestion", cfg.Suggestion, cfg.Since),
			auxProfileOf("title", cfg.Title, cfg.TitleSince),
		},
	}
}

func (m *Manager) auxState(session, message string, afterTurn uint64, limit int32) (*api.AuxState, error) {
	if session == "" {
		return nil, fmt.Errorf("select a session")
	}
	c, err := m.auxiliaryController()
	if err != nil {
		return nil, err
	}
	job, err := c.Status(session)
	if err != nil {
		return nil, err
	}
	cfg, err := c.SessionConfig(session)
	if err != nil {
		return nil, err
	}
	summaries, err := c.SummariesAfter(session, afterTurn, int(limit))
	if err != nil {
		return nil, err
	}
	recent, err := c.Tasks(session, int(limit))
	if err != nil {
		return nil, err
	}
	title, _ := c.Title(session)
	out := &api.AuxState{
		Current: auxOf(session, job),
		Title:   title.Text,
		Message: message,
		Preferences: []*api.AuxPreference{
			{Kind: "summary", Enabled: cfg.Summary, SinceMs: cfg.Since},
			{Kind: "suggestion", Enabled: cfg.Suggestion, SinceMs: cfg.Since},
		},
	}
	for _, s := range summaries {
		out.Summaries = append(out.Summaries, &api.AuxSummary{RunId: s.Run, Turn: s.Turn, Text: s.Text})
	}
	for i := range recent {
		out.Recent = append(out.Recent, auxOf(session, &recent[i]))
	}
	return out, nil
}

func auxProfileOf(kind string, p auxiliary.Profile, since int64) *api.AuxProfile {
	return &api.AuxProfile{
		Kind: kind, Enabled: p.Enabled, Account: p.Account, Agent: p.Agent,
		Backend: p.Backend, Model: p.Model, Effort: p.Effort, SinceMs: since,
	}
}

func auxProfile(p *api.AuxProfile) auxiliary.Profile {
	if p == nil {
		return auxiliary.Profile{}
	}
	return auxiliary.Profile{
		Enabled: p.Enabled, Account: p.Account, Agent: p.Agent,
		Backend: p.Backend, Model: p.Model, Effort: p.Effort,
	}
}

func auxModels(models []agentview.ModelOption) []*api.AuxModel {
	out := make([]*api.AuxModel, 0, len(models))
	for _, o := range models {
		out = append(out, &api.AuxModel{
			Id: o.ID, ResolvedId: o.ResolvedID, Name: o.Name,
			Efforts: o.Efforts, DefaultEffort: o.DefaultEffort, Default: o.Default,
		})
	}
	return out
}

// auxOf reports a job as the task it is. kinds is what was asked for and
// results is what came back, so a running task that already produced its
// summary says so instead of waiting for the suggestion beside it.
func auxOf(session string, j *auxiliary.Job) *api.Aux {
	if j == nil {
		return nil
	}
	out := &api.Aux{
		Id: j.ID, SessionId: j.Session, RunId: j.Run, Turn: j.Turn,
		Revision: j.Revision, State: j.Status, Message: j.Error,
	}
	if out.SessionId == "" {
		out.SessionId = session
	}
	if j.SummaryRequested {
		out.Kinds = append(out.Kinds, "summary")
	}
	if j.SuggestionRequested {
		out.Kinds = append(out.Kinds, "suggestion")
	}
	out.Results = auxResults(map[string]string{"summary": j.Summary, "suggestion": j.Suggestion})
	for _, u := range j.Usage {
		out.Usage = append(out.Usage, &api.AuxUsage{Account: u.Account, Model: u.Model, Kind: u.Task, Data: u.Data})
	}
	return out
}

func auxResults(texts map[string]string) []*api.AuxResult {
	var out []*api.AuxResult
	for _, kind := range AuxKinds {
		text := texts[kind]
		if text == "" {
			continue
		}
		out = append(out, &api.AuxResult{
			Kind: kind, Text: text,
			Truncated: strings.HasSuffix(text, "\n[truncated]"),
		})
	}
	return out
}

// AuxText finds one kind's text in a task's results, for the callers that want
// one answer rather than the set.
func AuxText(a *api.Aux, kind string) string {
	if a == nil {
		return ""
	}
	for _, r := range a.Results {
		if r.Kind == kind {
			return r.Text
		}
	}
	return ""
}

// AuxEvents sends this session's aux state now, and again whenever it changes.
// The heartbeat is a safety net rather than a poll: a watcher is woken by the
// controller in the same place it writes, and a read that finds nothing new
// sends nothing.
func (m *Manager) AuxEvents(in *api.AuxStatusInput, stream api.Sessions_AuxEventsServer) error {
	if in.SessionId == "" {
		return fmt.Errorf("select a session")
	}
	c, err := m.auxiliaryController()
	if err != nil {
		return err
	}
	wake, stop := c.Watch(in.SessionId)
	defer stop()
	ctx := stream.Context()
	ticker := time.NewTicker(auxHeartbeat)
	defer ticker.Stop()
	var last *api.AuxState
	for {
		state, err := m.auxState(in.SessionId, "", in.AfterTurn, in.Limit)
		if err != nil {
			return err
		}
		if last == nil || !proto.Equal(last, state) {
			if err = stream.Send(state); err != nil {
				return err
			}
			last = state
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-wake:
		case <-ticker.C:
		}
	}
}

// auxHeartbeat is how long a watcher waits before reading anyway. Nothing
// depends on it: it covers a wake lost to a process that wrote and died, and a
// state that did not change costs one read and sends nothing.
const auxHeartbeat = 30 * time.Second
