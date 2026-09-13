package tui

import (
	"context"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/lesomnus/cxz/api"
	"google.golang.org/grpc"
)

type activityClient struct {
	api.SessionsClient
	requests []*api.ActivityInput
}

func (c *activityClient) Activity(_ context.Context, r *api.ActivityInput, _ ...grpc.CallOption) (*api.Receipt, error) {
	c.requests = append(c.requests, r)
	return &api.Receipt{}, nil
}

func TestIdleActivityHeartbeatAndDraft(t *testing.T) {
	c := &activityClient{}
	m := &model{ctx: context.Background(), client: c, activityID: "client", input: textarea.New(), sessions: []*api.Session{{Id: "s", RunId: "r"}}}
	m.program = tea.NewProgram(m)
	m.reportActivity()()
	if len(c.requests) != 1 || c.requests[0].Busy {
		t.Fatal("empty idle UI reported busy", c.requests)
	}
	if m.reportActivity() != nil {
		t.Fatal("heartbeat not throttled")
	}
	m.lastActivityReport = time.Time{}
	m.input.SetValue("unsent draft")
	m.reportActivity()()
	if !c.requests[1].Busy {
		t.Fatal("draft not protected")
	}
	m.lastActivityReport = time.Time{}
	m.input.SetValue("")
	m.lastUIInput = time.Now()
	m.reportActivity()()
	if !c.requests[2].Busy {
		t.Fatal("recent input ignored")
	}
	for _, r := range c.requests {
		if r.SessionId != "s" || r.RunId != "r" || r.ClientId != "client" {
			t.Fatal("incorrect lease identity", r)
		}
	}
}
