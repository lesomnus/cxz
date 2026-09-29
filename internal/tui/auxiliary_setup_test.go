package tui

import (
	"context"
	"encoding/json"
	"errors"
	"google.golang.org/grpc"
	"io"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/agentview"
	"github.com/lesomnus/cxz/internal/auxiliary"
	"github.com/lesomnus/cxz/resource"
)

type setupClient struct {
	api.SessionsClient
	loginErr error
	got      auxiliary.Request
	alias    string
}

func (c *setupClient) LoginAuxiliary(ctx context.Context, alias string, in io.ReadCloser, out io.Writer) error {
	c.alias = alias
	return c.loginErr
}
func (c *setupClient) Docker(ctx context.Context, in *api.DockerInput, _ ...grpc.CallOption) (*api.Receipt, error) {
	json.Unmarshal(in.Spec, &c.got)
	out := auxiliary.Reply{}
	if c.got.Action == "models" {
		out.Models = []agentview.ModelOption{{ID: "fixture", Efforts: []string{"low", "high"}}}
	}
	b, _ := json.Marshal(out)
	return &api.Receipt{Status: string(b)}, nil
}
func TestAuxiliarySetupAuthenticatesThenSelectsCapabilities(t *testing.T) {
	for _, fail := range []bool{false, true} {
		m := conversationModel()
		m.ctx = context.Background()
		c := &setupClient{}
		if fail {
			c.loginErr = errors.New("login canceled")
		}
		m.client = c
		p := &auxiliaryPage{editing: true, step: "account", accounts: []*resource.Account{resource.Account_builder{Alias: "work", Agent: "claude"}.Build()}}
		m.settingsPage = &settingsPage{auxiliary: p}
		batch := m.auxiliaryKey(tea.KeyMsg{Type: tea.KeyEnter})().(tea.BatchMsg)
		if !strings.Contains(m.View(), "AI task account login") {
			t.Fatal("login hidden behind settings")
		}
		_, cmd := m.Update(batch[0]())
		if c.alias != "work" || m.busy {
			t.Fatal("wrong login target or busy state")
		}
		if fail {
			if cmd != nil || p.message != "login canceled" || c.got.Action != "" {
				t.Fatal("failed auth advanced wizard")
			}
			continue
		}
		m.Update(cmd())
		if p.step != "model" || len(p.models) != 1 {
			t.Fatal("catalog not presented")
		}
		m.auxiliaryKey(tea.KeyMsg{Type: tea.KeyEnter})
		if p.step != "effort" || len(p.choices()) != 3 {
			t.Fatal("missing efforts or provider default")
		}
		m.auxiliaryKey(tea.KeyMsg{Type: tea.KeyDown})
		cmd = m.auxiliaryKey(tea.KeyMsg{Type: tea.KeyEnter})
		m.Update(cmd())
		if c.got.Action != "put" || c.got.Profile.Account != "work" || c.got.Profile.Model != "fixture" || c.got.Profile.Effort != "low" || p.editing {
			t.Fatalf("wrong saved profile: %+v", c.got)
		}
	}
}
func TestAuxiliaryEmptyChoicesAndCancelDoNotSave(t *testing.T) {
	m := conversationModel()
	p := &auxiliaryPage{editing: true, step: "model"}
	m.settingsPage = &settingsPage{auxiliary: p}
	if m.auxiliaryKey(tea.KeyMsg{Type: tea.KeyEnter}) != nil {
		t.Fatal("empty selection saved")
	}
	m.auxiliaryKey(tea.KeyMsg{Type: tea.KeyEsc})
	if p.editing || p.config.Configured() {
		t.Fatal("cancel persisted draft")
	}
}

func TestAuxiliaryAccountListingOnlyUpdatesActivePage(t *testing.T) {
	m := conversationModel()
	p := &auxiliaryPage{editing: true, step: "account", busy: true}
	m.settingsPage = &settingsPage{auxiliary: p}
	a := resource.Account_builder{Alias: "registered", Agent: "codex"}.Build()
	m.Update(auxiliaryAccounts{page: p, accounts: []*resource.Account{a}})
	if p.busy || len(p.choices()) != 1 || !strings.Contains(m.auxiliaryScreen(), "registered") {
		t.Fatal("account list unavailable")
	}
	m.Update(auxiliaryAccounts{page: &auxiliaryPage{}, err: errors.New("stale")})
	if p.message != "" {
		t.Fatal("old account request affected wizard")
	}
}
