package tui

import (
	"context"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
	"strings"
	"testing"
)

type flatMemoryRPC struct {
	resource.MemoryServiceClient
	row *resource.Memory
}

func (c *flatMemoryRPC) List(_ context.Context, r *resource.MemoryListRequest, _ ...grpc.CallOption) (*resource.MemoryListResponse, error) {
	if c.row.HasDateDeleted() != (r.GetScope() == resource.MemoryScope_MEMORY_SCOPE_TRASH) {
		return &resource.MemoryListResponse{}, nil
	}
	copy := proto.Clone(c.row).(*resource.Memory)
	copy.SetContent("")
	return resource.MemoryListResponse_builder{Items: []*resource.Memory{copy}}.Build(), nil
}
func (c *flatMemoryRPC) Get(context.Context, *resource.MemoryGetRequest, ...grpc.CallOption) (*resource.Memory, error) {
	return proto.Clone(c.row).(*resource.Memory), nil
}
func (c *flatMemoryRPC) Erase(context.Context, *resource.MemoryRef, ...grpc.CallOption) (*resource.MemoryEraseResponse, error) {
	c.row.SetDateDeleted(timestamppb.Now())
	return resource.MemoryEraseResponse_builder{Erased: proto.Bool(true)}.Build(), nil
}
func (c *flatMemoryRPC) Restore(context.Context, *resource.MemoryRestoreRequest, ...grpc.CallOption) (*resource.Memory, error) {
	c.row.ClearDateDeleted()
	return proto.Clone(c.row).(*resource.Memory), nil
}

type flatMemoryClient struct {
	api.SessionsClient
	rpc *flatMemoryRPC
}

func (c *flatMemoryClient) MemoryResources(_ context.Context, project string) (resource.MemoryServiceClient, string, error) {
	return c.rpc, project, nil
}
func applyMemory(t *testing.T, m *model, cmd tea.Cmd) {
	t.Helper()
	if cmd == nil {
		return
	}
	_, next := m.Update(cmd())
	if next != nil {
		applyMemory(t, m, next)
	}
}
func flatMemoryModel(t *testing.T) (*model, *flatMemoryRPC) {
	t.Helper()
	m := conversationModel()
	rpc := &flatMemoryRPC{row: resource.Memory_builder{Id: []byte("1234567890123456"), Name: "handoff.md", Content: "PR still pending", Revision: 1, DateCreated: timestamppb.Now(), DateUpdated: timestamppb.Now()}.Build()}
	m.client = &flatMemoryClient{m.client, rpc}
	applyMemory(t, m, m.openMemory(m.current()))
	return m, rpc
}
func TestFlatMemoryBrowseDeleteRestore(t *testing.T) {
	m, _ := flatMemoryModel(t)
	if m.library == nil || len(m.library.rows) != 1 {
		t.Fatal("list missing")
	}
	applyMemory(t, m, m.libraryKey(tea.KeyMsg{Type: tea.KeyEnter}))
	if !strings.Contains(m.libraryScreen(), "PR still pending") {
		t.Fatal(m.libraryScreen())
	}
	m.libraryKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	if m.library.dialog != "delete" {
		t.Fatal("confirmation missing")
	}
	applyMemory(t, m, m.libraryKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")}))
	if len(m.library.rows) != 0 {
		t.Fatal("active memory retained")
	}
	applyMemory(t, m, m.libraryKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")}))
	if len(m.library.rows) != 1 {
		t.Fatal("trash missing")
	}
	applyMemory(t, m, m.libraryKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("u")}))
	if len(m.library.rows) != 0 {
		t.Fatal("restore did not remove trash entry")
	}
}
func TestFlatMemoryLateReplyCannotReplaceNewPage(t *testing.T) {
	m, _ := flatMemoryModel(t)
	old := m.library
	cmd := m.loadLibrary()
	m.library = &libraryPage{}
	m.receiveLibrary(cmd().(libraryResult))
	if m.library == old || len(m.library.rows) != 0 {
		t.Fatal("late reply replaced page")
	}
}
