package tui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lesomnus/cxz/internal/auxiliary"
	"github.com/lesomnus/cxz/resource"
)

func configured(account, model, effort string) auxiliary.Profile {
	return auxiliary.Profile{Enabled: true, Account: account, Agent: "claude", Backend: "project-local-oauth", Model: model, Effort: effort}
}

// Choosing another task's profile has to produce the same profile, field for
// field, because that equality is what makes the two generate in one call. The
// wizard also has nothing left to ask: no catalog, no login probe.
func TestReuseAnotherTasksProfileSavesInOneStep(t *testing.T) {
	m := conversationModel()
	m.ctx = context.Background()
	c := &setupClient{}
	m.client = c
	summary := configured("work1", "sonnet", "medium")
	// Editing the suggestion offers the summary's profile first, as one line.
	p := &auxiliaryPage{config: auxiliary.Config{Summary: summary}, selected: 1, editing: true, step: "account",
		accounts: []*resource.Account{resource.Account_builder{Alias: "other", Agent: "claude"}.Build()}}
	m.settingsPage = &settingsPage{auxiliary: p}
	choices := p.choices()
	if len(choices) != 2 || choices[0] != "use work1/sonnet/medium from Summary" {
		t.Fatalf("%q", choices)
	}
	if !strings.Contains(m.auxiliaryScreen(), "use work1/sonnet/medium from Summary") {
		t.Fatal("the choice is not on screen")
	}

	cmd := m.auxiliaryKey(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("selecting a reused profile did nothing")
	}
	m.Update(cmd())
	if c.got.call != "set" || c.got.kind != "suggestion" {
		t.Fatalf("not saved as the suggestion: %+v", c.got)
	}
	// Copied whole: what travels is the account, the model and the effort. What
	// an account authenticates as is read off the registered account by the
	// server, so a client no longer sends it at all.
	saved := c.got.profile
	if saved.Account != summary.Account || saved.Model != summary.Model || saved.Effort != summary.Effort || !saved.Enabled {
		t.Fatalf("the saved profile differs from the one it copied:\n got %+v\nwant %+v", saved, summary)
	}
	if c.got.call == "models" || c.loginCount != 0 {
		t.Fatal("a validated profile was probed again")
	}
	if p.step == "model" || p.step == "effort" {
		t.Fatal("the wizard still asked for a model:", p.step)
	}
}

// An account chosen from the list still walks the wizard, and must not be read
// as a reuse entry just because reuse entries come first.
func TestAccountChoiceIsOffsetByReuseEntries(t *testing.T) {
	m := conversationModel()
	m.ctx = context.Background()
	c := &setupClient{}
	m.client = c
	p := &auxiliaryPage{
		config:   auxiliary.Config{Summary: configured("work1", "sonnet", "medium")},
		accounts: []*resource.Account{resource.Account_builder{Alias: "second", Agent: "codex", AuthBackend: "project-local-oauth"}.Build()},
		selected: 1, editing: true, step: "account",
		choice: 1, // the account, below the single reuse entry
	}
	m.settingsPage = &settingsPage{auxiliary: p}
	cmd := m.auxiliaryKey(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("selecting the account did nothing")
	}
	m.Update(cmd())
	if p.step != "model" {
		t.Fatal("the wizard did not continue to the model step:", p.step)
	}
	if c.got.call != "models" || c.got.account != "second" {
		t.Fatalf("wrong account taken: %+v", c.got)
	}
}

// What is offered and in what order: the other half of the pair that can share
// a call first, nothing that was never configured, and no duplicate of a
// profile already listed.
func TestReusableOffersOnlyUsefulProfiles(t *testing.T) {
	shared := configured("work1", "sonnet", "medium")
	p := &auxiliaryPage{config: auxiliary.Config{Summary: shared, Suggestion: shared, Title: configured("work2", "haiku", "")}}
	p.selected = 2 // editing the title
	labels := []string{}
	for _, v := range p.reusable(p.task()) {
		labels = append(labels, v.label())
	}
	if len(labels) != 1 || labels[0] != "use work1/sonnet/medium from Summary" {
		t.Fatalf("duplicate or missing entries: %q", labels)
	}

	p.selected = 0 // editing the summary
	labels = labels[:0]
	for _, v := range p.reusable(p.task()) {
		labels = append(labels, v.label())
	}
	if len(labels) != 2 || labels[0] != "use work1/sonnet/medium from Next-message suggestion" || labels[1] != "use work2/haiku from Session title" {
		t.Fatalf("wrong order or labels: %q", labels)
	}

	// Nothing configured, nothing to reuse.
	empty := &auxiliaryPage{}
	if got := empty.reusable("summary"); len(got) != 0 {
		t.Fatalf("%+v", got)
	}
	// A profile with an account but no model was never finished.
	half := &auxiliaryPage{config: auxiliary.Config{Summary: auxiliary.Profile{Account: "work1", Agent: "claude"}}}
	if got := half.reusable("suggestion"); len(got) != 0 {
		t.Fatalf("%+v", got)
	}
	// A disabled source is still worth copying, and the copy is enabled: a task
	// turned off elsewhere is not a reason to configure this one as off.
	off := configured("work1", "sonnet", "medium")
	off.Enabled = false
	disabled := &auxiliaryPage{config: auxiliary.Config{Summary: off}}
	got := disabled.reusable("suggestion")
	if len(got) != 1 || !got[0].profile.Enabled {
		t.Fatalf("%+v", got)
	}
}

// The saving is invisible in the rows, so the screen says whether the pair
// shares a call -- and does not claim it when the profiles only look alike.
func TestScreenReportsWhetherThePairSharesACall(t *testing.T) {
	m := conversationModel()
	shared := configured("work1", "sonnet", "medium")
	p := &auxiliaryPage{config: auxiliary.Config{Summary: shared, Suggestion: shared}}
	m.settingsPage = &settingsPage{auxiliary: p}
	if !strings.Contains(m.auxiliaryScreen(), "one call per turn") {
		t.Fatal("a shared profile is not reported")
	}
	// Same account and model, different effort: still two calls.
	p.config.Suggestion = configured("work1", "sonnet", "high")
	screen := m.auxiliaryScreen()
	if strings.Contains(screen, "one call per turn") {
		t.Fatal("profiles that differ were reported as shared")
	}
	if !strings.Contains(screen, "a call each") {
		t.Fatal("the difference is not explained:", screen)
	}
	// One of them off: there is no pair to talk about.
	p.config.Suggestion = auxiliary.Profile{}
	if screen = m.auxiliaryScreen(); strings.Contains(screen, "a call each") || strings.Contains(screen, "one call per turn") {
		t.Fatal("an unconfigured task was described as a pair:", screen)
	}
}
