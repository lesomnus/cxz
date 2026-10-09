package tui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lesomnus/cxz/api"
	"google.golang.org/grpc"
)

// The transcript used to ask once a second whether a task had finished. It
// does not have to: the controller that runs the task is in the manager this
// client is already talking to, so the answer can be pushed. One subscription
// per open conversation replaces the poll, and what arrives is the same state
// the question used to answer with -- so everything that reads it is unchanged.

// auxRetry is how long a failed subscription waits. A stream that cannot be
// opened is reported once and retried quietly; the alternative is a client that
// shows a stale summary forever without saying so.
const auxRetry = 5 * time.Second

type auxiliaryWatch struct {
	cancel     context.CancelFunc
	stream     grpc.ServerStreamingClient[api.AuxState]
	open       bool
	retryAfter time.Time
}

// auxiliaryStream is a subscription that was opened, or the failure to open it.
type auxiliaryStream struct {
	connection, session string
	stream              grpc.ServerStreamingClient[api.AuxState]
	cancel              context.CancelFunc
	err                 error
}

// auxiliaryPushed is one state from a subscription, or its end.
type auxiliaryPushed struct {
	connection, session string
	state               *api.AuxState
	err                 error
}

// watchAuxiliary keeps one subscription for the open conversation and drops
// the ones for conversations that are no longer open.
func (m *model) watchAuxiliary() tea.Cmd {
	if m.ctx == nil || m.client == nil {
		return nil
	}
	if m.auxiliaryWatches == nil {
		m.auxiliaryWatches = map[string]*auxiliaryWatch{}
	}
	s := m.current()
	key := ""
	if s != nil {
		key = m.connectionRef() + "/" + s.Id
	}
	for k, w := range m.auxiliaryWatches {
		if k != key {
			w.cancel()
			delete(m.auxiliaryWatches, k)
		}
	}
	if s == nil {
		return nil
	}
	if w := m.auxiliaryWatches[key]; w != nil && (w.open || time.Now().Before(w.retryAfter)) {
		return nil
	}
	connection, session := m.connectionRef(), s.Id
	ctx, cancel := context.WithCancel(m.contextFor(connection))
	m.auxiliaryWatches[key] = &auxiliaryWatch{cancel: cancel, open: true}
	client := m.client
	return func() tea.Msg {
		stream, err := client.AuxEvents(ctx, &api.AuxStatusInput{SessionId: session})
		return auxiliaryStream{connection: connection, session: session, stream: stream, cancel: cancel, err: err}
	}
}

func (m *model) receiveAuxiliaryStream(v auxiliaryStream) tea.Cmd {
	key := v.connection + "/" + v.session
	w := m.auxiliaryWatches[key]
	if w == nil || w.cancel == nil {
		v.cancel()
		return nil
	}
	if v.err != nil {
		v.cancel()
		w.open, w.retryAfter = false, time.Now().Add(auxRetry)
		m.auxiliaryError = v.err.Error()
		return nil
	}
	m.auxiliaryError = ""
	w.stream = v.stream
	return auxiliaryRecv(v)
}

// auxiliaryRecv waits for the next state. It is re-armed on every message, so
// the stream is read for as long as the conversation is open.
func auxiliaryRecv(v auxiliaryStream) tea.Cmd {
	return func() tea.Msg {
		state, err := v.stream.Recv()
		return auxiliaryPushed{connection: v.connection, session: v.session, state: state, err: err}
	}
}

func (m *model) receiveAuxiliaryPush(v auxiliaryPushed) tea.Cmd {
	key := v.connection + "/" + v.session
	w := m.auxiliaryWatches[key]
	if w == nil {
		return nil
	}
	if v.err != nil {
		w.cancel()
		w.open, w.retryAfter = false, time.Now().Add(auxRetry)
		if m.ctx != nil && m.ctx.Err() == nil {
			m.auxiliaryError = v.err.Error()
		}
		return nil
	}
	// The same answer the poll used to carry, so the same code reads it.
	cmd := m.receiveAuxiliary(auxiliaryResult{
		version:    m.auxiliaryVersions[key],
		connection: v.connection,
		session:    v.session,
		action:     "status",
		reply:      auxStateOf(v.state),
	})
	if w.stream == nil {
		return cmd
	}
	return tea.Batch(cmd, auxiliaryRecv(auxiliaryStream{connection: v.connection, session: v.session, stream: w.stream}))
}
