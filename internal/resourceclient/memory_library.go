package resourceclient

import (
	"context"
	"fmt"
	"time"

	"github.com/lesomnus/cxz/internal/memorylib"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// memorylib.Request is a view model here, not a wire format: the library's
// operations are thirteen methods on the resource API now, and this is where
// the TUI's existing shape is adapted onto them -- which is what this package
// is for. Nothing opaque crosses the connection.
func (c *Client) Library(ctx context.Context, id string, q memorylib.Request) (memorylib.Reply, error) {
	ref := sr(id)
	scope := resource.LibraryScope_LIBRARY_SCOPE_LIBRARY
	if q.Trash {
		scope = resource.LibraryScope_LIBRARY_SCOPE_TRASH
	}
	action := q.Action
	if action == "" {
		action = "list"
	}
	switch action {
	case "list", "search":
		v, e := c.sessions.LibraryMemories(ctx, resource.SessionLibraryMemoriesRequest_builder{
			Ref: ref, Scope: &scope, Query: &q.Query,
		}.Build())
		if e != nil {
			return memorylib.Reply{}, e
		}
		out := memorylib.Reply{Location: v.GetLocation()}
		for _, m := range v.GetMemories() {
			out.Memories = append(out.Memories, libraryMemory(m))
		}
		return out, nil
	case "read":
		if q.Document != "" {
			v, e := c.sessions.LibraryDocument(ctx, resource.SessionLibraryDocumentRequest_builder{
				Ref: ref, Id: &q.ID, Document: &q.Document, Scope: &scope,
			}.Build())
			if e != nil {
				return memorylib.Reply{}, e
			}
			out := memorylib.Reply{Content: v.GetContent(), Revision: v.GetRevision()}
			if m := v.GetMemory(); m != nil {
				v := libraryMemory(m)
				out.Memory = &v
				out.Location = ""
			}
			return out, nil
		}
		return c.libraryMemory(ctx, ref, q.ID, scope, resource.LibraryDocuments_LIBRARY_DOCUMENTS_PRESENT)
	case "deleted_documents":
		return c.libraryMemory(ctx, ref, q.ID, scope, resource.LibraryDocuments_LIBRARY_DOCUMENTS_DELETED)
	case "changes":
		limit := int32(q.Limit)
		v, e := c.sessions.LibraryChanges(ctx, resource.SessionLibraryChangesRequest_builder{
			Ref: ref, Cursor: &q.Cursor, Limit: &limit,
		}.Build())
		if e != nil {
			return memorylib.Reply{}, e
		}
		out := memorylib.Reply{
			Cursor: v.GetCursor(), HasMore: v.GetHasMore(),
			Baseline: v.GetBaseline(), ResetRequired: v.GetResetRequired(),
		}
		for _, ch := range v.GetChanges() {
			out.Changes = append(out.Changes, memorylib.Change{
				Sequence: ch.GetSequence(), Kind: ch.GetKind(), MemoryID: ch.GetMemory(),
				Document: ch.GetDocument(), Name: ch.GetName(), Revision: ch.GetRevision(),
			})
		}
		return out, nil
	case "update":
		v, e := c.sessions.WriteLibraryDocument(ctx, resource.SessionLibraryWriteRequest_builder{
			Ref: ref, Document: &q.Document, Content: &q.Content, Revision: &q.Revision,
		}.Build())
		if e != nil {
			return memorylib.Reply{}, e
		}
		return memorylib.Reply{Content: v.GetContent(), Revision: v.GetRevision()}, nil
	case "import":
		v, e := c.sessions.ImportLibraryMemories(ctx, resource.SessionLibraryImportRequest_builder{Ref: ref}.Build())
		if e != nil {
			return memorylib.Reply{}, e
		}
		out := memorylib.Reply{Location: v.GetLocation()}
		for _, m := range v.GetMemories() {
			out.Memories = append(out.Memories, libraryMemory(m))
		}
		return out, nil
	}
	return c.libraryEdit(ctx, ref, action, q)
}

// libraryEdit covers the writes that answer with the library as it is
// afterwards. Which of them a request is decides which arguments it may carry,
// so each one names only its own.
func (c *Client) libraryEdit(ctx context.Context, ref *resource.SessionRef, action string, q memorylib.Request) (memorylib.Reply, error) {
	document := func() (*resource.SessionLibraryMemoryReply, error) {
		r := resource.SessionLibraryDocumentEditRequest_builder{
			Ref: ref, Id: &q.ID, Document: &q.Document, Revision: &q.Revision,
		}.Build()
		switch action {
		case "delete":
			return c.sessions.DeleteLibraryDocument(ctx, r)
		case "restore":
			return c.sessions.RestoreLibraryDocument(ctx, r)
		}
		return c.sessions.PurgeLibraryDocument(ctx, r)
	}
	memory := func() (*resource.SessionLibraryMemoryReply, error) {
		r := resource.SessionLibraryMemoryEditRequest_builder{Ref: ref, Id: &q.ID}.Build()
		switch action {
		case "delete":
			return c.sessions.DeleteLibraryMemory(ctx, r)
		case "restore":
			return c.sessions.RestoreLibraryMemory(ctx, r)
		}
		return c.sessions.PurgeLibraryMemory(ctx, r)
	}
	var v *resource.SessionLibraryMemoryReply
	var e error
	switch action {
	case "delete", "restore", "purge":
		if q.Document != "" {
			v, e = document()
		} else {
			v, e = memory()
		}
	case "snapshot", "fork":
		r := resource.SessionLibraryCopyRequest_builder{Ref: ref, Id: &q.ID, Name: &q.Name}.Build()
		if action == "fork" {
			v, e = c.sessions.ForkLibraryMemory(ctx, r)
		} else {
			v, e = c.sessions.SnapshotLibraryMemory(ctx, r)
		}
	case "merge":
		v, e = c.sessions.MergeLibraryMemories(ctx, resource.SessionLibraryMergeRequest_builder{
			Ref: ref, Ids: q.IDs, Name: &q.Name,
		}.Build())
	case "rename":
		v, e = c.sessions.RenameLibraryMemory(ctx, resource.SessionLibraryRenameRequest_builder{
			Ref: ref, Id: &q.ID, Name: &q.Name,
		}.Build())
	case "init":
		// Creating this session's own memory is what opening the library does;
		// reading it is enough to have it.
		return c.libraryMemory(ctx, ref, "", resource.LibraryScope_LIBRARY_SCOPE_LIBRARY, resource.LibraryDocuments_LIBRARY_DOCUMENTS_PRESENT)
	default:
		return memorylib.Reply{}, fmt.Errorf("unknown memory operation")
	}
	if e != nil {
		return memorylib.Reply{}, e
	}
	return libraryReply(v), nil
}

func (c *Client) libraryMemory(ctx context.Context, ref *resource.SessionRef, id string, scope resource.LibraryScope, documents resource.LibraryDocuments) (memorylib.Reply, error) {
	v, e := c.sessions.LibraryMemory(ctx, resource.SessionLibraryMemoryRequest_builder{
		Ref: ref, Id: &id, Scope: &scope, Documents: &documents,
	}.Build())
	if e != nil {
		return memorylib.Reply{}, e
	}
	return libraryReply(v), nil
}

func libraryReply(v *resource.SessionLibraryMemoryReply) memorylib.Reply {
	out := memorylib.Reply{Location: v.GetLocation()}
	if m := v.GetMemory(); m != nil {
		memory := libraryMemory(m)
		out.Memory = &memory
	}
	for _, d := range v.GetDocuments() {
		out.Documents = append(out.Documents, memorylib.Document{
			Name: d.GetName(), Size: d.GetSize(), Revision: d.GetRevision(),
			Deleted: d.GetDeleted(), Modified: stamp(d.GetModified()),
		})
	}
	return out
}

func libraryMemory(m *resource.LibraryMemory) memorylib.Memory {
	return memorylib.Memory{
		Format: 1, ID: m.GetId(), Name: m.GetName(), Session: m.GetSession(),
		Agent: m.GetAgent(), Snapshot: m.GetSnapshot(), Parents: m.GetParents(),
		Size: m.GetSize(), Deleted: m.GetDeleted(),
		Created: stamp(m.GetCreated()), Updated: stamp(m.GetUpdated()),
	}
}

// stamp keeps an absent time absent. A nil Timestamp answers AsTime with the
// epoch, which would print as 1970 beside a memory that has no such date.
func stamp(v *timestamppb.Timestamp) time.Time {
	if v == nil {
		return time.Time{}
	}
	return v.AsTime()
}
