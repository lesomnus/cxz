package memorylib

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/lesomnus/cxz/internal/conversation"
	"github.com/lesomnus/cxz/internal/transport"
	"github.com/lesomnus/cxz/resource"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func conversationTools(server *mcp.Server, store *Store) {
	c := &conversation.Store{Root: store.stateRoot, Caller: store.Session.ID, Project: store.Session.ProjectID}
	c.Registry = func(ctx context.Context) ([]conversation.Session, error) {
		// Installed projects use the Manager's registry: runtime aliases are separate.
		b, err := os.ReadFile(filepath.Join(filepath.Dir(store.stateRoot), "runtime.json"))
		if err == nil {
			var runtime struct{ Token, ProjectID string }
			if json.Unmarshal(b, &runtime) != nil || runtime.Token == "" || runtime.ProjectID != store.Session.ProjectID {
				return nil, fmt.Errorf("invalid project runtime identity")
			}
			return conversation.RemoteRegistry(ctx, conversation.Socket, runtime.Token, conversation.RegistryRequest{Project: runtime.ProjectID, Caller: store.Session.ID})
		}
		if !os.IsNotExist(err) {
			return nil, err
		}
		conn, err := transport.Dial(store.stateRoot)
		if err != nil {
			return nil, err
		}
		defer conn.Close()
		client := resource.NewSessionServiceClient(conn)
		return conversation.RegistryFromResources(ctx, store.Session.ProjectID, func(ctx context.Context, r *resource.SessionListRequest) (*resource.SessionListResponse, error) {
			return client.List(ctx, r)
		})
	}
	mcp.AddTool(server, &mcp.Tool{Name: "session_lookup", Description: "Resolve a session alias or UUID in this project. Returns canonical UUID, current alias, state, activity and retained history bounds. Use UUID for subsequent calls. User mentions @seal or session seal mean alias seal, without @."}, func(ctx context.Context, _ *mcp.CallToolRequest, in struct {
		Session string `json:"session"`
	}) (*mcp.CallToolResult, conversation.Reply, error) { r, e := c.Lookup(ctx, in.Session); return nil, r, e })
	mcp.AddTool(server, &mcp.Tool{Name: "conversation_search", Description: "Search retained cxz conversation text (or raw vendor events) in a project session by substring/RE2 and time. Returns metadata only: event seq, UTC time and UTF-8 JSON byte size, never bodies. Follow next_cursor; use conversation_read with UUID, seqs and snapshot_seq. bytes are not token counts."}, func(ctx context.Context, _ *mcp.CallToolRequest, in conversation.Query) (*mcp.CallToolResult, conversation.Reply, error) {
		r, e := c.Search(ctx, in)
		return nil, r, e
	})
	mcp.AddTool(server, &mcp.Tool{Name: "conversation_read", Description: "Read retained session events by UUID/alias, seqs or time, optionally surrounding messages or message line ranges. Default inline body budget 32 KiB. output=file exports a fixed JSONL snapshot plus metadata inside your container for rg/jq; returns paths only. Up to 32 exports/64 MiB per caller, oldest pruned on export. raw means recorded vendor events, not provider transcript files or current model context. Never opens secret-file references."}, func(ctx context.Context, _ *mcp.CallToolRequest, in conversation.Query) (*mcp.CallToolResult, conversation.Reply, error) {
		r, e := c.Read(ctx, in)
		return nil, r, e
	})
}
