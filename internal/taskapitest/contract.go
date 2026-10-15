// Package taskapitest exercises the public wire contract against both storage
// adapters. Tests use independent clients, not private store mutation helpers.
package taskapitest

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

type Fixture struct {
	Project, OtherProject *resource.ProjectRef
	Actors                []*resource.TaskActor
	// Restart changes the authoritative live run, without refreshing a catalog.
	Restart func(string)
}

func Run(t *testing.T, conn *grpc.ClientConn, f Fixture) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	client := resource.NewTaskServiceClient(conn)
	selectAll := resource.TaskSelect_builder{All: proto.Bool(true), Project: resource.ProjectSelect_builder{All: proto.Bool(true)}.Build()}.Build()
	ref := func(v *resource.Task) *resource.TaskRef {
		return resource.TaskRef_builder{Id: v.GetId(), Project: f.Project}.Build()
	}
	get := func(v *resource.Task) *resource.Task {
		t.Helper()
		out, err := client.Get(ctx, resource.TaskGetRequest_builder{Ref: ref(v), Select: selectAll}.Build())
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	add := func(key string) *resource.Task {
		t.Helper()
		v, err := client.Add(ctx, resource.TaskAddRequest_builder{Project: f.Project, ClientId: key, Name: key, Labels: resource.TaskLabels_builder{Values: []string{"api"}}.Build()}.Build())
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	wantCode := func(err error, code codes.Code) {
		t.Helper()
		if status.Code(err) != code {
			t.Fatalf("want %s, got %v", code, err)
		}
	}

	// An empty project has a visible initial Watch frame; clients can stop loading.
	watchCtx, stopWatch := context.WithCancel(ctx)
	defer stopWatch()
	watch, err := client.Watch(watchCtx, resource.TaskWatchRequest_builder{Project: f.Project}.Build())
	if err != nil {
		t.Fatal(err)
	}
	frame, err := watch.Recv()
	if err != nil || len(frame.GetItems()) != 0 {
		t.Fatalf("empty snapshot: %v %v", frame, err)
	}
	v := add("shared-work")
	frame, err = watch.Recv()
	if err != nil || len(frame.GetItems()) != 1 || !bytes.Equal(frame.GetItems()[0].GetId(), v.GetId()) {
		t.Fatalf("committed Add: %v %v", frame, err)
	}
	stopWatch()
	if len(v.GetId()) != 16 || v.GetId()[9] != 11 || v.GetStatus() != resource.TaskStatus_TASK_STATUS_TODO {
		t.Fatalf("invalid initial task: %v", v)
	}
	duplicate := add("shared-work")
	if !bytes.Equal(v.GetId(), duplicate.GetId()) {
		t.Fatal("Add retry created duplicate work")
	}
	_, err = client.Get(ctx, resource.TaskGetRequest_builder{Ref: resource.TaskRef_builder{ClientId: proto.String("shared-work"), Project: f.Project}.Build(), Select: selectAll}.Build())
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.List(ctx, &resource.TaskListRequest{})
	wantCode(err, codes.InvalidArgument)
	_, err = client.Get(ctx, resource.TaskGetRequest_builder{Ref: resource.TaskRef_builder{Id: v.GetId(), Project: f.OtherProject}.Build(), Select: selectAll}.Build())
	wantCode(err, codes.NotFound)
	_, err = client.List(ctx, resource.TaskListRequest_builder{Project: f.Project, Filters: []*resource.TaskFilter{resource.TaskFilter_builder{Project: f.OtherProject}.Build()}}.Build())
	wantCode(err, codes.InvalidArgument)
	_, err = client.Patch(ctx, resource.TaskPatchRequest_builder{Ref: ref(v), Name: proto.String("stale")}.Build())
	wantCode(err, codes.InvalidArgument)
	_, err = client.Patch(ctx, resource.TaskPatchRequest_builder{Ref: ref(v), DateUpdatedForce: proto.Bool(true), Name: proto.String("force")}.Build())
	wantCode(err, codes.InvalidArgument)
	_, err = client.Patch(ctx, resource.TaskPatchRequest_builder{Ref: ref(v), DateUpdated: v.GetDateUpdated(), Assignee: proto.String("spoofed")}.Build())
	wantCode(err, codes.InvalidArgument)
	_, err = client.Apply(ctx, resource.TaskApplyRequest_builder{Ref: ref(v)}.Build())
	wantCode(err, codes.PermissionDenied)
	_, err = client.Erase(ctx, ref(v))
	wantCode(err, codes.PermissionDenied)

	// Distinct sessions race using exactly the same observed version.
	start := make(chan struct{})
	var wg sync.WaitGroup
	type result struct {
		value *resource.Task
		err   error
		actor *resource.TaskActor
	}
	results := make(chan result, len(f.Actors))
	for _, a := range f.Actors {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			value, err := resource.NewTaskServiceClient(conn).Claim(ctx, resource.TaskClaimRequest_builder{Ref: ref(v), Actor: a, DateUpdated: v.GetDateUpdated()}.Build())
			results <- result{value, err, a}
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	var winner result
	wins := 0
	for r := range results {
		if r.err == nil {
			winner = r
			wins++
		} else {
			wantCode(r.err, codes.FailedPrecondition)
		}
	}
	if wins != 1 {
		t.Fatalf("expected exactly one claim winner, got %d", wins)
	}
	owned := winner.value
	if owned.GetStatus() != resource.TaskStatus_TASK_STATUS_IN_PROGRESS {
		t.Fatalf("claim did not start work: %v", owned)
	}
	retry, err := client.Claim(ctx, resource.TaskClaimRequest_builder{Ref: ref(v), Actor: winner.actor, DateUpdated: v.GetDateUpdated()}.Build())
	if err != nil || !proto.Equal(retry.GetDateUpdated(), owned.GetDateUpdated()) {
		t.Fatalf("same-run Claim retry: %v %v", retry, err)
	}
	_, err = client.Patch(ctx, resource.TaskPatchRequest_builder{Ref: ref(v), DateUpdated: v.GetDateUpdated(), Name: proto.String("old edit")}.Build())
	wantCode(err, codes.FailedPrecondition)
	_, err = client.Patch(ctx, resource.TaskPatchRequest_builder{Ref: ref(v), DateUpdated: owned.GetDateUpdated(), Status: resource.TaskStatus_TASK_STATUS_DONE.Enum()}.Build())
	wantCode(err, codes.InvalidArgument)
	owned, err = client.Patch(ctx, resource.TaskPatchRequest_builder{Ref: ref(v), DateUpdated: owned.GetDateUpdated(), Actor: winner.actor, Status: resource.TaskStatus_TASK_STATUS_WAITING.Enum(), Labels: resource.TaskLabels_builder{}.Build()}.Build())
	if err != nil {
		t.Fatal(err)
	}
	if len(get(owned).GetLabels().GetValues()) != 0 {
		t.Fatal("empty label set did not clear labels")
	}

	// Filtered subscriptions remove a row when Release changes its owner.
	filteredCtx, stopFiltered := context.WithCancel(ctx)
	defer stopFiltered()
	filtered, err := client.Watch(filteredCtx, resource.TaskWatchRequest_builder{Project: f.Project, Filters: []*resource.TaskFilter{resource.TaskFilter_builder{Assignee: proto.String(owned.GetAssignee())}.Build()}}.Build())
	if err != nil {
		t.Fatal(err)
	}
	frame, err = filtered.Recv()
	if err != nil || len(frame.GetItems()) != 1 {
		t.Fatalf("owned snapshot: %v %v", frame, err)
	}
	released, err := client.Release(ctx, resource.TaskClaimRequest_builder{Ref: ref(owned), DateUpdated: owned.GetDateUpdated(), Actor: winner.actor}.Build())
	if err != nil {
		t.Fatal(err)
	}
	frame, err = filtered.Recv()
	if err != nil || len(frame.GetItems()) != 1 || frame.GetItems()[0].HasValue() {
		t.Fatalf("filtered removal: %v %v", frame, err)
	}
	stopFiltered()
	if released.GetAssignee() != "" || released.GetStatus() != resource.TaskStatus_TASK_STATUS_TODO {
		t.Fatal("Release retained ownership")
	}
	claimed, err := client.Claim(ctx, resource.TaskClaimRequest_builder{Ref: ref(released), DateUpdated: released.GetDateUpdated(), Actor: winner.actor}.Build())
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Release(ctx, resource.TaskClaimRequest_builder{Ref: ref(owned), DateUpdated: owned.GetDateUpdated(), Actor: winner.actor}.Build())
	wantCode(err, codes.FailedPrecondition)
	if get(claimed).GetAssignee() == "" {
		t.Fatal("old Release cleared a new claim")
	}

	for i := 0; i < 5; i++ {
		add(fmt.Sprintf("page-%d", i))
	}
	after := ""
	ids := map[string]bool{}
	for {
		page, err := client.List(ctx, resource.TaskListRequest_builder{Project: f.Project, Size: 2, After: after}.Build())
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range page.GetItems() {
			if ids[string(item.GetId())] {
				t.Fatal("duplicate row across pages")
			}
			ids[string(item.GetId())] = true
		}
		after = page.GetNext()
		if after == "" {
			break
		}
	}
	if len(ids) != 6 {
		t.Fatalf("pagination lost tasks: %d", len(ids))
	}
	_, err = client.List(ctx, resource.TaskListRequest_builder{Project: f.Project, After: "broken"}.Build())
	wantCode(err, codes.InvalidArgument)

	if f.Restart != nil {
		f.Restart(claimed.GetAssignee())
		_, err = client.Claim(ctx, resource.TaskClaimRequest_builder{Ref: ref(claimed), Actor: winner.actor, DateUpdated: claimed.GetDateUpdated()}.Build())
		wantCode(err, codes.FailedPrecondition)
		_, err = client.Patch(ctx, resource.TaskPatchRequest_builder{Ref: ref(claimed), Actor: winner.actor, DateUpdated: claimed.GetDateUpdated(), Status: resource.TaskStatus_TASK_STATUS_DONE.Enum()}.Build())
		wantCode(err, codes.FailedPrecondition)
		// An explicit release still recovers abandoned work using its stored owner.
		_, err = client.Release(ctx, resource.TaskClaimRequest_builder{Ref: ref(claimed), Actor: winner.actor, DateUpdated: claimed.GetDateUpdated()}.Build())
		if err != nil {
			t.Fatal(err)
		}
	}
}
