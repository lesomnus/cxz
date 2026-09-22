package tui

import (
	"context"
	"io"
	"sync"
	"testing"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/containerterm"
	"github.com/lesomnus/cxz/internal/transport"
)

type remoteTerminalClient struct {
	api.SessionsClient // Any inventory lookup panics: only the remote RPC is needed.
	project            string
	terminal           *testRemoteTerminal
}

func (c *remoteTerminalClient) OpenTerminal(_ context.Context, project string, _, _ int) (containerterm.Terminal, error) {
	c.project = project
	return c.terminal, nil
}

type testRemoteTerminal struct {
	done chan struct{}
	once sync.Once
}

func (t *testRemoteTerminal) Read([]byte) (int, error)    { <-t.done; return 0, io.EOF }
func (t *testRemoteTerminal) Write(p []byte) (int, error) { return len(p), nil }
func (t *testRemoteTerminal) Resize(int, int) error       { return nil }
func (t *testRemoteTerminal) Wait() error                 { return nil }
func (t *testRemoteTerminal) Close() error                { t.once.Do(func() { close(t.done) }); return nil }

func TestRemoteTerminalCapturesTargetAndRetainsShell(t *testing.T) {
	m := conversationModel()
	m.ctx = transport.WithRemote(m.ctx)
	client := &remoteTerminalClient{terminal: &testRemoteTerminal{done: make(chan struct{})}}
	m.client = client
	m.current().Id, m.current().ProjectId = "work::session", "work::project"
	m.project, m.panelProjects = nil, nil
	cmd := m.toggleTerminal()
	m.current().Id, m.current().ProjectId = "home::session", "home::project"
	opened := cmd().(terminalOpened)
	if opened.err != nil || opened.session == nil || opened.id != "work::session" || client.project != "work::project" {
		t.Fatal("remote terminal used local metadata or changed targets", opened, client.project)
	}
	defer opened.session.Close()
	m.Update(opened)
	if m.terminal() != nil {
		t.Fatal("remote terminal appeared on another session")
	}
	m.current().Id, m.current().ProjectId = "work::session", "work::project"
	m.toggleTerminal() // fold
	if m.terminal().open {
		t.Fatal("panel did not fold")
	}
	select {
	case <-client.terminal.done:
		t.Fatal("fold closed remote shell")
	default:
	}
	if cmd := m.toggleTerminal(); cmd != nil || m.terminal().session != opened.session {
		t.Fatal("unfold replaced remote shell")
	}
}
