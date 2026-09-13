package supervisor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/lesomnus/cxz/internal/agentview"
	"github.com/lesomnus/cxz/internal/core"
)

func TestClaudeCatalogRefresh(t *testing.T) {
	s, input := displaySupervisor(t, "claude")
	s.readModels()
	if !bytes.Contains(input.Bytes(), []byte(`"subtype":"list_models"`)) {
		t.Fatal(input.String())
	}
	before := input.Len()
	s.readModels()
	if input.Len() != before {
		t.Fatal("unthrottled polling")
	}
	s.consume([]byte(`{"type":"control_response","response":{"request_id":"cxz-models","subtype":"success","response":{"models":[{"value":"fable-test","supportedEffortLevels":["low","high"]}]}}}`))
	if len(s.modelOptions) != 1 || s.modelOptions[0].ID != "fable-test" || s.modelSource != "list_models" {
		t.Fatal(s.modelOptions)
	}
	s.modelRequested = time.Now().Add(-2 * time.Minute)
	s.readModels()
	if input.Len() == before {
		t.Fatal("no refresh")
	}
	s.consume([]byte(`{"type":"control_response","response":{"request_id":"cxz-models","subtype":"error","error":"unsupported control"}}`))
	s.modelRequested = time.Time{}
	before = input.Len()
	s.readModels()
	if input.Len() != before || len(s.modelOptions) != 1 {
		t.Fatal("unsupported refresh lost fallback catalog")
	}
}

func TestProviderModelSettings(t *testing.T) {
	for _, provider := range []string{"claude", "codex"} {
		t.Run(provider, func(t *testing.T) {
			s, input := displaySupervisor(t, provider)
			s.modelOptions = []agentview.ModelOption{{ID: "test", Efforts: []string{"low", "high"}, Default: true, DefaultEffort: "low"}}
			for i, text := range []string{"/model test", "/effort high", "/effort default"} {
				c := core.Command{ClientID: fmt.Sprint(i), RunID: "run", Text: text}
				if _, err := s.execute("send", c); err != nil {
					t.Fatal(err)
				}
				if provider == "claude" {
					if s.settingPending != c.ClientID {
						t.Fatal("setting not pending")
					}
					if _, err := s.execute("send", core.Command{ClientID: "blocked", RunID: "run", Text: "chat"}); err == nil {
						t.Fatal("sent during pending update")
					}
					s.consume([]byte(fmt.Sprintf(`{"type":"control_response","response":{"subtype":"success","request_id":"cxz-setting-%s","response":{}}}`, c.ClientID)))
				}
				before := input.Len()
				if _, err := s.execute("send", c); err != nil || input.Len() != before {
					t.Fatal("setting retried", err)
				}
				if s.snap.State != "idle" {
					t.Fatal("setting started a turn")
				}
			}
			if s.session.Model != "test" || s.effort != "" {
				t.Fatal("settings not applied")
			}
			for _, e := range s.log.All() {
				if e.Kind == "input" || e.Kind == "turn_end" {
					t.Fatal("configuration became chat")
				}
			}
			if _, err := s.execute("send", core.Command{ClientID: "invalid", RunID: "run", Text: "/effort invented"}); err == nil {
				t.Fatal("unreported effort accepted")
			}
			if provider == "codex" {
				input.Reset()
				if _, err := s.execute("send", core.Command{ClientID: "chat", RunID: "run", Text: "hello"}); err != nil {
					t.Fatal(err)
				}
				if !bytes.Contains(input.Bytes(), []byte(`"model":"test"`)) || !bytes.Contains(input.Bytes(), []byte(`"effort":"low"`)) {
					t.Fatal(input.String())
				}
			}
		})
	}
}

func TestCatalogAdaptersAndClaudeRejection(t *testing.T) {
	for provider, raw := range map[string]string{
		"claude": `{"models":[{"value":"test","displayName":"Test","supportedEffortLevels":["low","high"]}]}`,
		"codex":  `{"data":[{"id":"test","model":"test","displayName":"Test","supportedReasoningEfforts":[{"reasoningEffort":"low"},{"reasoningEffort":"high"}]}]}`,
	} {
		models := agentview.Models(provider, []byte(raw))
		if len(models) != 1 || models[0].ID != "test" || len(models[0].Efforts) != 2 {
			t.Fatal(models)
		}
	}
	s, _ := displaySupervisor(t, "claude")
	s.modelOptions = []agentview.ModelOption{{ID: "test"}}
	_, err := s.execute("send", core.Command{ClientID: "change", RunID: "run", Text: "/model test"})
	if err != nil {
		t.Fatal(err)
	}
	s.consume([]byte(`{"type":"control_response","response":{"subtype":"error","request_id":"cxz-setting-change","error":"unsupported"}}`))
	if s.session.Model != "" || s.settingPending != "" || s.snap.State != "idle" {
		t.Fatal("rejected setting applied")
	}
	raw, _ := json.Marshal(map[string]string{"value": "restored"})
	s.restoreSetting("model", raw)
	if s.session.Model != "restored" {
		t.Fatal("setting replay failed")
	}
}

func TestCodexCatalogPaginationAndFailure(t *testing.T) {
	s, input := displaySupervisor(t, "codex")
	s.readModels()
	s.consume([]byte(`{"id":"cxz-models","result":{"data":[{"model":"one"}],"nextCursor":"next"}}`))
	if !bytes.Contains(input.Bytes(), []byte(`"cursor":"next"`)) || len(s.modelOptions) != 0 {
		t.Fatal("partial catalog published or not paginated")
	}
	s.consume([]byte(`{"id":"cxz-models","result":{"data":[{"model":"two"}],"nextCursor":null}}`))
	if len(s.modelOptions) != 2 || !s.modelRequested.IsZero() {
		t.Fatal("catalog incomplete")
	}
	s.consume([]byte(`{"id":"cxz-models","error":{"code":-32601}}`))
	if s.snap.State != "idle" || len(s.modelOptions) != 2 {
		t.Fatal("catalog failure affected session")
	}
}
