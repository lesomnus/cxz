package tui

import (
	"github.com/lesomnus/cxz/api"
	"testing"
	"time"
)

func TestFrontendRestartWaitsForUnsavedInputAndInteractiveWork(t *testing.T) {
	cases := map[string]func(*model){
		"draft":               func(m *model) { m.input.SetValue("do not lose this") },
		"other session draft": func(m *model) { m.drafts = map[string]string{"other": "private draft"} },
		"terminal":            func(m *model) { m.terminals = map[string]*terminalPanel{"s": {open: true}} },
		"login":               func(m *model) { m.workflow = &accountWorkflow{} },
		"modal":               func(m *model) { m.settingsPage = &settingsPage{} },
		"request in flight":   func(m *model) { m.busy = true },
		"recent input":        func(m *model) { m.lastUIInput = time.Now() },
		"startup":             func(m *model) { m.autoStarted = time.Now() },
	}
	for name, block := range cases {
		t.Run(name, func(t *testing.T) {
			m := &model{input: newComposer(), autoStarted: time.Now().Add(-10 * time.Minute), lastUIInput: time.Now().Add(-10 * time.Minute)}
			if !m.frontendIdle(time.Now()) {
				t.Fatal("idle baseline blocked")
			}
			block(m)
			if m.frontendIdle(time.Now()) {
				t.Fatal("unsafe automatic restart")
			}
		})
	}
}
func TestAgentWorkDoesNotPreventFrontendOnlyRestart(t *testing.T) {
	m := &model{input: newComposer(), autoStarted: time.Now().Add(-10 * time.Minute), sessions: []*api.Session{{Id: "s", State: "working"}}}
	if !m.frontendIdle(time.Now()) {
		t.Fatal("frontend restart unnecessarily depends on agent idle")
	}
}
