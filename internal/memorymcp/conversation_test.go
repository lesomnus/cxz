//go:build !windows

package memorymcp

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/lesomnus/cxz/internal/conversation"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/projectid"
	"github.com/lesomnus/cxz/internal/sessionstore"
	"github.com/lesomnus/cxz/internal/transport"
	"github.com/lesomnus/cxz/resource"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"google.golang.org/grpc"
)

type conversationSessions struct {
	resource.UnimplementedSessionServiceServer
	items []*resource.Session
}

func (s *conversationSessions) List(context.Context, *resource.SessionListRequest) (*resource.SessionListResponse, error) {
	return resource.SessionListResponse_builder{Items: s.items}.Build(), nil
}
func TestConversationMCPMetadataThenFile(t *testing.T) {
	root := t.TempDir()
	a := core.Session{ID: core.ID(), ProjectID: "project", Kind: "codex"}
	b := core.Session{ID: core.ID(), ProjectID: "project", Kind: "codex"}
	if err := core.Prepare(root); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(core.Dir(root, b.ID), 0700); err != nil {
		t.Fatal(err)
	}
	dir := core.Dir(root, b.ID)
	if err := core.WriteJSON(filepath.Join(dir, "session.json"), b); err != nil {
		t.Fatal(err)
	}
	db, err := sessionstore.Open(t.Context(), sessionstore.Path(dir), b.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.Append(t.Context(), []core.Event{
		{SessionID: b.ID, Seq: 1, Kind: "assistant", Text: "distinctive response"},
		{SessionID: b.ID, Seq: 2, Kind: "input", Text: "continue"},
		{SessionID: b.ID, Seq: 3, Kind: "tool_result", Text: "tool output"},
		{SessionID: b.ID, Seq: 4, Kind: "assistant", Text: "latest answer"},
	}); err != nil {
		t.Fatal(err)
	}
	id := uuid.New()
	g := grpc.NewServer()
	resource.RegisterSessionServiceServer(g, &conversationSessions{items: []*resource.Session{resource.Session_builder{Id: id[:], Alias: "seal", RuntimeId: b.ID, Agent: "codex", Project: resource.Project_builder{Id: projectid.Bytes("project")}.Build(), Status: resource.SessionStatus_builder{State: "stopped"}.Build()}.Build()}})
	ln, err := net.Listen("unix", transport.Socket(root))
	if err != nil {
		t.Fatal(err)
	}
	go g.Serve(ln)
	defer g.Stop()
	st, ct := mcp.NewInMemoryTransports()
	provider := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	conversationTools(provider, root, a)
	server, err := provider.Connect(t.Context(), st, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	client, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil).Connect(t.Context(), ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	call := func(name string, args map[string]any) conversation.Reply {
		t.Helper()
		result, err := client.CallTool(t.Context(), &mcp.CallToolParams{Name: name, Arguments: args})
		if err != nil || result.IsError {
			t.Fatal(result, err)
		}
		raw, err := json.Marshal(result.StructuredContent)
		if err != nil {
			t.Fatal(err)
		}
		var reply conversation.Reply
		if err = json.Unmarshal(raw, &reply); err != nil {
			t.Fatal(err)
		}
		return reply
	}
	found := call("session_lookup", map[string]any{"session": "seal"})
	if found.ID != id.String() || found.State != "stopped" || found.FirstSeq != 1 || found.SnapshotSeq != 4 {
		t.Fatal(found)
	}
	hits := call("conversation_search", map[string]any{"session": found.ID, "query": "distinctive"})
	if len(hits.Hits) != 1 || len(hits.Bodies) != 0 {
		t.Fatal(hits)
	}
	recent := call("conversation_read", map[string]any{"session": found.ID, "tail": 2})
	if len(recent.Bodies) != 2 || recent.Bodies[0].Text != "continue" || recent.Bodies[1].Text != "latest answer" {
		t.Fatal(recent)
	}
	file := call("conversation_read", map[string]any{"session": found.ID, "seqs": []uint64{1}, "snapshot_seq": hits.SnapshotSeq, "output": "file"})
	if file.Path == "" || len(file.Bodies) != 0 {
		t.Fatal(file)
	}
	raw, err := os.ReadFile(file.Path)
	if err != nil {
		t.Fatal(err)
	}
	var event conversation.Event
	if err = json.Unmarshal(raw, &event); err != nil || event.Text != "distinctive response" {
		t.Fatal(event, err)
	}
	if filepath.Dir(file.Path) != filepath.Join(core.Dir(root, a.ID), "conversation-exports") {
		t.Fatal("export not owned by caller")
	}
}
