package tui

import (
	"context"
	"fmt"
	"testing"

	"github.com/lesomnus/cxz/api"
	"google.golang.org/grpc"
)

type pagedClient struct {
	api.SessionsClient
	after uint64
}

func (c *pagedClient) History(_ context.Context, r *api.WatchRequest, _ ...grpc.CallOption) (*api.EventBatch, error) {
	c.after = r.AfterSeq
	b := &api.EventBatch{}
	for i := r.AfterSeq + 1; i <= r.AfterSeq+128; i++ {
		b.Events = append(b.Events, &api.Event{Seq: i, Kind: "assistant", Text: fmt.Sprint(i)})
	}
	return b, nil
}

func TestHistoryTailAndPrependAnchor(t *testing.T) {
	m := conversationModel()
	client := &pagedClient{}
	m.client = client
	tail := []*api.Event{}
	for i := uint64(257); i <= 384; i++ {
		tail = append(tail, &api.Event{Seq: i, Kind: "assistant", Text: fmt.Sprint(i)})
	}
	m.applyHistoryPage(historyPage{id: "s", start: 256, initial: true, events: tail})
	if len(m.events["s"]) != 128 || m.cursor["s"] != 384 || !m.view.AtBottom() {
		t.Fatal("tail not loaded atomically")
	}
	m.view.GotoTop()
	height := m.view.TotalLineCount()
	cmd := m.loadOlderHistory()
	if cmd == nil || m.loadOlderHistory() != nil {
		t.Fatal("missing page or duplicate in-flight fetch")
	}
	m.Update(cmd())
	if client.after != 128 || m.historyStart["s"] != 128 || len(m.events["s"]) != 256 || m.cursor["s"] != 384 {
		t.Fatal("reverse cursor corrupted")
	}
	if m.view.YOffset != m.view.TotalLineCount()-height {
		t.Fatal("prepend moved visible content")
	}
	m.watchEpoch = 2
	m.applyHistoryPage(historyPage{id: "s", initial: true, epoch: 1, events: tail})
	if len(m.events["s"]) != 256 {
		t.Fatal("stale watch overwrote history")
	}
}
