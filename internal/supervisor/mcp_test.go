package supervisor

import (
	"bytes"
	"encoding/json"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/mcpconfig"
	"strings"
	"testing"
)

func TestMCPAdapterOnlyPublishesResolvedServers(t *testing.T) {
	snapshot := mcpconfig.Snapshot{Servers: map[string]mcpconfig.Server{"local": {Kind: "stdio", Command: "not-in-agent", Args: []string{"private"}}, "remote": {Kind: "http", URL: "https://example.test/mcp", Headers: map[string]string{"Authorization": "test-token"}}}}
	args := mcpArgs(claudeArgs(), "claude", "/tools/cxz", "/state dir", "session", snapshot)
	var config map[string]map[string]map[string]any
	for i, a := range args {
		if a == "--mcp-config" {
			if e := json.Unmarshal([]byte(args[i+1]), &config); e != nil {
				t.Fatal(e)
			}
		}
	}
	servers := config["mcpServers"]
	if len(servers) != 2 || servers["local"]["command"] != "/tools/cxz" || servers["remote"]["type"] != "http" {
		t.Fatal(config)
	}
	codex := strings.Join(mcpArgs([]string{"app-server"}, "codex", "/tools/cxz", "/state dir", "session", snapshot), " ")
	if !strings.Contains(codex, `mcp_servers.remote.url="https://example.test/mcp"`) || !strings.Contains(codex, "_mcp-bridge") || strings.Contains(codex, "not-in-agent") {
		t.Fatal(codex)
	}
	empty := mcpArgs(claudeArgs(), "claude", "cxz", "state", "id", mcpconfig.Snapshot{})
	for i, a := range empty {
		if a == "--mcp-config" && empty[i+1] != `{"mcpServers":{}}` {
			t.Fatal(empty)
		}
	}
}

type mcpBuffer struct{ bytes.Buffer }

func (*mcpBuffer) Close() error { return nil }
func TestMCPDisabledClearsInstructionsOnCodexResume(t *testing.T) {
	out := &mcpBuffer{}
	s := &Supervisor{session: core.Session{ProjectID: "P"}, snap: core.Snapshot{VendorID: "old-thread"}, stdin: out, modelDisabled: true}
	c := &codexProtocol{s: s}
	c.startThread()
	var request struct {
		Method string
		Params map[string]any
	}
	if e := json.Unmarshal(out.Bytes(), &request); e != nil {
		t.Fatal(e)
	}
	value, ok := request.Params["developerInstructions"]
	if request.Method != "thread/resume" || !ok || value != "" {
		t.Fatal(request)
	}
}
