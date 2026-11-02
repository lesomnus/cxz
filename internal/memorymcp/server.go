// Package memorymcp exposes project/session-bound tools over MemoryService.
package memorymcp

import (
	"context"
	_ "embed"
	"fmt"
	"github.com/google/uuid"
	"github.com/lesomnus/cxz/internal/projectid"
	"github.com/lesomnus/cxz/resource"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
	"strconv"
	"time"
)

//go:embed instructions.txt
var Instructions string

type Binding struct{ Project, Session, Run string }
type tools struct {
	binding  Binding
	memories resource.MemoryServiceClient
	sessions resource.SessionServiceClient
}
type memoryView struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	OwnerSession string            `json:"owner_session"`
	Revision     string            `json:"revision"`
	Version      string            `json:"version"`
	Labels       map[string]string `json:"labels,omitempty"`
}
type readView struct {
	Memory         memoryView `json:"memory"`
	Revision       string     `json:"revision"`
	BaseRevision   string     `json:"base_revision,omitempty"`
	Format         string     `json:"format"`
	Content        string     `json:"content,omitempty"`
	Diff           string     `json:"diff,omitempty"`
	FallbackReason string     `json:"fallback_reason,omitempty"`
}
type listInput struct {
	Mine   bool              `json:"mine,omitempty"`
	Labels map[string]string `json:"labels,omitempty"`
	Query  string            `json:"query,omitempty"`
	Limit  int32             `json:"limit,omitempty"`
	After  string            `json:"after,omitempty"`
}
type listOutput struct {
	Memories []memoryView `json:"memories"`
	Next     string       `json:"next,omitempty"`
}
type readInput struct {
	ID           string `json:"id"`
	Revision     string `json:"revision,omitempty"`
	BaseRevision string `json:"base_revision,omitempty"`
	Format       string `json:"format,omitempty" jsonschema:"full (default), auto or diff; only use diff when the base text remains in context"`
}
type addInput struct {
	ClientID string            `json:"client_id"`
	Name     string            `json:"name"`
	Content  string            `json:"content"`
	Labels   map[string]string `json:"labels,omitempty"`
}
type updateInput struct {
	ID          string            `json:"id"`
	Revision    string            `json:"revision"`
	Content     *string           `json:"content,omitempty"`
	Name        *string           `json:"name,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	ClearLabels bool              `json:"clear_labels,omitempty"`
	Version     string            `json:"version,omitempty" jsonschema:"Version from the last read, required for name/labels changes"`
}
type changesInput struct {
	Cursor string `json:"cursor,omitempty"`
	Limit  int32  `json:"limit,omitempty"`
}
type changeView struct {
	Sequence string `json:"sequence"`
	Kind     string `json:"kind"`
	ID       string `json:"memory_id"`
	Revision string `json:"revision"`
}
type changesOutput struct {
	Changes       []changeView `json:"changes"`
	Cursor        string       `json:"cursor"`
	Baseline      bool         `json:"baseline"`
	HasMore       bool         `json:"has_more"`
	ResetRequired bool         `json:"reset_required"`
}

func New(conn grpc.ClientConnInterface, binding Binding) (*mcp.Server, error) {
	if binding.Project == "" || binding.Session == "" || binding.Run == "" {
		return nil, fmt.Errorf("Memory MCP requires a bound project, session and launch run")
	}
	t := tools{binding, resource.NewMemoryServiceClient(conn), resource.NewSessionServiceClient(conn)}
	s := mcp.NewServer(&mcp.Implementation{Name: "cxz-memory", Version: "2"}, &mcp.ServerOptions{Instructions: Instructions})
	s.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, r mcp.Request) (mcp.Result, error) {
			ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			return next(ctx, method, r)
		}
	})
	mcp.AddTool(s, &mcp.Tool{Name: "memory_list", Description: "List project Memory documents and metadata. Optional labels are ANDed; mine restricts to your owner session."}, t.list)
	mcp.AddTool(s, &mcp.Tool{Name: "memory_search", Description: "Search project Memory names and text; returns metadata, not full bodies."}, t.list)
	mcp.AddTool(s, &mcp.Tool{Name: "memory_read", Description: "Read one Memory by ID, optionally an older revision or contextual diff from a known base. Full text is the default."}, t.read)
	mcp.AddTool(s, &mcp.Tool{Name: "memory_add", Description: "Publish a new Markdown Memory owned by your session. Reuse client_id only to retry the same creation."}, t.add)
	mcp.AddTool(s, &mcp.Tool{Name: "memory_update", Description: "Update your own Memory using its last read revision. Metadata changes require the last read version. Conflicts preserve server data."}, t.update)
	mcp.AddTool(s, &mcp.Tool{Name: "memory_changes", Description: "Discover changed Memory IDs since a saved cursor. Returns metadata, not body history; do not poll routinely."}, t.changes)
	return s, nil
}
func (t tools) project() *resource.ProjectRef { return projectid.Ref(t.binding.Project) }
func (t tools) ref(id string) (*resource.MemoryRef, error) {
	v, e := uuid.Parse(id)
	if e != nil {
		return nil, fmt.Errorf("memory id must be a UUID")
	}
	return resource.MemoryRef_builder{Id: v[:], Project: t.project()}.Build(), nil
}
func (t tools) active(ctx context.Context) (*resource.Session, error) {
	s, e := t.sessions.Get(ctx, resource.SessionGetRequest_builder{Ref: resource.SessionRef_builder{RuntimeId: proto.String(t.binding.Session)}.Build(), Select: resource.SessionSelect_builder{All: proto.Bool(true), Project: resource.ProjectSelect_builder{All: proto.Bool(true)}.Build()}.Build()}.Build())
	if e != nil {
		return nil, e
	}
	if projectid.String(s.GetProject().GetId()) != projectid.Canonical(t.binding.Project) || s.GetStatus().GetRunId() != t.binding.Run {
		return nil, status.Error(codes.PermissionDenied, "Memory MCP launch/project changed; restart the agent")
	}
	return s, nil
}
func view(m *resource.Memory) memoryView {
	version := ""
	if m.GetDateUpdated() != nil {
		version = m.GetDateUpdated().AsTime().Format(time.RFC3339Nano)
	}
	return memoryView{projectid.String(m.GetId()), m.GetName(), m.GetOwnerSession(), strconv.FormatUint(m.GetRevision(), 10), version, m.GetLabels()}
}
func number(s string) (uint64, error) {
	if s == "" {
		return 0, nil
	}
	return strconv.ParseUint(s, 10, 64)
}
func (t tools) list(ctx context.Context, _ *mcp.CallToolRequest, in listInput) (*mcp.CallToolResult, listOutput, error) {
	current, e := t.active(ctx)
	if e != nil {
		return nil, listOutput{}, e
	}
	r := resource.MemoryListRequest_builder{Project: t.project(), Size: in.Limit, After: in.After, Query: in.Query}.Build()
	if in.Mine {
		r.SetSession(resource.SessionRef_builder{Id: current.GetId()}.Build())
	}
	if len(in.Labels) > 0 {
		r.SetFilters([]*resource.MemoryFilter{resource.MemoryFilter_builder{Labels: in.Labels}.Build()})
	}
	p, e := t.memories.List(ctx, r)
	out := listOutput{Memories: []memoryView{}}
	if e != nil {
		return nil, out, e
	}
	for _, m := range p.GetItems() {
		out.Memories = append(out.Memories, view(m))
	}
	out.Next = p.GetNext()
	return nil, out, nil
}
func (t tools) read(ctx context.Context, _ *mcp.CallToolRequest, in readInput) (*mcp.CallToolResult, readView, error) {
	if _, e := t.active(ctx); e != nil {
		return nil, readView{}, e
	}
	ref, e := t.ref(in.ID)
	if e != nil {
		return nil, readView{}, e
	}
	rev, e := number(in.Revision)
	if e != nil {
		return nil, readView{}, e
	}
	base, e := number(in.BaseRevision)
	if e != nil {
		return nil, readView{}, e
	}
	format := resource.MemoryReadFormat_MEMORY_READ_FORMAT_FULL
	switch in.Format {
	case "", "full":
	case "auto":
		format = resource.MemoryReadFormat_MEMORY_READ_FORMAT_AUTO
	case "diff":
		format = resource.MemoryReadFormat_MEMORY_READ_FORMAT_DIFF
	default:
		return nil, readView{}, fmt.Errorf("unknown format")
	}
	p, e := t.memories.Read(ctx, resource.MemoryReadRequest_builder{Ref: ref, Revision: rev, BaseRevision: base, Format: format}.Build())
	if e != nil {
		return nil, readView{}, e
	}
	out := readView{Memory: view(p.GetMemory()), Revision: strconv.FormatUint(p.GetRevision(), 10), Format: "full", Content: p.GetContent(), Diff: p.GetDiff(), FallbackReason: p.GetFallbackReason()}
	if p.GetFormat() == resource.MemoryReadFormat_MEMORY_READ_FORMAT_DIFF {
		out.Format = "diff"
		out.BaseRevision = strconv.FormatUint(p.GetBaseRevision(), 10)
	}
	return nil, out, nil
}
func (t tools) add(ctx context.Context, _ *mcp.CallToolRequest, in addInput) (*mcp.CallToolResult, memoryView, error) {
	current, e := t.active(ctx)
	if e != nil {
		return nil, memoryView{}, e
	}
	if in.ClientID == "" {
		return nil, memoryView{}, fmt.Errorf("client_id is required")
	}
	m, e := t.memories.Add(ctx, resource.MemoryAddRequest_builder{Project: t.project(), OwnerSession: projectid.String(current.GetId()), Name: in.Name, Content: in.Content, Labels: in.Labels, ClientId: in.ClientID}.Build())
	if e != nil {
		return nil, memoryView{}, e
	}
	return nil, view(m), nil
}
func (t tools) update(ctx context.Context, _ *mcp.CallToolRequest, in updateInput) (*mcp.CallToolResult, memoryView, error) {
	current, e := t.active(ctx)
	if e != nil {
		return nil, memoryView{}, e
	}
	ref, e := t.ref(in.ID)
	if e != nil {
		return nil, memoryView{}, e
	}
	m, e := t.memories.Get(ctx, resource.MemoryGetRequest_builder{Ref: ref, Select: resource.MemorySelect_builder{All: proto.Bool(true), Project: resource.ProjectSelect_builder{All: proto.Bool(true)}.Build()}.Build()}.Build())
	if e != nil {
		return nil, memoryView{}, e
	}
	if m.GetOwnerSession() != projectid.String(current.GetId()) {
		return nil, memoryView{}, status.Error(codes.PermissionDenied, "only your own Memory can be updated")
	}
	rev, e := number(in.Revision)
	if e != nil || rev == 0 {
		return nil, memoryView{}, fmt.Errorf("last read revision is required")
	}
	if rev != m.GetRevision() {
		return nil, memoryView{}, status.Error(codes.Aborted, "memory revision changed; read current state")
	}
	at := m.GetDateUpdated()
	if in.Name != nil || len(in.Labels) > 0 || in.ClearLabels {
		parsed, err := time.Parse(time.RFC3339Nano, in.Version)
		if err != nil {
			return nil, memoryView{}, fmt.Errorf("last read version is required for metadata changes")
		}
		at = timestamppb.New(parsed)
	}
	saved, e := t.memories.Patch(ctx, resource.MemoryPatchRequest_builder{Ref: ref, DateUpdated: at, BaseRevision: proto.Uint64(rev), Content: in.Content, Name: in.Name, Labels: in.Labels, ClearLabels: in.ClearLabels}.Build())
	if e != nil {
		return nil, memoryView{}, e
	}
	return nil, view(saved), nil
}
func (t tools) changes(ctx context.Context, _ *mcp.CallToolRequest, in changesInput) (*mcp.CallToolResult, changesOutput, error) {
	if _, e := t.active(ctx); e != nil {
		return nil, changesOutput{}, e
	}
	p, e := t.memories.Changes(ctx, resource.MemoryChangesRequest_builder{Project: t.project(), Cursor: in.Cursor, Size: in.Limit}.Build())
	if e != nil {
		return nil, changesOutput{}, e
	}
	out := changesOutput{Changes: []changeView{}, Cursor: p.GetCursor(), Baseline: p.GetBaseline(), HasMore: p.GetHasMore(), ResetRequired: p.GetResetRequired()}
	for _, c := range p.GetChanges() {
		out.Changes = append(out.Changes, changeView{strconv.FormatUint(c.GetSequence(), 10), c.GetKind(), projectid.String(c.GetMemoryId()), strconv.FormatUint(c.GetRevision(), 10)})
	}
	return nil, out, nil
}
