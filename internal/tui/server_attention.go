package tui

import (
	"context"
	"slices"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/notification"
	"github.com/lesomnus/cxz/internal/sessionattention"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type attentionRead struct {
	wanted, saved, rejected uint64
	busy                    bool
	retry                   time.Time
}
type serverAttentionMessage struct {
	epoch  uint64
	update sessionattention.Update
	ended  bool
}
type serverReadRecorded struct {
	id      string
	through uint64
	err     error
}

func (m *model) attentionBackend() (sessionattention.Backend, bool) {
	if m.attentionUnsupported {
		return nil, false
	}
	if c, ok := m.client.(interface{ SupportsAttention() bool }); ok && !c.SupportsAttention() {
		return nil, false
	}
	backend, ok := m.client.(sessionattention.Backend)
	return backend, ok
}
func (m *model) observeServerAttention(backend sessionattention.Backend, sessions []*api.Session) tea.Cmd {
	if m.sessionActivity == nil {
		m.sessionActivity = map[string]*sessionActivity{}
	}
	parts := []string{}
	live := map[string]bool{}
	for _, s := range sessions {
		parts = append(parts, s.Id+"\x00"+s.ProjectId)
		live[s.Id] = true
		a := m.sessionActivity[s.Id]
		if a == nil {
			a = &sessionActivity{}
			m.sessionActivity[s.Id] = a
		}
		a.run, a.state, a.lastSeq = s.RunId, s.State, s.LastSeq
	}
	for id := range m.attentionReads {
		if !live[id] {
			delete(m.attentionReads, id)
		}
	}
	slices.Sort(parts)
	key := strings.Join(parts, "\n")
	if m.program == nil || key == m.attentionKey {
		return nil
	}
	if m.attentionCancel != nil {
		m.attentionCancel()
	}
	m.attentionKey = key
	m.attentionEpoch++
	epoch := m.attentionEpoch
	if len(sessions) == 0 {
		return nil
	}
	parent := m.ctx
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	m.attentionCancel = cancel
	program := m.program
	values := append([]*api.Session(nil), sessions...)
	go func() {
		err := backend.WatchAttention(ctx, values, func(update sessionattention.Update) {
			if ctx.Err() == nil {
				program.Send(serverAttentionMessage{epoch: epoch, update: update})
			}
		})
		if err != nil && ctx.Err() == nil {
			program.Send(serverAttentionMessage{epoch: epoch, update: sessionattention.Update{Err: err}, ended: true})
		}
	}()
	return nil
}
func (m *model) receiveServerAttention(v serverAttentionMessage) tea.Cmd {
	if v.epoch != m.attentionEpoch {
		return nil
	}
	if v.update.Err != nil {
		m.notice = "Notification sync: " + v.update.Err.Error()
		if v.ended && status.Code(v.update.Err) == codes.Unimplemented {
			m.attentionUnsupported = true
			m.attentionKey = ""
		}
		return nil
	}
	if m.sessionActivity == nil {
		m.sessionActivity = map[string]*sessionActivity{}
	}
	for _, id := range v.update.Sessions {
		a := m.sessionActivity[id]
		if a != nil {
			a.done = 0
			a.serverQuestion = false
		}
	}
	for id, entry := range v.update.Unread {
		a := m.sessionActivity[id]
		if a == nil {
			a = &sessionActivity{}
			m.sessionActivity[id] = a
		}
		a.done = max(entry.Seq, 1)
		a.seen = 0
		a.serverQuestion = entry.Question
	}
	var cmds []tea.Cmd
	for _, kind := range v.update.Alerts {
		sound := notification.Attention
		if kind == "complete" {
			sound = notification.Complete
		}
		cmds = append(cmds, requestSound(sound))
	}
	m.acknowledgeSession()
	return tea.Batch(cmds...)
}
func (m *model) queueAttentionRead(id string, through uint64) {
	if through == 0 {
		return
	}
	if m.attentionReads == nil {
		m.attentionReads = map[string]*attentionRead{}
	}
	r := m.attentionReads[id]
	if r == nil {
		r = &attentionRead{}
		m.attentionReads[id] = r
	}
	if through != r.rejected {
		r.wanted = max(r.wanted, through)
	}
}
func (m *model) flushAttentionReads() tea.Cmd {
	backend, ok := m.attentionBackend()
	if !ok {
		return nil
	}
	parent := m.ctx
	if parent == nil {
		parent = context.Background()
	}
	var cmds []tea.Cmd
	for id, r := range m.attentionReads {
		if r.busy || r.wanted <= r.saved || time.Now().Before(r.retry) {
			continue
		}
		r.busy = true
		through := r.wanted
		cmds = append(cmds, func() tea.Msg {
			ctx, cancel := context.WithTimeout(parent, 5*time.Second)
			defer cancel()
			return serverReadRecorded{id, through, backend.MarkConversationRead(ctx, id, through)}
		})
	}
	return tea.Batch(cmds...)
}
func (m *model) receiveServerRead(v serverReadRecorded) tea.Cmd {
	r := m.attentionReads[v.id]
	if r == nil {
		return nil
	}
	r.busy = false
	r.retry = time.Now().Add(time.Second)
	if v.err == nil {
		r.saved = max(r.saved, v.through)
		if a := m.sessionActivity[v.id]; a != nil {
			a.seen = max(a.seen, v.through)
		}
	} else {
		switch status.Code(v.err) {
		case codes.FailedPrecondition, codes.OutOfRange, codes.NotFound:
			r.rejected = v.through
			r.wanted, r.saved = 0, 0
		}
	}
	return nil
}
