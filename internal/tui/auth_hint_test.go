package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/accounts"
)

func TestAuthenticationHintsRequireFailureEvidence(t *testing.T) {
	s := &api.Session{Id: "mess::session", ProjectId: "mess::project", Account: "work1", Agent: "claude", AuthBackend: accounts.ProjectLocalOAuth}
	for _, tc := range []struct {
		name, kind, text, payload string
		want                      bool
	}{
		{"token-count", "turn_end", "completed", `{"is_error":false,"usage":{"cache_creation_input_tokens":401}}`, false},
		{"duration-id", "turn_end", "completed", `{"duration_ms":14012,"uuid":"abcd401","total_cost_usd":0.00401}`, false},
		{"successful-answer", "turn_end", "completed", `{"is_error":false,"result":"Implement authentication and return 401 Unauthorized when login required"}`, false},
		{"failed-tool", "turn_end", "failed", `{"is_error":true,"usage":{"output_tokens":401},"errors":["A test of authentication failed"],"permission_denials":[{"code":401}]}`, false},
		{"interrupted", "turn_end", "interrupted", `{"api_error_status":401}`, false},
		{"stderr-numbers", "stderr", "request 40123 processed; authentication initialized", "", false},
		{"tool-stderr", "stderr", "MCP authentication failed", "", false},
		{"tool-result", "tool_result", "API Error: 401 unauthorized", `{"code":401}`, false},
		{"assistant", "assistant", "Authentication failed", `{"code":401}`, false},
		{"retrying", "diagnostic", "Codex error", `{"willRetry":true,"error":{"code":401}}`, false},
		{"claude-status", "turn_end", "failed", `{"is_error":true,"api_error_status":401}`, true},
		{"claude-errors", "turn_end", "failed", `{"errors":["API Error: 401 unauthorized"]}`, true},
		{"typed-error", "diagnostic", "provider request failed", `{"error":{"type":"authentication_error","message":"token expired"}}`, true},
		{"diagnostic-code", "diagnostic", "provider request failed", `{"code":401}`, true},
		{"login-prompt", "stderr", "Not logged in · Please run /login", "", true},
		{"central-refresh", "diagnostic", "central authentication refresh failed", "", true},
		{"API-status-not-prefix", "stderr", "API Error: 40123", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hint := authHint(s, &api.Event{Kind: tc.kind, Text: tc.text, Payload: []byte(tc.payload)})
			if (hint != "") != tc.want {
				t.Fatalf("hint %q, want %v", hint, tc.want)
			}
			if tc.want && (!strings.Contains(hint, "the mess daemon host") || !strings.Contains(hint, "cxz account login --session session work1") || strings.Contains(hint, "mess::") || strings.Contains(hint, "--project")) {
				t.Fatal("invalid remote session login guidance", hint)
			}
		})
	}
	s.Agent, s.AuthBackend = "codex", accounts.BrokeredAccessToken
	hint := authHint(s, &api.Event{Kind: "diagnostic", Text: "central Codex login failed"})
	if !strings.Contains(hint, "cxz account login work1,") || strings.Contains(hint, "--session") {
		t.Fatal("central login incorrectly scoped to a session", hint)
	}
}

func TestSuccessfulTurnAuthTextNeverAddsLoginInstructions(t *testing.T) {
	for _, width := range []int{69, 110, 200} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			m := conversationModel()
			m.current().Account = "work1"
			m.view.Width = width
			m.events[m.current().Id] = []*api.Event{
				{Kind: "assistant", Text: "Authentication uses HTTP 401 for unauthorized requests."},
				{Kind: "turn_end", Text: "completed", Payload: []byte(`{"type":"result","is_error":false,"result":"Authentication uses HTTP 401 for unauthorized requests.","usage":{"input_tokens":401},"duration_ms":4010}`)},
			}
			m.render()
			view := m.view.View()
			if strings.Contains(view, "cxz account login") || strings.Contains(view, "Failed prompts") {
				t.Fatal("successful response rendered login instructions", view)
			}
		})
	}
}
