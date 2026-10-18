package tui

import (
	"fmt"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/accounts"
	"github.com/lesomnus/cxz/internal/agentview"
	"github.com/lesomnus/cxz/internal/multiclient"
)

func authHint(s *api.Session, e *api.Event) string {
	if s == nil || e == nil || s.ProjectId == "" || s.Account == "" {
		return ""
	}
	if !agentview.AuthenticationFailed(e) {
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
