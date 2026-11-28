package supervisor

import (
	"time"

	"github.com/lesomnus/cxz/internal/core"
)

func claudeSessionArgs(session core.Session, effort, resume string) []string {
	if resume == "" && session.Fork != nil && session.Fork.VendorID != "" {
		return append(claudeRunArgs(session.Model, effort, session.Fork.VendorID), "--fork-session")
	}
	return claudeRunArgs(session.Model, effort, resume)
}

func claudeArgs() []string {
	// Use Claude's default tool set. Built-in /context and /compact are enabled;
	// workspace/user settings, hooks and MCP remain isolated as before.
	return []string{"-p", "--input-format", "stream-json", "--output-format", "stream-json", "--verbose", "--permission-mode", "manual", "--permission-prompts", "host", "--permission-prompt-tool", "stdio", "--setting-sources=", "--strict-mcp-config", "--mcp-config", `{"mcpServers":{}}`, "--settings", `{"permissions":{"defaultMode":"manual","allow":[],"deny":[],"ask":["Bash","Edit","Write"]},"disableAllHooks":true}`}
}

func claudeRunArgs(model, effort, resume string) []string {
	args := claudeArgs()
	if resume != "" {
		args = append(args, "--resume", resume)
	}
	if model != "" {
		args = append(args, "--model", model)
	}
	if effort != "" {
		// Session-only levels such as max are not persisted by Claude's
		// settings layer. Restore cxz's journaled choice as a launch flag.
		args = append(args, "--effort", effort)
	}
	return args
}

// Ask the already authenticated process; never read or refresh OAuth tokens
// here. Older CLIs can reject this experimental control without harming a turn.
//
// Every session polls for itself. Claude's only backend logs in per session
// profile, so two sessions under one account alias can hold credentials for
// different provider accounts -- and did: cxz#400 measured one session shown 2%
// of a weekly limit while the account it actually spends was at 100%, because a
// snapshot shared by alias had each publishing the other's numbers. Sharing a
// poll between logins cannot be made safe by asking who they belong to; a
// credential does not always say, and a usage figure that is confidently the
// wrong account's is worse than one poll a minute per session.
func (s *Supervisor) readClaudeQuota() {
	if !s.quotaDisabled && time.Since(s.quotaPolledAt) >= time.Minute {
		s.sendClaudeQuota()
	}
}

func (s *Supervisor) sendClaudeQuota() {
	s.quotaPolledAt = time.Now()
	s.requestQuota(map[string]any{"type": "control_request", "request_id": "cxz-quota", "request": map[string]any{"subtype": "get_usage", "skip_behaviors": true}})
}
