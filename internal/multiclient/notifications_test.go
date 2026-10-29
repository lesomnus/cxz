package multiclient

import (
	"context"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/sessionattention"
	"sync"
	"testing"
	"time"
)

type attentionSource struct {
	api.SessionsClient
	mu     sync.Mutex
	marked string
}

func (s *attentionSource) MarkConversationRead(_ context.Context, id string, _ uint64) error {
	s.mu.Lock()
	s.marked = id
	s.mu.Unlock()
	return nil
}
func (s *attentionSource) WatchAttention(ctx context.Context, sessions []*api.Session, emit func(sessionattention.Update)) error {
	emit(sessionattention.Update{Sessions: []string{sessions[0].Id}, Unread: map[string]sessionattention.Entry{sessions[0].Id: {Seq: 7}}})
	<-ctx.Done()
	return nil
}
func TestNotificationIdentityIsScopedAcrossConnections(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	left, right := &attentionSource{}, &attentionSource{}
	c := &Client{primary: "left", sources: map[string]*Source{"left": {Client: left}, "right": {Client: right}}}
	ch := make(chan sessionattention.Update, 2)
	go c.WatchAttention(ctx, []*api.Session{{Id: Scope("left", "same"), ProjectId: Scope("left", "project")}, {Id: Scope("right", "same"), ProjectId: Scope("right", "project")}}, func(u sessionattention.Update) { ch <- u })
	seen := map[string]bool{}
	for range 2 {
		select {
		case u := <-ch:
			for id := range u.Unread {
				seen[id] = true
			}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	if !seen["left::same"] || !seen["right::same"] {
		t.Fatal("merged unrelated session identities", seen)
	}
	if err := c.MarkConversationRead(ctx, "right::same", 7); err != nil {
		t.Fatal(err)
	}
	left.mu.Lock()
	l := left.marked
	left.mu.Unlock()
	right.mu.Lock()
	r := right.marked
	right.mu.Unlock()
	if l != "" || r != "same" {
		t.Fatal("read routed across connections", l, r)
	}
}
