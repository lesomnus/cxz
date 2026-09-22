package tui

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/accounts"
	"github.com/lesomnus/cxz/internal/multiclient"
)

var authAPIError = regexp.MustCompile(`(?i)^api error:\s*401\b`)

func authErrorText(text string) bool {
	text = strings.ToLower(strings.TrimSpace(text))
	switch text {
	case "authentication failed", "not logged in", "login required",
		"not logged in · please run /login", "not logged in. please run /login",
		"central codex login failed", "central authentication unavailable; check account login",
		"central authentication refresh failed":
		return true
	}
	return authAPIError.MatchString(text)
}

// Only error fields carry authentication evidence. Never scan result text,
// usage metrics, IDs, tool output or arbitrary nested JSON for matching words.
func authErrorPayload(raw []byte, depth int) bool {
	if depth > 4 {
		return false
	}
	f := fields(raw)
	if len(f) == 0 {
		var text string
		return json.Unmarshal(raw, &text) == nil && authErrorText(text)
	}
	var retrying bool
	if json.Unmarshal(f["willRetry"], &retrying) == nil && retrying {
		return false
	}
	for _, key := range []string{"api_error_status", "status", "status_code", "httpStatusCode", "code"} {
		if code, ok := f.number(key); ok && code == 401 {
			return true
		}
	}
	for _, key := range []string{"type", "code"} {
		switch f.text(key) {
		case "authentication_error", "unauthorized", "invalid_api_key", "token_expired":
			return true
		}
	}
	if authErrorText(f.text("message")) || authErrorPayload(f["error"], depth+1) {
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

func authHint(s *api.Session, e *api.Event) string {
	if s == nil || e == nil || s.ProjectId == "" || s.Account == "" {
		return ""
	}
	switch e.Kind {
	case "turn_end":
		if e.Text != "failed" {
			return ""
		}
	case "diagnostic", "stderr":
	default:
		return ""
	}
	if !authErrorText(e.Text) && !authErrorPayload(e.Payload, 0) {
		return ""
	}
	connection, id := multiclient.Split(s.Id)
	host := "the daemon host"
	if connection != "" {
		host = "the " + connection + " daemon host"
	}
	login := "cxz account login --session " + id + " " + s.Account
	if s.AuthBackend == accounts.BrokeredAccessToken {
		login = "cxz account login " + s.Account
	}
	return fmt.Sprintf("Agent authentication failed. On %s, stop this session, run %s, then cxz session resume %s. Failed prompts are not resent.", host, login, id)
}
