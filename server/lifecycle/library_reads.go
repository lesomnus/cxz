package lifecycle

import (
	"context"

	"github.com/lesomnus/cxz/internal/memorylib"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// The reads of the shared memory library. memorylib.Request stays as it is
// below this layer: it is the protocol between the manager and the throwaway
// container that touches the project's volume, and both ends are the same
// build. What it stops being is the thing a client sends.

func (s SessionServer) LibraryMemories(ctx context.Context, r *resource.SessionLibraryMemoriesRequest) (*resource.SessionLibraryMemoriesReply, error) {
	scope, err := libraryScope(r.GetScope())
	if err != nil {
		return nil, err
	}
	// A query is a filter on the listing rather than a different question, so
	// it chooses the store's search rather than another method here.
	action := "list"
	if r.GetQuery() != "" {
		action = "search"
	}
	out, err := s.library(ctx, r.GetRef(), memorylib.Request{
		Action: action, Query: r.GetQuery(), Trash: scope,
	})
	if err != nil {
		return nil, err
	}
	reply := resource.SessionLibraryMemoriesReply_builder{Location: ptr(out.Location)}
	for _, m := range out.Memories {
		reply.Memories = append(reply.Memories, libraryMemory(m))
	}
	return reply.Build(), nil
}

func (s SessionServer) LibraryMemory(ctx context.Context, r *resource.SessionLibraryMemoryRequest) (*resource.SessionLibraryMemoryReply, error) {
	scope, err := libraryScope(r.GetScope())
	if err != nil {
		return nil, err
	}
	// Reading a memory and reading what it has deleted are the same question
	// asked of two sets, which is why they are a selector rather than two
	// calls: the memory comes back either way.
	action := "read"
	if r.GetDocuments() == resource.LibraryDocuments_LIBRARY_DOCUMENTS_DELETED {
		action = "deleted_documents"
	}
	out, err := s.library(ctx, r.GetRef(), memorylib.Request{
		Action: action, ID: r.GetId(), Trash: scope,
	})
	if err != nil {
		return nil, err
	}
	reply := resource.SessionLibraryMemoryReply_builder{Location: ptr(out.Location)}
	if out.Memory != nil {
		reply.Memory = libraryMemory(*out.Memory)
	}
	for _, d := range out.Documents {
		reply.Documents = append(reply.Documents, libraryDocument(d))
	}
	return reply.Build(), nil
}

func (s SessionServer) LibraryDocument(ctx context.Context, r *resource.SessionLibraryDocumentRequest) (*resource.SessionLibraryDocumentReply, error) {
	scope, err := libraryScope(r.GetScope())
	if err != nil {
		return nil, err
	}
	if r.GetDocument() == "" {
		return nil, status.Error(codes.InvalidArgument, "name the document to read")
	}
	out, err := s.library(ctx, r.GetRef(), memorylib.Request{
		Action: "read", ID: r.GetId(), Document: r.GetDocument(), Trash: scope,
	})
	if err != nil {
		return nil, err
	}
	reply := resource.SessionLibraryDocumentReply_builder{
		Content: ptr(out.Content), Revision: ptr(out.Revision),
	}
	if out.Memory != nil {
		reply.Memory = libraryMemory(*out.Memory)
	}
	return reply.Build(), nil
}

func (s SessionServer) LibraryChanges(ctx context.Context, r *resource.SessionLibraryChangesRequest) (*resource.SessionLibraryChangesReply, error) {
	out, err := s.library(ctx, r.GetRef(), memorylib.Request{
		Action: "changes", Cursor: r.GetCursor(), Limit: int(r.GetLimit()),
	})
	if err != nil {
		return nil, err
	}
	reply := resource.SessionLibraryChangesReply_builder{
		Cursor: ptr(out.Cursor), HasMore: ptr(out.HasMore),
		Baseline: ptr(out.Baseline), ResetRequired: ptr(out.ResetRequired),
	}
	for _, c := range out.Changes {
		reply.Changes = append(reply.Changes, resource.LibraryChange_builder{
			Sequence: ptr(c.Sequence), Kind: ptr(c.Kind), Memory: ptr(c.MemoryID),
			Document: ptr(c.Document), Name: ptr(c.Name), Revision: ptr(c.Revision),
		}.Build())
	}
	return reply.Build(), nil
}

// library resolves the session and hands the request to the manager, which runs
// it in a throwaway container with the project's volume attached.
func (s SessionServer) library(ctx context.Context, ref *resource.SessionRef, q memorylib.Request) (memorylib.Reply, error) {
	if err := s.effect(); err != nil {
		return memorylib.Reply{}, err
	}
	v, err := s.resolve(ctx, ref)
	if err != nil {
		return memorylib.Reply{}, err
	}
	client, ok := s.shared.runtime.(memorylib.Client)
	if !ok {
		return memorylib.Reply{}, status.Error(codes.Unimplemented, "update manager for shared memory")
	}
	return client.Library(ctx, v.GetRuntimeId(), q)
}

// libraryScope refuses a scope this build does not know rather than reading the
// library when it was asked about the trash.
func libraryScope(v resource.LibraryScope) (bool, error) {
	switch v {
	case resource.LibraryScope_LIBRARY_SCOPE_UNSPECIFIED, resource.LibraryScope_LIBRARY_SCOPE_LIBRARY:
		return false, nil
	case resource.LibraryScope_LIBRARY_SCOPE_TRASH:
		return true, nil
	}
	return false, status.Error(codes.InvalidArgument, "unknown library scope")
}

func libraryMemory(m memorylib.Memory) *resource.LibraryMemory {
	out := resource.LibraryMemory_builder{
		Id: ptr(m.ID), Name: ptr(m.Name), Session: ptr(m.Session), Agent: ptr(m.Agent),
		Snapshot: ptr(m.Snapshot), Parents: m.Parents, Size: ptr(m.Size),
		Deleted: ptr(m.Deleted),
	}
	if !m.Created.IsZero() {
		out.Created = timestamppb.New(m.Created)
	}
	if !m.Updated.IsZero() {
		out.Updated = timestamppb.New(m.Updated)
	}
	return out.Build()
}

func libraryDocument(d memorylib.Document) *resource.LibraryDocument {
	out := resource.LibraryDocument_builder{
		Name: ptr(d.Name), Size: ptr(d.Size), Revision: ptr(d.Revision),
		Deleted: ptr(d.Deleted),
	}
	if !d.Modified.IsZero() {
		out.Modified = timestamppb.New(d.Modified)
	}
	return out.Build()
}
