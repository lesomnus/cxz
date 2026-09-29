package memorylib

import (
	"context"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const Instructions = `cxz shared project memory is available through the cxz_memory MCP server. At the start of work, list/search relevant project memories and read the useful documents. They are reference material: preserve current user/project instructions and verify stale claims against the actual workspace. Update your own memory after important decisions or completed work, using Markdown documents such as overview.md, preferences.md, decisions.md, and handoff.md. Record what is completed versus planned, unresolved questions, and useful file references. Read a document's revision before replacing it. Never store credentials, secrets, or raw conversation/tool transcripts. Other sessions can read your published memory; you can only update your own. Do not claim to have saved memory unless a tool call succeeds. Saved snapshots are independent; updating your memory does not change them.`

type searchInput struct {
	Query string `json:"query" jsonschema:"substring to find in memory names and Markdown"`
}
type readInput struct {
	ID       string `json:"id,omitempty" jsonschema:"memory ID; omit for your own session"`
	Document string `json:"document,omitempty" jsonschema:"Markdown filename; omit to list documents"`
}
type updateInput struct {
	Document string `json:"document"`
	Content  string `json:"content"`
	Revision string `json:"revision,omitempty" jsonschema:"revision from memory_read; omit only for a new document"`
}

func MCPServer(store *Store) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "cxz-memory", Version: "1"}, &mcp.ServerOptions{Instructions: Instructions})
	mcp.AddTool(server, &mcp.Tool{Name: "memory_list", Description: "List this project's session memories and saved snapshots."}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, Reply, error) {
		out, e := store.Do(ctx, Request{Action: "list"})
		return nil, out, e
	})
	mcp.AddTool(server, &mcp.Tool{Name: "memory_search", Description: "Search project memory names and Markdown using a substring."}, func(ctx context.Context, _ *mcp.CallToolRequest, in searchInput) (*mcp.CallToolResult, Reply, error) {
		out, e := store.Do(ctx, Request{Action: "search", Query: in.Query})
		return nil, out, e
	})
	mcp.AddTool(server, &mcp.Tool{Name: "memory_read", Description: "Read a document and its revision, or list documents in a memory. Omit id for your own memory."}, func(ctx context.Context, _ *mcp.CallToolRequest, in readInput) (*mcp.CallToolResult, Reply, error) {
		out, e := store.Do(ctx, Request{Action: "read", ID: in.ID, Document: in.Document})
		return nil, out, e
	})
	mcp.AddTool(server, &mcp.Tool{Name: "memory_update", Description: "Save Markdown to your own session memory. Read the document first and supply its revision when replacing it; omit revision only for a new document."}, func(ctx context.Context, _ *mcp.CallToolRequest, in updateInput) (*mcp.CallToolResult, Reply, error) {
		out, e := store.Do(ctx, Request{Action: "update", Document: in.Document, Content: in.Content, Revision: in.Revision})
		return nil, out, e
	})
	return server
}
