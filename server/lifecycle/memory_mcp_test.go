package lifecycle

import (
	"encoding/json"
	"github.com/lesomnus/cxz/internal/memorymcp"
	"github.com/lesomnus/cxz/resource"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"testing"
)

func TestMemoryMCPUsesResourceAPIAndRetainsOwnership(t *testing.T) {
	f, stack, _ := taskStack(t)
	conn := taskConn(t, stack)
	ctx := t.Context()
	connect := func(index int) *mcp.ClientSession {
		t.Helper()
		owner := f.sessions[index]
		provider, err := memorymcp.New(conn, memorymcp.Binding{Project: f.p.Id, Session: owner.Id, Run: owner.RunId})
		if err != nil {
			t.Fatal(err)
		}
		st, ct := mcp.NewInMemoryTransports()
		server, err := provider.Connect(ctx, st, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { server.Close() })
		client, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil).Connect(ctx, ct, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { client.Close() })
		return client
	}
	a, b := connect(0), connect(1)
	call := func(c *mcp.ClientSession, name string, args map[string]any, wantError bool) map[string]any {
		t.Helper()
		r, e := c.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		if e != nil {
			t.Fatal(e)
		}
		if r.IsError != wantError {
			t.Fatalf("%s: %v", name, r)
		}
		if wantError {
			return nil
		}
		raw, e := json.Marshal(r.StructuredContent)
		if e != nil {
			t.Fatal(e)
		}
		var out map[string]any
		if e = json.Unmarshal(raw, &out); e != nil {
			t.Fatal(e)
		}
		return out
	}
	created := call(a, "memory_add", map[string]any{"client_id": "first", "name": "one.md", "content": "initial", "labels": map[string]string{"topic": "design"}}, false)
	id := created["id"].(string)
	again := call(a, "memory_add", map[string]any{"client_id": "first", "name": "retry", "content": "ignored"}, false)
	if again["id"] != id {
		t.Fatal("creation retry duplicated memory")
	}
	call(b, "memory_read", map[string]any{"id": id}, false)
	call(b, "memory_update", map[string]any{"id": id, "revision": "1", "content": "forbidden"}, true)
	saved := call(a, "memory_update", map[string]any{"id": id, "revision": "1", "content": "changed"}, false)
	if saved["revision"] != "2" {
		t.Fatal(saved)
	}
	call(a, "memory_update", map[string]any{"id": id, "revision": "1", "content": "stale"}, true)
	read := call(a, "memory_read", map[string]any{"id": id, "base_revision": "1", "format": "diff"}, false)
	if read["format"] != "diff" || read["content"] != nil {
		t.Fatal(read)
	}
	list := call(a, "memory_list", map[string]any{"labels": map[string]string{"topic": "design"}}, false)
	if len(list["memories"].([]any)) != 1 {
		t.Fatal(list)
	}
	call(a, "memory_changes", map[string]any{}, false)
	// The MCP cannot switch its scope or session through tool arguments.
	call(a, "memory_add", map[string]any{"client_id": "escape", "name": "bad", "content": "bad", "project": "other"}, true)
	f.sessions[0].RunId += "-restarted"
	layer, _ := resource.Find[Layer](stack)
	if err := layer.RefreshSession(ctx, f.sessions[0].Id); err != nil {
		t.Fatal(err)
	}
	call(a, "memory_read", map[string]any{"id": id}, true)
}
