package supervisor

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/lesomnus/cxz/internal/agentview"
	"github.com/lesomnus/cxz/internal/core"
)

// Routine republishing stopped, so the ops that exist to answer a person have
// to answer: a client whose window has scrolled past the last catalog gets
// nothing from a refresh that confirms the catalog is the same.
func TestAnExplicitAskIsAlwaysAnswered(t *testing.T) {
	s, _ := displaySupervisor(t, "codex")
	s.modelOptions = []agentview.ModelOption{{ID: "one"}}
	s.publishModels()
	s.publishModels()
	if n := recorded(s, "models", "catalog"); n != 1 {
		t.Fatal(n)
	}
	if _, err := s.configure(core.Command{ClientID: "picker", RunID: "run", Text: "/model"}); err != nil {
		t.Fatal(err)
	}
	if n := recorded(s, "models", "catalog"); n != 2 {
		t.Fatalf("opening a picker was not answered: %d", n)
	}
	// A refresh asks and returns -- it publishes nothing itself, because a
	// client must not read a provider's answer before it arrives -- so what has
	// to be answered is the publish that the answer triggers.
	if _, err := s.refreshModels(core.Command{ClientID: "refresh", RunID: "run", Text: "/models"}); err != nil {
		t.Fatal(err)
	}
	if n := recorded(s, "models", "catalog"); n != 2 {
		t.Fatalf("a refresh published before the provider answered: %d", n)
	}
	s.publishModels()
	if n := recorded(s, "models", "catalog"); n != 3 {
		t.Fatalf("the answer to a refresh was not recorded: %d", n)
	}
	// And the ask does not leave the session expecting a change that is not
	// coming: the next routine republish is still quiet.
	s.publishModels()
	if n := recorded(s, "models", "catalog"); n != 3 {
		t.Fatalf("a forced answer reopened routine republishing: %d", n)
	}
}

func recorded(s *Supervisor, kind, text string) int {
	var n int
	for _, e := range s.log.All() {
		if e.Kind == kind && (text == "" || e.Text == text) {
			n++
		}
	}
	return n
}

func TestCatalogRecordsOnlyWhatChanged(t *testing.T) {
	s, _ := displaySupervisor(t, "codex")
	s.modelOptions = []agentview.ModelOption{{ID: "one"}}
	for range 5 {
		s.publishModels()
	}
	if n := recorded(s, "models", "catalog"); n != 1 {
		t.Fatalf("recorded %d catalogs for one value", n)
	}
	s.modelOptions = []agentview.ModelOption{{ID: "one"}, {ID: "two"}}
	s.publishModels()
	if n := recorded(s, "models", "catalog"); n != 2 {
		t.Fatalf("a changed catalog was not recorded: %d", n)
	}
	// The effort is in the same state, so changing it is a change.
	s.effort = "high"
	s.publishModels()
	if n := recorded(s, "models", "catalog"); n != 3 {
		t.Fatalf("a changed effort was not recorded: %d", n)
	}
	last := s.log.All()[len(s.log.All())-1]
	if !strings.Contains(string(last.Payload), `"effort":"high"`) {
		t.Fatal(string(last.Payload))
	}
}

func TestQuotaRecordsOnlyWhatChanged(t *testing.T) {
	s, _ := displaySupervisor(t, "codex")
	limits := json.RawMessage(`{"rateLimits":{"primary":{"usedPercent":40,"windowDurationMins":300}}}`)
	s.publishQuota("account/rateLimits/updated", limits)
	for range 4 {
		s.publishQuota("account/rateLimits/updated", limits)
	}
	if n := recorded(s, "usage", "account/rateLimits/updated"); n != 1 {
		t.Fatalf("recorded %d quota payloads for one value", n)
	}
	// Each repetition says the value was seen again, which is what keeps a
	// reader from rendering it as stale.
	if n := recorded(s, "usage_status", agentview.QuotaObserved); n != 4 {
		t.Fatalf("observations recorded: %d", n)
	}
	for _, e := range s.log.All() {
		if e.Kind == "usage_status" && len(e.Payload) > 0 {
			t.Fatal("an observation carried the value again")
		}
	}
	s.publishQuota("account/rateLimits/updated", json.RawMessage(`{"rateLimits":{"primary":{"usedPercent":41,"windowDurationMins":300}}}`))
	if n := recorded(s, "usage", "account/rateLimits/updated"); n != 2 {
		t.Fatalf("a changed quota was not recorded: %d", n)
	}
}

// A turn's token usage is a fact about that turn, not a current value.
func TestTurnTokenUsageIsAlwaysRecorded(t *testing.T) {
	s, _ := displaySupervisor(t, "codex")
	for range 3 {
		s.consume([]byte(`{"method":"thread/tokenUsage/updated","params":{"threadId":"thread","turnId":"u","tokenUsage":{"inputTokens":10}}}`))
	}
	if n := recorded(s, "usage", "thread/tokenUsage/updated"); n != 3 {
		t.Fatalf("recorded %d turn metrics for 3 turns", n)
	}
}

// Retention keeps settings, receipts and background work in its checkpoint, not
// the polled states, so a trim takes the catalog and the quota with it. The
// next poll has to record them again or a reader is left with neither.
func TestTrimAsksForPolledStateAgain(t *testing.T) {
	s, _ := displaySupervisor(t, "codex")
	s.modelOptions = []agentview.ModelOption{{ID: "one"}}
	s.publishModels()
	s.publishModels()
	if n := recorded(s, "models", "catalog"); n != 1 {
		t.Fatal(n)
	}
	s.event("input", "work", "", nil, nil)
	s.event("state", "idle", "", nil, nil)
	checkpoint, err := makeHistoryCheckpoint(s.log.All())
	if err != nil {
		t.Fatal(err)
	}
	if err = s.log.Compact(checkpoint); err != nil {
		t.Fatal(err)
	}
	for _, e := range s.log.All() {
		if e.Kind == "models" {
			t.Fatal("the checkpoint kept a catalog; this test no longer proves anything")
		}
	}
	s.forgetLatest()
	s.publishModels()
	if n := recorded(s, "models", "catalog"); n != 1 {
		t.Fatalf("the catalog was not recorded again after a trim: %d", n)
	}
}
