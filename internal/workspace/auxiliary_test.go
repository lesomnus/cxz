package workspace

import (
	"context"
	"encoding/json"
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
func TestAuxiliarySessionActionPersistsAndEnabledBareCommandDoesNotFetch(t *testing.T) {
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
	enabled := true
	for _, r := range []auxiliary.Request{{Action: "session", Session: "s", Task: "summary", Enabled: &enabled}, {Action: "session", Session: "s", Task: "summary"}, {Action: "status", Session: "s"}} {
		b, _ := json.Marshal(r)
		receipt, err := m.auxiliaryRequest(t.Context(), b)
		if err != nil {
			t.Fatal(err)
		}
		var out auxiliary.Reply
		if err = json.Unmarshal([]byte(receipt.Status), &out); err != nil || out.SessionConfig == nil || !out.SessionConfig.Summary || out.Config.Summary.Enabled {
			t.Fatal(out, err)
		}
	}
}
