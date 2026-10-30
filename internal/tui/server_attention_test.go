package tui

import (
	"context"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/sessionattention"
)

type attentionClient struct {
	api.SessionsClient
	through uint64
}

func (c *attentionClient) WatchAttention(context.Context, []*api.Session, func(sessionattention.Update)) error {
	return nil
}
func (c *attentionClient) MarkConversationRead(_ context.Context, _ string, through uint64) error {
	c.through = through
	return nil
}
func TestServerAttentionControlsUnreadAndAcknowledgment(t *testing.T) {
	c := &attentionClient{}
	s := &api.Session{Id: "one", ProjectId: "project", State: "idle"}
	m := model{ctx: t.Context(), client: c, sessions: []*api.Session{s}, panelFocus: true}
	m.observeSessions([]*api.Session{s})
	m.receiveServerAttention(serverAttentionMessage{update: sessionattention.Update{Sessions: []string{"one"}, Unread: map[string]sessionattention.Entry{"one": {Seq: 9}}}})
	if ansi.Strip(m.sessionIndicator(s)) != "+" {
		t.Fatal("server unread state not rendered")
	}
	if len(m.attentionReads) != 0 {
		t.Fatal("panel focus acknowledged unseen content")
	}
	m.queueAttentionRead("one", 8)
	m.queueAttentionRead("one", 9)
	cmd := m.flushAttentionReads()
	if cmd == nil {
		t.Fatal("read cursor not scheduled")
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		msg = batch[0]()
	}
	recorded, ok := msg.(serverReadRecorded)
	if !ok {
		t.Fatalf("unexpected command: %T", msg)
	}
	m.receiveServerRead(recorded)
	if c.through != 9 {
		t.Fatal("cursor was not sent to server", c.through)
	}
	m.receiveServerAttention(serverAttentionMessage{update: sessionattention.Update{Sessions: []string{"one"}, Unread: map[string]sessionattention.Entry{}}})
	if ansi.Strip(m.sessionIndicator(s)) != " " {
		t.Fatal("remote receipt did not clear the indicator")
	}
}
