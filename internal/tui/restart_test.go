package tui

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lesomnus/cxz/api"
	"google.golang.org/grpc"
)

type restartClient struct {
	api.SessionsClient
	calls              []string
	run, state         string
	stopErr, resumeErr error
	changeAfterStop    bool
	keys               []string
}

func (c *restartClient) Get(_ context.Context, r *api.SessionRef, _ ...grpc.CallOption) (*api.Session, error) {
	c.calls = append(c.calls, "get:"+r.Id)
	return &api.Session{Id: r.Id, RunId: c.run, State: c.state}, nil
}
func (c *restartClient) Stop(_ context.Context, r *api.Control, _ ...grpc.CallOption) (*api.Receipt, error) {
	c.calls = append(c.calls, "stop:"+r.SessionId)
	c.keys = append(c.keys, r.ClientId)
	if r.RunId != c.run {
		return nil, errors.New("wrong run")
	}
	if c.changeAfterStop {
		c.run = "new-run"
	}
	c.state = "stopped"
	return &api.Receipt{}, c.stopErr
}
func (c *restartClient) Resume(_ context.Context, r *api.Control, _ ...grpc.CallOption) (*api.Session, error) {
	c.calls = append(c.calls, "resume:"+r.SessionId)
	c.keys = append(c.keys, r.ClientId)
	if r.RunId != c.run {
		return nil, errors.New("wrong run")
	}
	return &api.Session{Id: r.SessionId, RunId: "new-run"}, c.resumeErr
}

func TestRestartConfirmedAndLocallyRouted(t *testing.T) {
	m := conversationModel()
	c := &restartClient{run: "run", state: "working"}
	m.client = c
	m.fullPermission = map[string]string{"s": "run"}
	for _, text := range []string{"/restart confirm", "/restart nonsense", "/restart cancel", "/restart"} {
		m.input.SetValue(text)
		_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
		if cmd != nil || len(c.calls) != 0 {
			t.Fatal("unconfirmed command performed an effect", text)
		}
	}
	m.input.SetValue("/restart confirm")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if cmd == nil || !m.restartBusy || m.fullPermission["s"] != "" {
		t.Fatal("restart not armed safely")
	}
	if m.restartCommand("/restart confirm") != nil {
		t.Fatal("duplicate restart")
	}
	msg := cmd().(restartFinished)
	if msg.err != nil {
		t.Fatal(msg.err)
	}
	if !reflect.DeepEqual(c.calls, []string{"get:s", "stop:s", "get:s", "resume:s"}) {
		t.Fatal(c.calls)
	}
	if c.keys[0] == "" || c.keys[0] == c.keys[1] {
		t.Fatal("effect keys reused")
	}
	m.Update(msg)
	if m.restartBusy || !strings.Contains(m.notice, "restarted") {
		t.Fatal("completion not applied")
	}
}

func TestRestartFailuresAndStaleConfirmation(t *testing.T) {
	for _, kind := range []string{"expired", "changed-session", "changed-run", "server-run", "stop", "resume", "concurrent-run", "stopped"} {
		t.Run(kind, func(t *testing.T) {
			m := conversationModel()
			c := &restartClient{run: "run", state: "idle"}
			m.client = c
			m.restartCommand("/restart")
			switch kind {
			case "expired":
				m.restartConfirm.expires = time.Now().Add(-time.Second)
			case "changed-session":
				m.current().Id = "other"
			case "changed-run":
				m.current().RunId = "new"
			case "server-run":
				c.run = "new"
			case "stop":
				c.stopErr = errors.New("stop failed")
			case "resume":
				c.resumeErr = errors.New("resume failed")
			case "concurrent-run":
				c.changeAfterStop = true
			case "stopped":
				c.state = "stopped"
			}
			cmd := m.restartCommand("/restart confirm")
			if kind == "expired" || kind == "changed-session" || kind == "changed-run" {
				if cmd != nil || len(c.calls) > 0 {
					t.Fatal("stale confirmation accepted")
				}
				return
			}
			if cmd == nil {
				t.Fatal("missing command")
			}
			msg := cmd().(restartFinished)
			if (msg.err == nil) != (kind == "stopped") {
				t.Fatal(msg.err)
			}
			for _, call := range c.calls {
				if kind == "stopped" && strings.HasPrefix(call, "stop:") {
					t.Fatal("stopped session stopped again")
				}
				if kind != "resume" && kind != "stopped" && strings.HasPrefix(call, "resume:") {
					t.Fatal("unsafe resume", c.calls)
				}
			}
		})
	}
}
