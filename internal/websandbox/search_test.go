package websandbox

import (
	"strings"
	"testing"
	"time"

	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func TestScopedSearchFindsUnloadedHistoryAndFencesPages(t *testing.T) {
	s := New(42, time.Millisecond)
	defer s.Close()
	x := &Sessions{S: s}
	request := resource.SessionSearchRequest_builder{Sessions: []string{"session-3"}, Query: proto.String("history item"), IgnoreCase: proto.Bool(true), Limit: proto.Int32(2)}.Build()
	first := &fixtureStream[resource.SessionSearchReply]{ctx: t.Context()}
	if err := x.Search(request, first); err != nil {
		t.Fatal(err)
	}
	if len(first.replies) != 2 || first.replies[0].GetVisit().GetSessionId() != "session-3" || len(first.replies[0].GetVisit().GetHits()) != 2 {
		t.Fatalf("wrong scope: %v", first.replies)
	}
	cursor := first.replies[1].GetSummary().GetNextCursor()
	if cursor == "" {
		t.Fatal("missing next page")
	}
	s.mu.Lock()
	s.event(s.sessions[2], "assistant", "History item NEW", "", nil)
	s.mu.Unlock()
	request.SetCursor(cursor)
	next := &fixtureStream[resource.SessionSearchReply]{ctx: t.Context()}
	if err := x.Search(request, next); err != nil {
		t.Fatal(err)
	}
	if next.replies[0].GetVisit().GetHits()[0].GetSeq() >= first.replies[0].GetVisit().GetHits()[1].GetSeq() {
		t.Fatal("cursor shifted after new response")
	}
	request.SetCursor("")
	request.SetQuery("HISTORY ITEM 0001")
	old := &fixtureStream[resource.SessionSearchReply]{ctx: t.Context()}
	if err := x.Search(request, old); err != nil || !strings.Contains(old.replies[0].GetVisit().GetHits()[0].GetSnippet(), "History item 0001") {
		t.Fatalf("old history unavailable: %v", err)
	}
	request.SetCursor(cursor)
	if err := x.Search(request, old); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("wrong query accepted cursor: %v", err)
	}
}
