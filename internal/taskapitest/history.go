package taskapitest

import (
	"bytes"
	"context"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"strings"
	"testing"
	"time"
)

// RunHistory checks authoritative scope, committed operations and stable paging.
func RunHistory(t *testing.T, conn *grpc.ClientConn, f Fixture) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	client := resource.NewTaskServiceClient(conn)
	task, err := client.Add(ctx, resource.TaskAddRequest_builder{Project: f.Project, ClientId: "history-task", Name: "Original title"}.Build())
	if err != nil {
		t.Fatal(err)
	}
	ref := resource.TaskRef_builder{Id: task.GetId(), Project: f.Project}.Build()
	name := "Updated title"
	task, err = client.Patch(ctx, resource.TaskPatchRequest_builder{Ref: ref, DateUpdated: task.GetDateUpdated(), Name: &name}.Build())
	if err != nil {
		t.Fatal(err)
	}
	task, err = client.Claim(ctx, resource.TaskClaimRequest_builder{Ref: ref, DateUpdated: task.GetDateUpdated(), Actor: f.Actors[0]}.Build())
	if err != nil {
		t.Fatal(err)
	}
	waiting := resource.TaskStatus_TASK_STATUS_WAITING
	task, err = client.Patch(ctx, resource.TaskPatchRequest_builder{Ref: ref, DateUpdated: task.GetDateUpdated(), Status: &waiting, Actor: f.Actors[0]}.Build())
	if err != nil {
		t.Fatal(err)
	}
	task, err = client.Release(ctx, resource.TaskClaimRequest_builder{Ref: ref, DateUpdated: task.GetDateUpdated(), Actor: f.Actors[0]}.Build())
	if err != nil {
		t.Fatal(err)
	}
	first, err := client.History(ctx, resource.TaskHistoryRequest_builder{Ref: ref, Size: proto.Int32(2)}.Build())
	if err != nil {
		t.Fatal(err)
	}
	if len(first.GetItems()) != 2 || first.GetNext() == "" {
		t.Fatalf("first page: %v", first)
	}
	// A new write ahead of a cursor must not shift the older continuation.
	desc := "Written after first history page"
	if _, err := client.Patch(ctx, resource.TaskPatchRequest_builder{Ref: ref, DateUpdated: task.GetDateUpdated(), Desc: &desc}.Build()); err != nil {
		t.Fatal(err)
	}
	rows := append([]*resource.Audit{}, first.GetItems()...)
	after := first.GetNext()
	for after != "" {
		page, err := client.History(ctx, resource.TaskHistoryRequest_builder{Ref: ref, Size: proto.Int32(2), After: &after}.Build())
		if err != nil {
			t.Fatal(err)
		}
		rows = append(rows, page.GetItems()...)
		after = page.GetNext()
	}
	operations := []string{"Release", "Patch", "Claim", "Patch", "Add"}
	if len(rows) != len(operations) {
		t.Fatalf("history has %d rows: %v", len(rows), rows)
	}
	seen := map[string]bool{}
	for i, row := range rows {
		if len(row.GetPatch()) != 0 || len(row.GetValue()) != 0 || len(row.GetActorId()) != 0 || len(row.GetTenantId()) != 0 || len(row.GetTraceId()) != 0 {
			t.Fatal("Task history exposed raw audit payload")
		}
		if !bytes.Equal(row.GetObjectId(), task.GetId()) || seen[string(row.GetId())] {
			t.Fatalf("wrong/duplicate subject: %v", row)
		}
		seen[string(row.GetId())] = true
		if !strings.HasSuffix(row.GetAction(), "/"+operations[i]) {
			t.Fatalf("operation %d: got %s want %s", i, row.GetAction(), operations[i])
		}
		if len(row.GetFields()) == 0 {
			t.Fatalf("missing changed fields: %v", row)
		}
	}
	for _, bad := range []*resource.TaskRef{
		resource.TaskRef_builder{Id: task.GetId(), Project: f.OtherProject}.Build(),
		resource.TaskRef_builder{Id: task.GetId()}.Build(),
	} {
		_, err := client.History(ctx, resource.TaskHistoryRequest_builder{Ref: bad}.Build())
		if status.Code(err) != codes.NotFound && status.Code(err) != codes.InvalidArgument {
			t.Fatalf("unscoped history: %v", err)
		}
	}
	alias := resource.TaskRef_builder{Alias: proto.String(task.GetAlias()), Project: f.Project}.Build()
	page, err := client.History(ctx, resource.TaskHistoryRequest_builder{Ref: alias}.Build())
	if err != nil || len(page.GetItems()) != 6 {
		t.Fatalf("alias history: %v %v", page, err)
	}
	if _, err := client.History(ctx, resource.TaskHistoryRequest_builder{Ref: ref, After: proto.String("bad-cursor")}.Build()); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("bad cursor accepted: %v", err)
	}
}
