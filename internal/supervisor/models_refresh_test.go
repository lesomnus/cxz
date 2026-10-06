package supervisor

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/lesomnus/cxz/internal/core"
)

// The refresh op exists so a client can ask the provider on purpose. That is a
// different question from routine republishing, so it is not subject to the
// throttle that keeps routine republishing from interrogating a provider.
func TestRefreshOpSkipsTheThrottle(t *testing.T) {
	s, input := displaySupervisor(t, "claude")
	s.readModels()
	before := input.Len()
	if before == 0 {
		t.Fatal("no catalog request at all")
	}
	if _, err := s.execute("models", core.Command{ClientID: "picker", RunID: "run"}); err != nil {
		t.Fatal(err)
	}
	if input.Len() == before {
		t.Fatal("an explicit refresh was throttled")
	}
	if !bytes.Contains(input.Bytes(), []byte(`"subtype":"list_models"`)) {
		t.Fatal(input.String())
	}
	// It asks and returns: the catalog is published as an event, so nothing waits
	// for a provider and no receipt pretends the answer has arrived.
	for _, e := range s.log.All() {
		if e.Kind == "models" {
			t.Fatal("refresh published a catalog before the provider answered")
		}
	}
}

// A session that is not idle cannot be interrogated -- the same rule a setting
// change follows -- and the refusal says so rather than leaving the client to
// guess from a missing update.
func TestRefreshRequiresAnIdleSession(t *testing.T) {
	s, _ := displaySupervisor(t, "claude")
	s.snap = Replay([]core.Event{{Kind: "state", Text: "working", RunID: "run"}})
	_, err := s.execute("models", core.Command{ClientID: "picker", RunID: "run"})
	if err == nil || !strings.Contains(err.Error(), "idle") {
		t.Fatal("a refusal to interrogate a working session did not say why:", err)
	}
	// A run that has ended is not a run to ask about, and that is a different
	// refusal from a busy one.
	if _, err = s.execute("models", core.Command{ClientID: "other", RunID: "gone"}); err == nil || !strings.Contains(err.Error(), "run_id") {
		t.Fatal("asked on behalf of a stale run:", err)
	}
}

// What cxz asked for and what the provider confirmed are different claims. Claude
// reads its applied settings back; Codex reports none, and saying so is what
// keeps a request from being displayed as the agent's state.
func TestPublishedCatalogSaysWhetherTheProviderConfirmed(t *testing.T) {
	for _, provider := range []string{"claude", "codex"} {
		t.Run(provider, func(t *testing.T) {
			s, _ := displaySupervisor(t, provider)
			s.publishModels()
			var payload struct {
				AppliedReported bool   `json:"applied_reported"`
				Source          string `json:"source"`
			}
			found := false
			for _, e := range s.log.All() {
				if e.Kind == "models" {
					if err := json.Unmarshal(e.Payload, &payload); err != nil {
						t.Fatal(err)
					}
					found = true
				}
			}
			if !found {
				t.Fatal("no catalog was published")
			}
			// Neither agent has reported applied settings yet, so neither may
			// claim confirmation.
			if payload.AppliedReported {
				t.Fatal("confirmation claimed without the provider reporting one")
			}
			if payload.Source == "" {
				t.Fatal("no source recorded")
			}
			if provider == "claude" {
				s.appliedModel, s.appliedEffort = "claude-opus-5[1m]", "high"
				s.publishModels()
				for _, e := range s.log.All() {
					if e.Kind == "models" {
						json.Unmarshal(e.Payload, &payload)
					}
				}
				if !payload.AppliedReported {
					t.Fatal("a read-back setting was not marked as confirmed")
				}
			}
		})
	}
}

func TestRefreshRejectedWhenTheAgentHasNoCatalog(t *testing.T) {
	s, _ := displaySupervisor(t, "claude")
	s.modelDisabled = true
	if _, err := s.execute("models", core.Command{ClientID: "a", RunID: "run"}); err == nil {
		t.Fatal("asked an agent that reports no catalog")
	}
	s.modelDisabled = false
	s.modelRequested = time.Now()
	if _, err := s.execute("models", core.Command{ClientID: "b", RunID: "run"}); err != nil {
		t.Fatal(err)
	}
}
