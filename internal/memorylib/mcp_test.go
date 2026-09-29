package memorylib

import (
	"context"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"testing"
)

func TestMCPDiscoveryAndOwnWrites(t *testing.T) {
	_, a, b := fixture(t)
	ctx := context.Background()
	server := MCPServer(a)
	st, ct := mcp.NewInMemoryTransports()
	ss, e := server.Connect(ctx, st, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer ss.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	cs, e := client.Connect(ctx, ct, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer cs.Close()
	list, e := cs.ListTools(ctx, nil)
	if e != nil || len(list.Tools) != 4 {
		t.Fatal(list, e)
	}
	out, e := cs.CallTool(ctx, &mcp.CallToolParams{Name: "memory_update", Arguments: map[string]any{"document": "handoff.md", "content": "pending"}})
	if e != nil || out.IsError {
		t.Fatal(out, e)
	}
	if got := must(t, b, Request{Action: "read", ID: a.OwnID(), Document: "handoff.md"}).Content; got != "pending" {
		t.Fatal(got)
	}
	out, e = cs.CallTool(ctx, &mcp.CallToolParams{Name: "memory_update", Arguments: map[string]any{"id": b.OwnID(), "document": "handoff.md", "content": "overwrite"}})
	if e == nil && !out.IsError {
		t.Fatal("cross session write accepted")
	}
}
