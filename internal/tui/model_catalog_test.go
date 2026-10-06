package tui

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
)

func catalogEvent(seq uint64, model string, applied bool, ms int64) *api.Event {
	payload, _ := json.Marshal(map[string]any{
		"model": model, "source": "initialize", "applied_reported": applied,
		"models": []map[string]any{{"id": model, "efforts": []string{"low", "high"}}},
	})
	return &api.Event{Seq: seq, RunId: "run", Kind: "models", Payload: payload, TimeMs: ms}
}

// Opening the picker is one request, and the second picker is none: the record
// describes a run, and /model followed by /effort is the same run.
func TestPickerAsksOncePerRun(t *testing.T) {
	m := conversationModel()
	c := &catalogClient{events: []*api.Event{catalogEvent(12, "sonnet", true, time.Now().UnixMilli())}}
	m.client = c
	m.Update(m.modelCommand("/model")())
	if c.modelCalls != 1 || c.pages != 0 {
		t.Fatalf("calls=%d history pages=%d", c.modelCalls, c.pages)
	}
	if p := m.modelPicker; p == nil || p.catalog == nil || p.catalog.Model != "sonnet" {
		t.Fatalf("%+v", m.modelPicker)
	}
	m.modelPicker = nil
	reopen(m, "/effort")
	if c.modelCalls != 1 {
		t.Fatal("the second picker asked again:", c.modelCalls)
	}
	if p := m.modelPicker; p == nil || p.catalog == nil || p.loading {
		t.Fatalf("the cached record was not used: %+v", m.modelPicker)
	}
	// A different run is a different agent state, so it is fetched.
	m.current().RunId = "later"
	m.modelPicker = nil
	m.Update(m.modelCommand("/model")())
	if c.modelCalls != 2 {
		t.Fatal("a new run reused another run's record:", c.modelCalls)
	}
}

// A manager without the capability RPC still works, and the scan reads backwards
// from the end rather than walking the whole journal to reach the newest run.
func TestPickerFallsBackToABackwardScan(t *testing.T) {
	m := conversationModel()
	var events []*api.Event
	for i := uint64(1); i <= 400; i++ {
		events = append(events, &api.Event{Seq: i, RunId: "old", Kind: "assistant"})
	}
	events = append(events, catalogEvent(401, "sonnet", true, time.Now().UnixMilli()))
	c := &catalogClient{events: events, legacy: true}
	m.client = c
	m.current().LastSeq = 401
	m.Update(m.modelCommand("/model")())
	if p := m.modelPicker; p == nil || p.catalog == nil || p.catalog.Model != "sonnet" {
		t.Fatalf("the fallback did not find the record: %+v", m.modelPicker)
	}
	// 401 events in pages of 128: a forward scan needs four, backwards needs one.
	if c.pages != 1 {
		t.Fatal("the fallback did not read backwards:", c.pages)
	}
}

// The picker says what it is showing: where it came from, how old it is, and
// whether the provider confirmed it. Codex reports no applied value, and a
// request must not read as a fact.
func TestPickerSaysHowOldAndHowSureItIs(t *testing.T) {
	m := conversationModel()
	old := time.Now().Add(-90 * time.Minute).UnixMilli()
	c := &catalogClient{events: []*api.Event{catalogEvent(3, "sonnet", false, old)}}
	m.client = c
	m.Update(m.modelCommand("/model")())
	view := ansi.Strip(m.modelPickerOverlay(strings.Repeat("transcript\n", 20)))
	for _, want := range []string{"initialize", "1h30m ago", "applied value not confirmed by the provider", "Alt+R ask the provider"} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing %q: %s", want, view)
		}
	}
	// A confirmed record says nothing about confirmation; the absence is the note.
	m.modelCatalogs = nil
	m.modelPicker = nil
	c.events = []*api.Event{catalogEvent(4, "sonnet", true, time.Now().UnixMilli())}
	m.Update(m.modelCommand("/model")())
	view = ansi.Strip(m.modelPickerOverlay(strings.Repeat("transcript\n", 20)))
	if strings.Contains(view, "not confirmed") {
		t.Fatal("a confirmed record was marked unconfirmed:", view)
	}
	if !strings.Contains(view, "just now") {
		t.Fatal("a fresh record is not described as fresh:", view)
	}
}

// Alt+R asks the provider. The answer arrives as a journal event, not as a
// reply, so the picker updates from the stream it is already watching.
func TestRefreshAsksTheProviderAndUpdatesFromTheEvent(t *testing.T) {
	m := conversationModel()
	c := &catalogClient{events: []*api.Event{catalogEvent(5, "sonnet", true, time.Now().UnixMilli())}}
	m.client = c
	m.Update(m.modelCommand("/model")())
	cmd := m.modelPickerKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}, Alt: true})
	if cmd == nil {
		t.Fatal("refresh did nothing")
	}
	m.Update(cmd())
	if c.refreshed != 1 {
		t.Fatal("the provider was not asked:", c.refreshed)
	}
	if !m.modelPicker.refreshing {
		t.Fatal("the picker does not say it is waiting")
	}
	view := ansi.Strip(m.modelPickerOverlay(strings.Repeat("transcript\n", 20)))
	if !strings.Contains(view, "updates when it answers") {
		t.Fatal(view)
	}
	// The provider's answer: a new catalog event for the same run.
	m.receiveEvent(received{id: m.current().Id, event: catalogEvent(6, "opus", true, time.Now().UnixMilli())}, false)
	p := m.modelPicker
	if p.refreshing || p.catalog.Model != "opus" {
		t.Fatalf("the picker did not take the provider's answer: %+v", p.catalog)
	}
	// The cache took it too, so the next picker does not undo the refresh.
	if v := m.modelCatalogs[p.id+"/"+p.run]; v == nil || v.Model != "opus" {
		t.Fatalf("%+v", v)
	}
}
