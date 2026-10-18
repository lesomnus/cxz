package authview

import (
	"github.com/lesomnus/cxz/api"
	"testing"
)

func TestAuthenticationFailed(t *testing.T) {
	for _, tc := range []struct {
		kind, text, payload string
		want                bool
	}{
		{"turn_end", "failed", `{"type":"result","subtype":"success","is_error":true,"result":"Failed to authenticate: OAuth session expired and could not be refreshed"}`, true},
		{"turn_end", "completed", `{"type":"result","is_error":false,"result":"Failed to authenticate: OAuth session expired and could not be refreshed"}`, false},
		{"diagnostic", "Claude error", `{"error":"authentication_failed"}`, false},
		{"diagnostic", "Claude error", `{"error":{"code":"authentication_failed"}}`, true},
		{"diagnostic", "Authentication failed", `{"willRetry":true}`, false},
		{"assistant", "Authentication failed", `{"error":{"code":401}}`, false},
		{"tool_result", "Not logged in", `{"status":401}`, false},
		{"turn_end", "failed", `{"usage":{"tokens":401},"result":"authentication failed"}`, false},
	} {
		if got := AuthenticationFailed(&api.Event{Kind: tc.kind, Text: tc.text, Payload: []byte(tc.payload)}); got != tc.want {
			t.Fatalf("%+v: got %v", tc, got)
		}
	}
}
