package supervisor

import "time"

func claudeArgs() []string {
	// Use Claude's default tool set. Built-in /context and /compact are enabled;
	// workspace/user settings, hooks and MCP remain isolated as before.
	return []string{"-p", "--input-format", "stream-json", "--output-format", "stream-json", "--verbose", "--permission-mode", "manual", "--permission-prompts", "host", "--permission-prompt-tool", "stdio", "--setting-sources=", "--strict-mcp-config", "--mcp-config", `{"mcpServers":{}}`, "--settings", `{"permissions":{"defaultMode":"manual","allow":[],"deny":[],"ask":["Bash","Edit","Write"]},"disableAllHooks":true}`}
}

func (s *Supervisor) readClaudeQuota() {
	// Ask the already authenticated process; never read/refresh OAuth tokens in
	// the TUI. Older CLIs can reject this experimental control without harming a turn.
	if s.quotaToken != "" {
		s.readRemoteClaudeQuota()
		return
	}
	if !s.claimClaudeQuota() {
		return
	}
	s.sendClaudeQuota()
}

func (s *Supervisor) sendClaudeQuota() {
	s.quotaFallbackAt = time.Now()
	s.requestQuota(map[string]any{"type": "control_request", "request_id": "cxz-quota", "request": map[string]any{"subtype": "get_usage", "skip_behaviors": true}})
}
