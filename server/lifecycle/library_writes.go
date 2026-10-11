package lifecycle

import (
	"context"

	"github.com/lesomnus/cxz/internal/memorylib"
	"github.com/lesomnus/cxz/resource"
)

// The writes of the shared memory library. Each one carries only the arguments
// its operation can mean something by: the action string took a scope and a
// memory id whatever was asked, and then refused most of the combinations.

func (s SessionServer) WriteLibraryDocument(ctx context.Context, r *resource.SessionLibraryWriteRequest) (*resource.SessionLibraryDocumentReply, error) {
	out, err := s.library(ctx, r.GetRef(), memorylib.Request{
		Action: "update", Document: r.GetDocument(),
		Content: r.GetContent(), Revision: r.GetRevision(),
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

func (s SessionServer) DeleteLibraryDocument(ctx context.Context, r *resource.SessionLibraryDocumentEditRequest) (*resource.SessionLibraryMemoryReply, error) {
	return s.libraryEdit(ctx, r.GetRef(), memorylib.Request{
		Action: "delete", ID: r.GetId(), Document: r.GetDocument(), Revision: r.GetRevision(),
	})
}

func (s SessionServer) RestoreLibraryDocument(ctx context.Context, r *resource.SessionLibraryDocumentEditRequest) (*resource.SessionLibraryMemoryReply, error) {
	return s.libraryEdit(ctx, r.GetRef(), memorylib.Request{
		Action: "restore", ID: r.GetId(), Document: r.GetDocument(),
	})
}

func (s SessionServer) PurgeLibraryDocument(ctx context.Context, r *resource.SessionLibraryDocumentEditRequest) (*resource.SessionLibraryMemoryReply, error) {
	return s.libraryEdit(ctx, r.GetRef(), memorylib.Request{
		Action: "purge", ID: r.GetId(), Document: r.GetDocument(),
	})
}

func (s SessionServer) SnapshotLibraryMemory(ctx context.Context, r *resource.SessionLibraryCopyRequest) (*resource.SessionLibraryMemoryReply, error) {
	return s.libraryEdit(ctx, r.GetRef(), memorylib.Request{
		Action: "snapshot", ID: r.GetId(), Name: r.GetName(),
	})
}

func (s SessionServer) ForkLibraryMemory(ctx context.Context, r *resource.SessionLibraryCopyRequest) (*resource.SessionLibraryMemoryReply, error) {
	return s.libraryEdit(ctx, r.GetRef(), memorylib.Request{
		Action: "fork", ID: r.GetId(), Name: r.GetName(),
	})
}

func (s SessionServer) MergeLibraryMemories(ctx context.Context, r *resource.SessionLibraryMergeRequest) (*resource.SessionLibraryMemoryReply, error) {
	return s.libraryEdit(ctx, r.GetRef(), memorylib.Request{
		Action: "merge", IDs: r.GetIds(), Name: r.GetName(),
	})
}

func (s SessionServer) RenameLibraryMemory(ctx context.Context, r *resource.SessionLibraryRenameRequest) (*resource.SessionLibraryMemoryReply, error) {
	return s.libraryEdit(ctx, r.GetRef(), memorylib.Request{
		Action: "rename", ID: r.GetId(), Name: r.GetName(),
	})
}

func (s SessionServer) DeleteLibraryMemory(ctx context.Context, r *resource.SessionLibraryMemoryEditRequest) (*resource.SessionLibraryMemoryReply, error) {
	return s.libraryEdit(ctx, r.GetRef(), memorylib.Request{Action: "delete", ID: r.GetId()})
}

func (s SessionServer) RestoreLibraryMemory(ctx context.Context, r *resource.SessionLibraryMemoryEditRequest) (*resource.SessionLibraryMemoryReply, error) {
	return s.libraryEdit(ctx, r.GetRef(), memorylib.Request{Action: "restore", ID: r.GetId()})
}

// Purging a memory requires that it was deleted first, which the store checks
// and this states: there is no scope to get wrong.
func (s SessionServer) PurgeLibraryMemory(ctx context.Context, r *resource.SessionLibraryMemoryEditRequest) (*resource.SessionLibraryMemoryReply, error) {
	return s.libraryEdit(ctx, r.GetRef(), memorylib.Request{
		Action: "purge", ID: r.GetId(), Trash: true,
	})
}

func (s SessionServer) ImportLibraryMemories(ctx context.Context, r *resource.SessionLibraryImportRequest) (*resource.SessionLibraryMemoriesReply, error) {
	out, err := s.library(ctx, r.GetRef(), memorylib.Request{Action: "import"})
	if err != nil {
		return nil, err
	}
	reply := resource.SessionLibraryMemoriesReply_builder{Location: ptr(out.Location)}
	for _, m := range out.Memories {
		reply.Memories = append(reply.Memories, libraryMemory(m))
	}
	return reply.Build(), nil
}

// libraryEdit is the shape every memory-level write shares: the store answers
// with what the library looks like afterwards, or with nothing when there is
// nothing to say beyond success.
func (s SessionServer) libraryEdit(ctx context.Context, ref *resource.SessionRef, q memorylib.Request) (*resource.SessionLibraryMemoryReply, error) {
	out, err := s.library(ctx, ref, q)
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
