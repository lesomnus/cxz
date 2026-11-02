//go:build !windows

package memorymcp

import (
	"context"
	"encoding/json"
	"github.com/lesomnus/cxz/internal/projectid"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/lesomnus/cxz/internal/conversation"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/journal"
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
	core.WriteJSON(filepath.Join(core.Dir(root, b.ID), "session.json"), b)
	log, err := journal.Open(filepath.Join(core.Dir(root, b.ID), "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = log.Append(core.Event{SessionID: b.ID, Kind: "assistant", Text: "distinctive response"})
	log.Close()
	if err != nil {
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
	if found.ID != id.String() || found.State != "stopped" {
		t.Fatal(found)
	}
	hits := call("conversation_search", map[string]any{"session": found.ID, "query": "distinctive"})
	if len(hits.Hits) != 1 || len(hits.Bodies) != 0 {
		t.Fatal(hits)
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
