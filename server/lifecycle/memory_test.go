package lifecycle

import (
	"github.com/lesomnus/cxz/internal/projectid"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	"strings"
	"testing"
	"time"
)

func TestMemoryVersionsDiffAndMetadata(t *testing.T) {
	f, stack, db := taskStack(t)
	ctx := t.Context()
	service := stack.Memory()
	initial := strings.Repeat("Unchanged contextual paragraph.\n", 100) + "old decision\n"
	m, err := service.Add(ctx, resource.MemoryAddRequest_builder{Project: projectRef(f.p.Id), Name: "decisions.md", Content: initial, ClientId: "first", OwnerSession: f.sessions[0].Id, Labels: map[string]string{"topic": "design"}}.Build())
	if err != nil {
		t.Fatal(err)
	}
	if m.GetRevision() != 1 {
		t.Fatal(m)
	}
	next, err := service.Patch(ctx, resource.MemoryPatchRequest_builder{Ref: m.Ref(), Content: ptr(strings.Replace(initial, "old decision", "new decision", 1)), BaseRevision: ptr(uint64(1)), DateUpdated: m.GetDateUpdated()}.Build())
	if err != nil {
		t.Fatal(err)
	}
	diff, err := service.Read(ctx, resource.MemoryReadRequest_builder{Ref: m.Ref(), BaseRevision: 1, Format: resource.MemoryReadFormat_MEMORY_READ_FORMAT_AUTO}.Build())
	if err != nil {
		t.Fatal(err)
	}
	if diff.GetFormat() != resource.MemoryReadFormat_MEMORY_READ_FORMAT_DIFF || !strings.Contains(diff.GetDiff(), "-old decision") || !strings.Contains(diff.GetDiff(), "+new decision") || diff.GetContent() != "" || diff.GetMemory().GetContent() != "" {
		t.Fatal(diff)
	}
	_, err = service.Patch(ctx, resource.MemoryPatchRequest_builder{Ref: m.Ref(), Content: ptr("stale"), BaseRevision: ptr(uint64(1)), DateUpdated: m.GetDateUpdated()}.Build())
	if status.Code(err) != codes.Aborted {
		t.Fatal(err)
	}
	renamed, err := service.Patch(ctx, resource.MemoryPatchRequest_builder{Ref: m.Ref(), Name: ptr("renamed.md"), DateUpdated: next.GetDateUpdated(), ClearLabels: true}.Build())
	if err != nil {
		t.Fatal(err)
	}
	if renamed.GetRevision() != 2 || len(renamed.GetLabels()) != 0 {
		t.Fatal(renamed)
	}
	latest, err := service.Patch(ctx, resource.MemoryPatchRequest_builder{Ref: m.Ref(), Content: ptr(initial), BaseRevision: ptr(uint64(2)), DateUpdated: renamed.GetDateUpdated()}.Build())
	if err != nil {
		t.Fatal(err)
	}
	if latest.GetRevision() != 3 {
		t.Fatal(latest)
	}
	versions, err := service.Versions(ctx, resource.MemoryVersionsRequest_builder{Ref: m.Ref()}.Build())
	if err != nil || len(versions.GetVersions()) != 3 {
		t.Fatal(versions, err)
	}
	if versions.GetVersions()[0].GetContentHash() != versions.GetVersions()[2].GetContentHash() {
		t.Fatal("A-B-A contents lost their independent versions")
	}
	old, err := service.Read(ctx, resource.MemoryReadRequest_builder{Ref: m.Ref(), Revision: 1}.Build())
	if err != nil || old.GetContent() != initial {
		t.Fatal(old, err)
	}
	fallback, err := service.Read(ctx, resource.MemoryReadRequest_builder{Ref: m.Ref(), BaseRevision: 999, Format: resource.MemoryReadFormat_MEMORY_READ_FORMAT_AUTO}.Build())
	if err != nil || fallback.GetContent() != initial || fallback.GetFallbackReason() == "" {
		t.Fatal(fallback, err)
	}
	var count int
	if err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit WHERE object_id=? AND (CAST(patch AS TEXT) LIKE ? OR CAST(value AS TEXT) LIKE ?)`, projectid.String(m.GetId()), "%Unchanged contextual%", "%Unchanged contextual%").Scan(&count); err != nil || count != 0 {
		t.Fatal("body duplicated in audit", count, err)
	}
}
func TestMemoryScopeTrashAndChanges(t *testing.T) {
	f, stack, _ := taskStack(t)
	ctx := t.Context()
	s := stack.Memory()
	base, err := s.Changes(ctx, &resource.MemoryChangesRequest{})
	if err != nil {
		t.Fatal(err)
	}
	m, err := s.Add(ctx, resource.MemoryAddRequest_builder{Project: projectRef(f.p.Id), Name: "one.md", Content: "first", OwnerSession: f.sessions[0].Id, Labels: map[string]string{"foo": "bar", "baz": "qux"}}.Build())
	if err != nil {
		t.Fatal(err)
	}
	list, err := s.List(ctx, resource.MemoryListRequest_builder{Project: projectRef(f.p.Id), Filters: []*resource.MemoryFilter{resource.MemoryFilter_builder{Labels: map[string]string{"foo": "bar", "baz": "qux"}}.Build()}}.Build())
	if err != nil || len(list.GetItems()) != 1 || list.GetItems()[0].GetContent() != "" {
		t.Fatal(list, err)
	}
	own, err := s.List(ctx, resource.MemoryListRequest_builder{Session: sessionRef(f.sessions[0].Id)}.Build())
	if err != nil || len(own.GetItems()) != 1 {
		t.Fatal(own, err)
	}
	if _, err = s.Erase(ctx, m.Ref()); err != nil {
		t.Fatal(err)
	}
	active, err := s.List(ctx, &resource.MemoryListRequest{})
	if err != nil || len(active.GetItems()) != 0 {
		t.Fatal(active, err)
	}
	trash, err := s.List(ctx, resource.MemoryListRequest_builder{Scope: resource.MemoryScope_MEMORY_SCOPE_TRASH}.Build())
	if err != nil || len(trash.GetItems()) != 1 {
		t.Fatal(trash, err)
	}
	restored, err := s.Restore(ctx, resource.MemoryRestoreRequest_builder{Ref: m.Ref(), DateUpdated: trash.GetItems()[0].GetDateUpdated()}.Build())
	if err != nil || restored.HasDateDeleted() {
		t.Fatal(restored, err)
	}
	if restored.GetRevision() != 1 {
		t.Fatal("trash changed body revision")
	}
	delta, err := s.Changes(ctx, resource.MemoryChangesRequest_builder{Cursor: base.GetCursor()}.Build())
	if err != nil || len(delta.GetChanges()) != 3 {
		t.Fatal(delta, err)
	}
	layer, _ := resource.Find[Layer](stack)
	_, err = layer.Next().Memory().Patch(ctx, resource.MemoryPatchRequest_builder{Ref: m.Ref(), DateUpdated: restored.GetDateUpdated(), DateDeleted: timestamppb.New(time.Now().Add(-time.Second))}.Build())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Get(ctx, resource.MemoryGetRequest_builder{Ref: m.Ref(), Select: memoryAll()}.Build()); status.Code(err) != codes.NotFound {
		t.Fatal(err)
	}
	if _, err = s.Restore(ctx, resource.MemoryRestoreRequest_builder{Ref: m.Ref(), DateUpdated: restored.GetDateUpdated()}.Build()); status.Code(err) != codes.NotFound {
		t.Fatal(err)
	}
	if _, err = s.Patch(ctx, resource.MemoryPatchRequest_builder{Ref: m.Ref(), Name: ptr("expired"), DateUpdated: restored.GetDateUpdated()}.Build()); status.Code(err) != codes.NotFound {
		t.Fatal(err)
	}
}
func TestMemoryVersionFailureRollsBackResourceAndAudit(t *testing.T) {
	f, stack, db := taskStack(t)
	ctx := t.Context()
	if _, err := db.ExecContext(ctx, `DROP TABLE memory_version`); err != nil {
		t.Fatal(err)
	}
	_, err := stack.Memory().Add(ctx, resource.MemoryAddRequest_builder{Project: projectRef(f.p.Id), Name: "rollback.md", Content: "must not commit"}.Build())
	if err == nil {
		t.Fatal("version failure accepted")
	}
	var count int
	if err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM memory`).Scan(&count); err != nil || count != 0 {
		t.Fatal(count, err)
	}
	if err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit WHERE domain=18`).Scan(&count); err != nil || count != 0 {
		t.Fatal(count, err)
	}
}
