package lifecycle

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lesomnus/cxz/internal/projectid"
	"github.com/lesomnus/cxz/resource"
	"github.com/protobuf-orm/ent/dialect"
	entsql "github.com/protobuf-orm/ent/dialect/sql"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestMemoryFilteredPaginationBothDirections(t *testing.T) {
	f, stack, _ := taskStack(t)
	ctx := t.Context()
	for i := 0; i < 12; i++ {
		m, err := stack.Memory().Add(ctx, resource.MemoryAddRequest_builder{Project: projectRef(f.p.Id), Name: fmt.Sprintf("entry %02d", i), Content: "find me", Labels: map[string]string{"topic": "design"}}.Build())
		if err != nil {
			t.Fatal(err)
		}
		if i%2 == 0 {
			if _, err = stack.Memory().Erase(ctx, m.Ref()); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, reverse := range []bool{false, true} {
		after := ""
		seen := map[string]bool{}
		for page := 0; ; page++ {
			if page > 10 {
				t.Fatal("pagination did not terminate")
			}
			p, err := stack.Memory().List(ctx, resource.MemoryListRequest_builder{Project: projectRef(f.p.Id), Size: 2, After: after, Reverse: reverse, Query: "find", Filters: []*resource.MemoryFilter{resource.MemoryFilter_builder{Labels: map[string]string{"topic": "design"}}.Build()}}.Build())
			if err != nil {
				t.Fatal(err)
			}
			for _, m := range p.GetItems() {
				id := projectid.String(m.GetId())
				if seen[id] {
					t.Fatal("duplicate", id)
				}
				seen[id] = true
				if m.GetContent() != "" || m.HasDateDeleted() {
					t.Fatal(m)
				}
			}
			after = p.GetNext()
			if after == "" {
				break
			}
		}
		if len(seen) != 6 {
			t.Fatal(reverse, len(seen))
		}
	}
}

func TestMemoryConcurrentBodyWritesAndPurgedOwner(t *testing.T) {
	f, stack, db := taskStack(t)
	ctx := t.Context()
	s := stack.Memory()
	m, err := s.Add(ctx, resource.MemoryAddRequest_builder{Project: projectRef(f.p.Id), OwnerSession: f.sessions[0].Id, Name: "concurrent", Content: "initial"}.Build())
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, e := s.Patch(ctx, resource.MemoryPatchRequest_builder{Ref: m.Ref(), Content: ptr(fmt.Sprint(i)), BaseRevision: ptr(uint64(1)), DateUpdated: m.GetDateUpdated()}.Build())
			results <- e
		}(i)
	}
	wg.Wait()
	close(results)
	wins := 0
	for e := range results {
		if e == nil {
			wins++
		} else if status.Code(e) != codes.Aborted {
			t.Fatal(e)
		}
	}
	if wins != 1 {
		t.Fatal("successful concurrent writes", wins)
	}
	history, err := s.Versions(ctx, resource.MemoryVersionsRequest_builder{Ref: m.Ref()}.Build())
	if err != nil || len(history.GetVersions()) != 2 {
		t.Fatal(history, err)
	}
	latest := history.GetVersions()[0]
	if !latest.HasLinesAdded() || !latest.HasLinesRemoved() || latest.GetLinesAdded() != 1 || latest.GetLinesRemoved() != 1 {
		t.Fatal("concurrent winner lost line stats", latest)
	}
	layer, _ := resource.Find[Layer](stack)
	if _, err = layer.Next().Session().Erase(ctx, resource.SessionById(projectid.Bytes(f.sessions[0].Id))); err != nil {
		t.Fatal(err)
	}
	read, err := s.Read(ctx, resource.MemoryReadRequest_builder{Ref: m.Ref()}.Build())
	if err != nil || read.GetMemory().GetOwnerSession() != m.GetOwnerSession() {
		t.Fatal("published memory lost with owner", read, err)
	}
	expired, err := layer.Next().Memory().Patch(ctx, resource.MemoryPatchRequest_builder{Ref: m.Ref(), DateUpdated: read.GetMemory().GetDateUpdated(), DateDeleted: timestamppb.New(time.Now().Add(-time.Hour))}.Build())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Read(ctx, resource.MemoryReadRequest_builder{Ref: m.Ref()}.Build()); status.Code(err) != codes.NotFound {
		t.Fatal("expired body accessible", err)
	}
	if _, err = layer.RetainMemories(ctx, time.Now()); err != nil {
		t.Fatal(err)
	}
	var n int
	if err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM memory_version WHERE memory_id=?", projectid.String(expired.GetId())).Scan(&n); err != nil || n != 0 {
		t.Fatal("versions survived permanent purge", n, err)
	}
}

func TestMemoryChangesResetAndWritesDuringBaseline(t *testing.T) {
	f, stack, db := taskStack(t)
	ctx := t.Context()
	s := stack.Memory()
	add := func(name string) {
		t.Helper()
		if _, err := s.Add(ctx, resource.MemoryAddRequest_builder{Project: projectRef(f.p.Id), Name: name, Content: name}.Build()); err != nil {
			t.Fatal(err)
		}
	}
	add("first")
	add("second")
	p, err := s.Changes(ctx, resource.MemoryChangesRequest_builder{Size: 1, Project: projectRef(f.p.Id)}.Build())
	if err != nil || !p.GetHasMore() || !p.GetBaseline() {
		t.Fatal(p, err)
	}
	add("during baseline")
	count := 0
	for i := 0; i < 10; i++ {
		next, e := s.Changes(ctx, resource.MemoryChangesRequest_builder{Size: 1, Cursor: p.GetCursor(), Project: projectRef(f.p.Id)}.Build())
		if e != nil {
			t.Fatal(e)
		}
		p = next
		for _, c := range p.GetChanges() {
			if !p.GetBaseline() && c.GetRevision() == 1 {
				count++
			}
		}
		if !p.GetHasMore() {
			break
		}
	}
	if count != 1 {
		t.Fatal("concurrent write missing from delta", count)
	}
	old := p.GetCursor()
	if _, err = db.ExecContext(ctx, "UPDATE memory_meta SET value='100' WHERE key='changes_floor'"); err != nil {
		t.Fatal(err)
	}
	reset, err := s.Changes(ctx, resource.MemoryChangesRequest_builder{Cursor: old, Project: projectRef(f.p.Id)}.Build())
	if err != nil || !reset.GetResetRequired() {
		t.Fatal(reset, err)
	}
	if _, err = db.ExecContext(ctx, "UPDATE memory_meta SET value='0' WHERE key='changes_floor'"); err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, "UPDATE memory_meta SET value='another-database' WHERE key='epoch'"); err != nil {
		t.Fatal(err)
	}
	reset, err = s.Changes(ctx, resource.MemoryChangesRequest_builder{Cursor: old, Project: projectRef(f.p.Id)}.Build())
	if err != nil || !reset.GetResetRequired() {
		t.Fatal(reset, err)
	}
}

func TestMemoryDoesNotDeadlockInBatch(t *testing.T) {
	_, stack, db := taskStack(t)
	layer, _ := resource.Find[Layer](stack)
	driver, tx, err := dialect.BeginTx(t.Context(), entsql.OpenDB(dialect.SQLite, db))
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	bound, err := layer.WithDriver(driver)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	_, err = bound.Memory().Changes(ctx, &resource.MemoryChangesRequest{})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatal(err)
	}
}

func TestRetainedMemoryHelperDoesNotOverwriteRuntimeProjection(t *testing.T) {
	f, stack, db := taskStack(t)
	ctx := t.Context()
	before, err := stack.Project().Get(ctx, resource.ProjectGetRequest_builder{Ref: projectRef(f.p.Id), Select: resource.ProjectSelect_builder{All: ptr(true)}.Build()}.Build())
	if err != nil {
		t.Fatal(err)
	}
	f.p.State = "stopped"
	helper, err := Build(ctx, db, f, nil, Options{RetainedMemory: true})
	if err != nil {
		t.Fatal(err)
	}
	after, err := helper.Project().Get(ctx, resource.ProjectGetRequest_builder{Ref: projectRef(f.p.Id), Select: resource.ProjectSelect_builder{All: ptr(true)}.Build()}.Build())
	if err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(before, after) {
		t.Fatal("helper changed live runtime state", before, after)
	}
}

func TestMemoryDiffPreservesEndOfFileNewlines(t *testing.T) {
	for _, tt := range []struct{ before, after, want string }{
		{"one", "two", "-one\n\\ No newline at end of file\n+two\n\\ No newline at end of file\n"},
		{"one", "one\n", "-one\n\\ No newline at end of file\n+one\n"},
		{"", "one", "@@ -0,0 +1 @@\n+one\n\\ No newline at end of file\n"},
	} {
		t.Run(tt.before+"->"+tt.after, func(t *testing.T) {
			f, stack, _ := taskStack(t)
			ctx := t.Context()
			m, e := stack.Memory().Add(ctx, resource.MemoryAddRequest_builder{Project: projectRef(f.p.Id), Name: "eof", Content: tt.before}.Build())
			if e != nil {
				t.Fatal(e)
			}
			_, e = stack.Memory().Patch(ctx, resource.MemoryPatchRequest_builder{Ref: m.Ref(), Content: ptr(tt.after), BaseRevision: ptr(uint64(1)), DateUpdated: m.GetDateUpdated()}.Build())
			if e != nil {
				t.Fatal(e)
			}
			diff, e := stack.Memory().Read(ctx, resource.MemoryReadRequest_builder{Ref: m.Ref(), BaseRevision: 1, Format: resource.MemoryReadFormat_MEMORY_READ_FORMAT_DIFF}.Build())
			if e != nil || !strings.Contains(diff.GetDiff(), tt.want) {
				t.Fatal(diff, e)
			}
		})
	}
}
