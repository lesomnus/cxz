package memorylib

import (
	"context"
	_ "embed"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

//go:embed instructions.txt
var Instructions string

type changesInput struct {
	Cursor string `json:"cursor,omitempty" jsonschema:"opaque cursor from memory_changes; omit for an initial metadata baseline"`
	Limit  int    `json:"limit,omitempty" jsonschema:"page size from 1 to 500; default 100"`
}

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
	Revision string `json:"revision,omitempty" jsonschema:"latest known revision from a successful memory_read or memory_update; omit only for a new document"`
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
	mcp.AddTool(server, &mcp.Tool{Name: "memory_update", Description: "Save Markdown to your own session memory. Read an existing document before the first edit, then reuse its latest known revision from a successful read or update. On revision conflict, reread and reconcile before retrying. Omit revision only for a new document."}, func(ctx context.Context, _ *mcp.CallToolRequest, in updateInput) (*mcp.CallToolResult, Reply, error) {
		out, e := store.Do(ctx, Request{Action: "update", Document: in.Document, Content: in.Content, Revision: in.Revision})
		return nil, out, e
	})
	mcp.AddTool(server, &mcp.Tool{Name: "memory_changes", Description: "List project memory and document metadata changed since a saved cursor, including deletions and host edits. Omit cursor for a baseline. Follow has_more pages. On reset_required, discard cached metadata and start without a cursor. No document bodies or automatic last-read tracking; do not poll routinely."}, func(ctx context.Context, _ *mcp.CallToolRequest, in changesInput) (*mcp.CallToolResult, Reply, error) {
		out, e := store.Do(ctx, Request{Action: "changes", Cursor: in.Cursor, Limit: in.Limit})
		return nil, out, e
	})
	conversationTools(server, store)
	return server
}
