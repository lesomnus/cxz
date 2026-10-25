package lifecycle

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/lesomnus/cxz/internal/taskapi"
	"github.com/lesomnus/cxz/internal/taskmcp"
	"github.com/lesomnus/cxz/resource"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func TestTaskLinkContract(t *testing.T) {
	f, stack, _ := taskStack(t)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	conn := taskConn(t, stack)
	tasks := resource.NewTaskServiceClient(conn)
	links := resource.NewTaskLinkServiceClient(conn)
	project := projectRef(f.p.Id)

	add := func(key string) *resource.Task {
		t.Helper()
		v, err := tasks.Add(ctx, resource.TaskAddRequest_builder{Project: project, ClientId: key, Name: key, Alias: key}.Build())
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	ref := func(v *resource.Task) *resource.TaskRef {
		return resource.TaskRef_builder{Id: v.GetId(), Project: project}.Build()
	}
	link := func(blocker, blocked *resource.Task) (*resource.TaskLink, error) {
		return links.Add(ctx, resource.TaskLinkAddRequest_builder{
			Project: project, Source: ref(blocker), Target: ref(blocked),
			Kind: resource.TaskLinkKind_TASK_LINK_KIND_BLOCKS,
		}.Build())
	}
	wantCode := func(err error, code codes.Code) {
		t.Helper()
		if status.Code(err) != code {
			t.Fatalf("want %v, got %v", code, err)
		}
	}

	a, b, c := add("task-a"), add("task-b"), add("task-c")

	// A link reads the same from both ends, because only one row is stored.
	first, err := link(a, b)
	if err != nil {
		t.Fatal(err)
	}
	for _, filter := range []*resource.TaskLinkFilter{
		resource.TaskLinkFilter_builder{Source: ref(a)}.Build(),
		resource.TaskLinkFilter_builder{Target: ref(b)}.Build(),
	} {
		page, err := links.List(ctx, resource.TaskLinkListRequest_builder{Project: project, Filters: []*resource.TaskLinkFilter{filter}}.Build())
		if err != nil || len(page.GetItems()) != 1 || !bytes.Equal(page.GetItems()[0].GetId(), first.GetId()) {
			t.Fatalf("one link read from one end: %v %v", page, err)
		}
	}

	// The same link again is a retry, not a second row.
	again, err := link(a, b)
	if err != nil || string(again.GetId()) != string(first.GetId()) {
		t.Fatalf("adding the same link twice: %v %v", again, err)
	}

	// Cycles: directly, and through another task.
	_, err = link(b, a)
	wantCode(err, codes.InvalidArgument)
	if _, err := link(b, c); err != nil {
		t.Fatal(err)
	}
	_, err = link(c, a)
	wantCode(err, codes.InvalidArgument)
	_, err = link(a, a)
	wantCode(err, codes.InvalidArgument)

	// A link joins two tasks of one project, and nothing else about it is
	// editable: every field is immutable, so there is no Patch to make.
	_, err = links.Add(ctx, resource.TaskLinkAddRequest_builder{
		Project: project, Source: ref(a), Target: resource.TaskRef_builder{Id: c.GetId(), Project: projectRef("other-project")}.Build(),
		Kind: resource.TaskLinkKind_TASK_LINK_KIND_BLOCKS,
	}.Build())
	wantCode(err, codes.InvalidArgument)
	_, err = links.Add(ctx, resource.TaskLinkAddRequest_builder{Project: project, Source: ref(a), Target: ref(c), Kind: resource.TaskLinkKind_TASK_LINK_KIND_RELATES}.Build())
	wantCode(err, codes.Unimplemented)
	_, err = links.Add(ctx, resource.TaskLinkAddRequest_builder{Project: project, Source: ref(a), Target: ref(c)}.Build())
	wantCode(err, codes.InvalidArgument)
	_, err = links.Patch(ctx, resource.TaskLinkPatchRequest_builder{Ref: resource.TaskLinkRef_builder{Id: first.GetId(), Project: project}.Build(), DateUpdatedForce: proto.Bool(true)}.Build())
	wantCode(err, codes.PermissionDenied)
	_, err = links.List(ctx, &resource.TaskLinkListRequest{})
	wantCode(err, codes.InvalidArgument)

	// Claim is the one place being blocked is enforced.
	actor := resource.TaskActor_builder{Session: sessionRef(f.sessions[0].Id), RunId: ptr(f.sessions[0].RunId)}.Build()
	claim := func(v *resource.Task) (*resource.Task, error) {
		current, err := tasks.Get(ctx, resource.TaskGetRequest_builder{Ref: ref(v), Select: resource.TaskSelect_builder{All: proto.Bool(true)}.Build()}.Build())
		if err != nil {
			t.Fatal(err)
		}
		return tasks.Claim(ctx, resource.TaskClaimRequest_builder{Ref: ref(v), Actor: actor, DateUpdated: current.GetDateUpdated()}.Build())
	}
	_, err = claim(b)
	wantCode(err, codes.FailedPrecondition)
	// Editing and finishing it stay possible; only starting it does not.
	current, err := tasks.Get(ctx, resource.TaskGetRequest_builder{Ref: ref(b), Select: resource.TaskSelect_builder{All: proto.Bool(true)}.Build()}.Build())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tasks.Patch(ctx, resource.TaskPatchRequest_builder{Ref: ref(b), DateUpdated: current.GetDateUpdated(), Name: proto.String("still editable")}.Build()); err != nil {
		t.Fatal(err)
	}

	// A blocker that is finished stops blocking, and its link stays as a record.
	owned, err := claim(a)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tasks.Patch(ctx, resource.TaskPatchRequest_builder{
		Ref: ref(a), DateUpdated: owned.GetDateUpdated(), Actor: actor, Status: resource.TaskStatus_TASK_STATUS_DONE.Enum(),
	}.Build()); err != nil {
		t.Fatal(err)
	}
	if _, err := claim(b); err != nil {
		t.Fatalf("a done blocker still blocks: %v", err)
	}
	page, err := links.List(ctx, resource.TaskLinkListRequest_builder{Project: project, Filters: []*resource.TaskLinkFilter{resource.TaskLinkFilter_builder{Target: ref(b)}.Build()}}.Build())
	if err != nil || len(page.GetItems()) != 1 {
		t.Fatalf("the link of a finished blocker is a record: %v %v", page, err)
	}

	// Unlinking twice is the same answer as unlinking once.
	erased, err := links.Erase(ctx, resource.TaskLinkRef_builder{Id: first.GetId(), Project: project}.Build())
	if err != nil || !erased.GetErased() {
		t.Fatalf("erase: %v %v", erased, err)
	}
	erased, err = links.Erase(ctx, resource.TaskLinkRef_builder{Id: first.GetId(), Project: project}.Build())
	if err != nil || erased.GetErased() {
		t.Fatalf("erase again: %v %v", erased, err)
	}
	// And the pair is free to be linked again, which the deterministic client_id
	// would otherwise collide with.
	if _, err := link(a, b); err != nil {
		t.Fatalf("relinking an unlinked pair: %v", err)
	}
}

func TestOneTaskCannotCollectLinksWithoutEnd(t *testing.T) {
	f, stack, _ := taskStack(t)
	ctx := t.Context()
	conn := taskConn(t, stack)
	tasks := resource.NewTaskServiceClient(conn)
	links := resource.NewTaskLinkServiceClient(conn)
	project := projectRef(f.p.Id)
	hub, err := tasks.Add(ctx, resource.TaskAddRequest_builder{Project: project, ClientId: "hub", Name: "hub"}.Build())
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i <= taskapi.LinksPerTask; i++ {
		v, err := tasks.Add(ctx, resource.TaskAddRequest_builder{Project: project, ClientId: "blocker-" + string(rune('a'+i%26)) + string(rune('a'+i/26)), Name: "blocker"}.Build())
		if err != nil {
			t.Fatal(err)
		}
		_, err = links.Add(ctx, resource.TaskLinkAddRequest_builder{
			Project: project,
			Source:  resource.TaskRef_builder{Id: v.GetId(), Project: project}.Build(),
			Target:  resource.TaskRef_builder{Id: hub.GetId(), Project: project}.Build(),
			Kind:    resource.TaskLinkKind_TASK_LINK_KIND_BLOCKS,
		}.Build())
		if i < taskapi.LinksPerTask {
			if err != nil {
				t.Fatalf("link %d: %v", i, err)
			}
			continue
		}
		if status.Code(err) != codes.FailedPrecondition {
			t.Fatalf("link %d past the cap: %v", i, err)
		}
	}
}

func TestTheAgentCanSeeAndRemoveWhatBlocksIt(t *testing.T) {
	f, stack, _ := taskStack(t)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	conn := taskConn(t, stack)
	session := f.sessions[0]
	s, err := taskmcp.New(conn, taskmcp.Binding{Project: f.p.Id, Session: session.Id, Run: session.RunId})
	if err != nil {
		t.Fatal(err)
	}
	st, ct := mcp.NewInMemoryTransports()
	ss, err := s.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ss.Close() })
	client, err := mcp.NewClient(&mcp.Implementation{Name: "link-test", Version: "1"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })

	type linkView struct{ Ref, Name, Status string }
	type view struct {
		Alias     string
		Version   string
		Blocked   bool
		BlockedBy []linkView `json:"blocked_by"`
		Blocks    []linkView
	}
	call := func(name string, args map[string]any) (view, error) {
		t.Helper()
		v, err := client.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		if err != nil {
			return view{}, err
		}
		if v.IsError {
			return view{}, errorOf(v)
		}
		raw, err := json.Marshal(v.StructuredContent)
		if err != nil {
			t.Fatal(err)
		}
		var out view
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatal(err)
		}
		return out, nil
	}
	mustCall := func(name string, args map[string]any) view {
		t.Helper()
		out, err := call(name, args)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		return out
	}

	mustCall("task_add", map[string]any{"client_id": "mcp-a", "name": "write the migration", "alias": "mcp-a"})
	mustCall("task_add", map[string]any{"client_id": "mcp-b", "name": "run the migration", "alias": "mcp-b"})
	blocked := mustCall("task_link", map[string]any{"ref": "mcp-b", "blocked_by": "mcp-a"})
	if !blocked.Blocked || len(blocked.BlockedBy) != 1 || blocked.BlockedBy[0].Ref != "mcp-a" {
		t.Fatalf("task_link should report what blocks: %+v", blocked)
	}
	if blocker := mustCall("task_get", map[string]any{"ref": "mcp-a"}); len(blocker.Blocks) != 1 || blocker.Blocks[0].Ref != "mcp-b" {
		t.Fatalf("the other end should read it too: %+v", blocker)
	}
	// What the agent is told when it tries to start blocked work.
	if _, err := call("task_claim", map[string]any{"ref": "mcp-b", "version": blocked.Version}); err == nil {
		t.Fatal("claiming blocked work should be refused")
	} else if !strings.Contains(err.Error(), "blocked by") || !strings.Contains(err.Error(), "mcp-a") {
		t.Fatalf("the refusal should name the blocker: %v", err)
	}
	// A list says which work cannot be started, since a claim will refuse it.
	var page struct {
		Tasks []struct {
			Alias   string
			Blocked bool
		}
	}
	raw, err := client.CallTool(ctx, &mcp.CallToolParams{Name: "task_list", Arguments: map[string]any{}})
	if err != nil || raw.IsError {
		t.Fatal(err)
	}
	body, _ := json.Marshal(raw.StructuredContent)
	if err := json.Unmarshal(body, &page); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range page.Tasks {
		if item.Alias == "mcp-b" {
			found = item.Blocked
		}
		if item.Alias == "mcp-a" && item.Blocked {
			t.Fatal("a blocker is not itself blocked")
		}
	}
	if !found {
		t.Fatalf("the list should mark blocked work: %+v", page.Tasks)
	}
	// And the agent can take the link back off.
	free := mustCall("task_unlink", map[string]any{"ref": "mcp-b", "blocked_by": "mcp-a"})
	if free.Blocked || len(free.BlockedBy) != 0 {
		t.Fatalf("task_unlink should clear it: %+v", free)
	}
	if _, err := call("task_claim", map[string]any{"ref": "mcp-b", "version": free.Version}); err != nil {
		t.Fatalf("claiming unblocked work: %v", err)
	}
}

func errorOf(v *mcp.CallToolResult) error {
	for _, c := range v.Content {
		if text, ok := c.(*mcp.TextContent); ok {
			return status.Error(codes.Unknown, text.Text)
		}
	}
	return status.Error(codes.Unknown, "tool error")
}

// Two sessions adding opposite edges at the same moment is the case the layer
// holds a mutex for: without it both read a graph without the other's edge and
// both pass the cycle check, and the project is left with a loop no later read
// can resolve.
func TestOppositeLinksAtOnceCannotBothWin(t *testing.T) {
	f, stack, _ := taskStack(t)
	ctx := t.Context()
	conn := taskConn(t, stack)
	tasks := resource.NewTaskServiceClient(conn)
	links := resource.NewTaskLinkServiceClient(conn)
	project := projectRef(f.p.Id)
	add := func(key string) *resource.Task {
		t.Helper()
		v, err := tasks.Add(ctx, resource.TaskAddRequest_builder{Project: project, ClientId: key, Name: key, Alias: key}.Build())
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	a, b := add("race-a"), add("race-b")
	ref := func(v *resource.Task) *resource.TaskRef {
		return resource.TaskRef_builder{Id: v.GetId(), Project: project}.Build()
	}
	link := func(blocker, blocked *resource.Task) error {
		_, err := links.Add(ctx, resource.TaskLinkAddRequest_builder{
			Project: project, Source: ref(blocker), Target: ref(blocked),
			Kind: resource.TaskLinkKind_TASK_LINK_KIND_BLOCKS,
		}.Build())
		return err
	}
	start := make(chan struct{})
	errs := make(chan error, 2)
	for _, pair := range [][2]*resource.Task{{a, b}, {b, a}} {
		go func(blocker, blocked *resource.Task) {
			<-start
			errs <- link(blocker, blocked)
		}(pair[0], pair[1])
	}
	close(start)
	ok := 0
	for range 2 {
		if err := <-errs; err == nil {
			ok++
		} else if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("the loser should be refused for the cycle: %v", err)
		}
	}
	if ok != 1 {
		t.Fatalf("exactly one of two opposite links may be stored, got %d", ok)
	}
}
