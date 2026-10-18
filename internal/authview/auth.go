package authview

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/lesomnus/cxz/api"
)

// AuthenticationFailed requires provider failure evidence, never assistant or tool text.
func AuthenticationFailed(e *api.Event) bool {
	if e == nil {
		return false
	}
	switch e.Kind {
	case "turn_end":
		if e.Text != "failed" {
			return false
		}
	case "diagnostic", "stderr", "error":
	default:
		return false
	}
	var p map[string]json.RawMessage
	_ = json.Unmarshal(e.Payload, &p)
	if string(p["willRetry"]) == "true" {
		return false
	}
	return authErrorText(e.Text) || authErrorPayload(e.Payload, 0)
}

func authField(f map[string]json.RawMessage, key string) string {
	var text string
	_ = json.Unmarshal(f[key], &text)
	return text
}

var authAPIError = regexp.MustCompile(`(?i)^api error:\s*401\b`)

func authErrorText(text string) bool {
	text = strings.ToLower(strings.TrimSpace(text))
	switch text {
	case "authentication failed", "not logged in", "login required",
		"not logged in · please run /login", "not logged in. please run /login",
		"central codex login failed", "central authentication unavailable; check account login",
		"central authentication refresh failed",
		"failed to authenticate: oauth session expired and could not be refreshed":
		return true
	}
	return authAPIError.MatchString(text)
}

// Only provider error fields carry evidence, including an explicitly failed
// Claude result. Never scan usage, IDs, tool output, or arbitrary nested JSON.
func authErrorPayload(raw []byte, depth int) bool {
	if depth > 4 {
		return false
	}
	var f map[string]json.RawMessage
	_ = json.Unmarshal(raw, &f)
	if len(f) == 0 {
		var text string
		return json.Unmarshal(raw, &text) == nil && authErrorText(text)
	}
	var retrying bool
	if json.Unmarshal(f["willRetry"], &retrying) == nil && retrying {
		return false
	}
	for _, key := range []string{"api_error_status", "status", "status_code", "httpStatusCode", "code"} {
		var code int
		if json.Unmarshal(f[key], &code) == nil && code == 401 {
			return true
		}
	}
	for _, key := range []string{"type", "code"} {
		switch authField(f, key) {
		case "authentication_error", "authentication_failed", "unauthorized", "invalid_api_key", "token_expired":
			return true
		}
	}
	if authField(f, "type") == "result" && string(f["is_error"]) == "true" && authErrorText(authField(f, "result")) {
		return true
	}
	if authErrorText(authField(f, "message")) || authErrorPayload(f["error"], depth+1) {
		return true
	}
	var errors []json.RawMessage
	_ = json.Unmarshal(f["errors"], &errors)
	for _, err := range errors {
		if authErrorPayload(err, depth+1) {
			return true
		}
	}
	return false
}
