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
	after  uint64
	before uint64
}

// Transcript answers a page of rows, which is what the pager asks for now. The
// fence it is asked with is recorded so a test can say which page was wanted.
func (c *pagedClient) Transcript(_ context.Context, r *api.TranscriptRequest, _ ...grpc.CallOption) (*api.TranscriptReply, error) {
	c.after, c.before = r.AfterSeq, r.BeforeSeq
	first, last := r.AfterSeq+1, r.AfterSeq+128
	if r.BeforeSeq > 0 {
		last = r.BeforeSeq - 1
		first = 1
		if last > 128 {
			first = last - 127
		}
	}
	out := &api.TranscriptReply{SnapshotSeq: max(r.SnapshotSeq, last), HasOlder: first > 1}
	for i := first; i <= last; i++ {
		out.Events = append(out.Events, &api.Event{Seq: i, Kind: "assistant", Text: fmt.Sprint(i)})
	}
	return out, nil
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
	m.applyHistoryPage(historyPage{id: "s", start: 256, initial: true, rows: rowsOf(tail)})
	if len(m.rows["s"]) != 128 || m.cursor["s"] != 384 || !m.view.AtBottom() {
		t.Fatal("tail not loaded atomically")
	}
	m.view.GotoTop()
	height := m.view.TotalLineCount()
	cmd := m.loadOlderHistory()
	if cmd == nil || m.loadOlderHistory() != nil {
		t.Fatal("missing page or duplicate in-flight fetch")
	}
	m.Update(cmd())
	// Asked for what is behind the loaded edge, not for what follows a journal
	// coordinate: a page of rows is fenced by before_seq.
	if client.before != 257 || m.historyStart["s"] != 128 || len(m.rows["s"]) != 256 || m.cursor["s"] != 384 {
		t.Fatal("reverse cursor corrupted")
	}
	if m.view.YOffset != m.view.TotalLineCount()-height {
		t.Fatal("prepend moved visible content")
	}
	m.watchEpoch = 2
	m.applyHistoryPage(historyPage{id: "s", initial: true, epoch: 1, rows: rowsOf(tail)})
	if len(m.rows["s"]) != 256 {
		t.Fatal("stale watch overwrote history")
	}
}
