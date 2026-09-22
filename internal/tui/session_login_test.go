package tui

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/transport"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type sessionLoginClient struct {
	api.SessionsClient
	opens, logins                              int
	request                                    *api.ProjectRequest
	wrongAccount, genericError, repeat, cancel bool
}

func (c *sessionLoginClient) Open(_ context.Context, r *api.ProjectRequest, _ ...grpc.CallOption) (*api.Session, error) {
	c.opens++
	if c.request != nil && (r.ClientId != c.request.ClientId || r.Account != c.request.Account || r.Agent != c.request.Agent || r.Workspace != c.request.Workspace || r.Model != c.request.Model || !r.NewSession) {
		return nil, fmt.Errorf("login retry changed session identity")
	}
	c.request = r
	if c.opens > 1 && !c.repeat {
		return &api.Session{Id: "work::new"}, nil
	}
	if c.genericError {
		return nil, status.Error(codes.FailedPrecondition, "invalid credential file")
	}
	alias := r.Account
	if c.wrongAccount {
		alias = "other"
	}
	s, _ := status.New(codes.FailedPrecondition, "account needs login").WithDetails(&errdetails.ErrorInfo{Domain: "cxz.auth", Reason: "PROJECT_LOGIN_REQUIRED", Metadata: map[string]string{"account": alias, "session_key": r.ClientId}})
	return nil, s.Err()
}

func (c *sessionLoginClient) LoginSession(ctx context.Context, project, account, key string, input io.ReadCloser, output io.Writer) error {
	c.logins++
	if project != c.request.Workspace || account != c.request.Account || key != c.request.ClientId {
		return fmt.Errorf("login targeted a different profile")
	}
	fmt.Fprintln(output, "Visit https://example.invalid/login")
	if c.cancel {
		<-ctx.Done()
		return ctx.Err()
	}
	code, err := bufio.NewReader(input).ReadString('\n')
	if err != nil || code != "fixture#state\n" {
		return fmt.Errorf("incorrect login input")
	}
	return nil
}

func TestDefaultSessionCreationLogsInAndRetries(t *testing.T) {
	for _, remote := range []bool{false, true} {
		t.Run(fmt.Sprint(remote), func(t *testing.T) {
			m := projectModel()
			m.createProjectSession = nil
			m.accountView = true
			if remote {
				m.ctx = transport.WithRemote(m.ctx)
			}
			c := &sessionLoginClient{}
			m.client = c
			batch := m.startAccountWorkflow("work1", "claude", "", true)().(tea.BatchMsg)
			defer m.workflow.close()
			done := make(chan tea.Msg, 1)
			go func() { done <- batch[0]() }()
			for !strings.Contains(m.workflow.output, "https://example.invalid/login") {
				select {
				case msg := <-m.workflow.updates:
					m.Update(msg)
				case msg := <-done:
					t.Fatal("creation ended before login", msg)
				case <-time.After(time.Second):
					t.Fatal("login URL never arrived")
				}
			}
			m.workflow.input.SetValue("fixture#state")
			if strings.Contains(m.View(), "fixture#state") {
				t.Fatal("login code visible")
			}
			m.Update(m.workflowKey(tea.KeyMsg{Type: tea.KeyEnter})())
			select {
			case msg := <-done:
				if result := msg.(workflowDone); result.err != nil {
					t.Fatal(result.err)
				}
				m.Update(msg)
			case <-time.After(time.Second):
				t.Fatal("login did not continue creation")
			}
			if c.opens != 2 || c.logins != 1 || m.wantID != "work::new" || m.accountView || m.workflow != nil {
				t.Fatal("login did not attach exactly one session", c.opens, c.logins, m.wantID)
			}
		})
	}
}

func TestDefaultSessionLoginStopsOnCancelOrUnrelatedError(t *testing.T) {
	for _, name := range []string{"cancel", "other-account", "invalid-credential", "still-missing"} {
		t.Run(name, func(t *testing.T) {
			m := projectModel()
			m.createProjectSession = nil
			c := &sessionLoginClient{cancel: name == "cancel", wrongAccount: name == "other-account", genericError: name == "invalid-credential", repeat: name == "still-missing"}
			m.client = c
			batch := m.startAccountWorkflow("work1", "claude", "", true)().(tea.BatchMsg)
			defer m.workflow.close()
			done := make(chan tea.Msg, 1)
			go func() { done <- batch[0]() }()
			if c.cancel || c.repeat {
				// Waiting on output synchronizes with the request reaching LoginSession.
				for range 2 {
					select {
					case msg := <-m.workflow.updates:
						m.Update(msg)
					case <-time.After(time.Second):
						t.Fatal("login did not start")
					}
				}
				if c.cancel {
					m.workflowKey(tea.KeyMsg{Type: tea.KeyEsc})
				} else {
					m.workflow.input.SetValue("fixture#state")
					m.Update(m.workflowKey(tea.KeyMsg{Type: tea.KeyEnter})())
				}
			}
			select {
			case msg := <-done:
				if msg.(workflowDone).err == nil {
					t.Fatal("failure treated as success")
				}
			case <-time.After(time.Second):
				t.Fatal("workflow blocked")
			}
			wantOpens, wantLogins := 1, 0
			if c.cancel || c.repeat {
				wantLogins = 1
			}
			if c.repeat {
				wantOpens = 2
			}
			if c.opens != wantOpens || c.logins != wantLogins {
				t.Fatal("unexpected retry/login", c.opens, c.logins)
			}
		})
	}
}
