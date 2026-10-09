package workspace

import (
	"context"
	"strings"
	"testing"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/accounts"
	"github.com/lesomnus/cxz/internal/auxiliary"
	"google.golang.org/grpc"
)

type auxiliaryHistoryClient struct {
	api.SessionsClient
	gets, pages   int
	first         uint64
	changed, busy bool
}

func (c *auxiliaryHistoryClient) Get(context.Context, *api.SessionRef, ...grpc.CallOption) (*api.Session, error) {
	c.gets++
	s := &api.Session{Id: "s", RunId: "run", LastSeq: 3000, State: "idle"}
	if c.busy || c.changed && c.gets > 1 {
		s.State = "working"
	}
	return s, nil
}
func (c *auxiliaryHistoryClient) History(_ context.Context, r *api.WatchRequest, _ ...grpc.CallOption) (*api.EventBatch, error) {
	if c.pages == 0 {
		c.first = r.AfterSeq
	}
	c.pages++
	b := &api.EventBatch{}
	for seq := r.AfterSeq + 1; seq <= min(r.AfterSeq+128, 3000); seq++ {
		e := &api.Event{SessionId: "s", RunId: "run", Seq: seq, Kind: "tool_result", Text: "omit result", Payload: []byte("omit raw data")}
		switch seq {
		case 2998:
			e.Kind = "input"
			e.Text = "question"
		case 2999:
			e.Kind = "assistant"
			e.Text = strings.Repeat("a", 20000)
		case 3000:
			e.Kind = "turn_end"
			e.Text = "completed"
		}
		b.Events = append(b.Events, e)
	}
	return b, nil
}
func TestAuxiliaryHistoryIsBoundedAndRechecksSource(t *testing.T) {
	c := &auxiliaryHistoryClient{}
	events, err := auxiliaryHistory(t.Context(), c, "s")
	if err != nil || c.pages != 16 || c.first != 952 || len(events) != 3 {
		t.Fatal(events, err, c)
	}
	for _, e := range events {
		if len(e.Text) > 8<<10 || len(e.Payload) > 0 {
			t.Fatal("unbounded/raw context")
		}
	}
	c = &auxiliaryHistoryClient{changed: true}
	if _, err = auxiliaryHistory(t.Context(), c, "s"); err == nil {
		t.Fatal("accepted changing source")
	}
	c = &auxiliaryHistoryClient{busy: true}
	if _, err = auxiliaryHistory(t.Context(), c, "s"); err == nil || c.pages != 0 {
		t.Fatal("read a working session")
	}
}
// A session preference persists, and asking for a kind that already runs
// automatically does not spend a second call on it.
func TestAuxPreferPersistsAndRunDoesNotRepeatAnAutomaticKind(t *testing.T) {
	c, err := auxiliary.New(t.TempDir(), func(context.Context, auxiliary.Input) (auxiliary.Output, error) {
		t.Error("unexpected paid execution")
		return auxiliary.Output{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	p := auxiliary.Profile{Account: "work", Agent: "codex", Backend: accounts.BrokeredAccessToken, Model: "test"}
	if _, err = c.Save("summary", p); err != nil {
		t.Fatal(err)
	}
	m := &Manager{aux: c}
	enabled := func(state *api.AuxState, kind string) bool {
		for _, pref := range state.Preferences {
			if pref.Kind == kind {
				return pref.Enabled
			}
		}
		return false
	}
	state, err := m.AuxPrefer(t.Context(), &api.AuxPreferInput{
		SessionId:   "s",
		Preferences: []*api.AuxPreference{{Kind: "summary", Enabled: true}},
	})
	if err != nil || !enabled(state, "summary") {
		t.Fatal(state, err)
	}
	// Already automatic, so this is a no-op rather than a run; the controller
	// would have called the provider, which the fixture refuses.
	if state, err = m.AuxRun(t.Context(), &api.AuxRunInput{SessionId: "s", Kinds: []string{"summary"}}); err != nil || !enabled(state, "summary") {
		t.Fatal(state, err)
	}
	if state, err = m.AuxStatus(t.Context(), &api.AuxStatusInput{SessionId: "s"}); err != nil || !enabled(state, "summary") {
		t.Fatal(state, err)
	}
	// The session said yes; the installation default did not.
	cfg, err := m.AuxConfig(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, profile := range cfg.Profiles {
		if profile.Kind == "summary" && profile.Enabled {
			t.Fatal("a session preference enabled the installation default")
		}
	}
}

// A kind this build does not know is refused rather than mistaken for another.
func TestAuxRefusesAnUnknownKind(t *testing.T) {
	c, err := auxiliary.New(t.TempDir(), func(context.Context, auxiliary.Input) (auxiliary.Output, error) {
		t.Error("unexpected paid execution")
		return auxiliary.Output{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	m := &Manager{aux: c}
	if _, err = m.AuxRun(t.Context(), &api.AuxRunInput{SessionId: "s", Kinds: []string{"haiku"}}); err == nil {
		t.Fatal("ran an unknown kind")
	}
	if _, err = m.AuxPrefer(t.Context(), &api.AuxPreferInput{SessionId: "s", Preferences: []*api.AuxPreference{{Kind: "title"}}}); err == nil {
		t.Fatal("accepted a per-session title preference")
	}
}

// Results carry the kind that produced them, and a clipped one says so instead
// of being thrown away.
func TestAuxReportsResultsPerKind(t *testing.T) {
	a := auxOf("s", &auxiliary.Job{
		ID: "j", Run: "run", Turn: 7, Status: "running",
		SummaryRequested: true, SuggestionRequested: true,
		Summary:          "half of it\n[truncated]",
	})
	if len(a.Kinds) != 2 || a.SessionId != "s" || a.State != "running" {
		t.Fatal(a)
	}
	// Partial while running: the summary is reported before the suggestion
	// beside it has been generated.
	if len(a.Results) != 1 || a.Results[0].Kind != "summary" || !a.Results[0].Truncated {
		t.Fatal(a.Results)
	}
	if AuxText(a, "summary") == "" || AuxText(a, "suggestion") != "" {
		t.Fatal("wrong text per kind")
	}
}
