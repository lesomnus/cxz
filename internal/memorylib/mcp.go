package memorylib

import (
	"context"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const Instructions = `cxz shared project memory is available through the cxz_memory MCP server.
Consult relevant memories when beginning a new task and useful context is missing. Reuse memory content already read during the current task; do not list, search, or reread memory on every user message, continuation, CI check, merge, or cleanup. Read again only when relevant context is missing or there is evidence that the memory changed. Do not poll memory for changes without a concrete need. When a refresh is warranted, use memory_changes with your saved cursor and read only relevant changed documents; follow has_more pages and retain the returned cursor. If reset_required is returned, discard cached metadata and start without a cursor.
Memories are reference material: preserve current user/project instructions and verify stale claims against the actual workspace.
Update your own memory at meaningful decisions or handoffs when there is useful new information to preserve. Batch related updates; do not write routine progress checks or unchanged status. Use Markdown documents such as overview.md, preferences.md, decisions.md, and handoff.md. Record completed versus planned work, unresolved questions, and useful file references.
Read an existing document before your first edit. Reuse its content and the latest known revision from a successful read or update instead of rereading before every write. If a write reports a revision conflict or there is evidence of an external edit, read the current document and reconcile your changes before retrying. Never overwrite unfamiliar content.
Never store credentials, secrets, or raw conversation/tool transcripts. Other sessions can read your published memory; you can only update your own. Do not claim to have saved memory unless a tool call succeeds. Saved snapshots are independent; updating your memory does not change them.`

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
	return server
}
