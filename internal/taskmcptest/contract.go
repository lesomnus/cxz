// Package taskmcptest checks the agent-facing MCP contract over real Task RPCs.
package taskmcptest

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lesomnus/cxz/internal/taskmcp"
	"github.com/lesomnus/cxz/resource"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"google.golang.org/grpc"
)

type Fixture struct {
	Actors       []taskmcp.Binding
	OtherProject *resource.ProjectRef
	Restart      func() taskmcp.Binding
}

func connect(t *testing.T, ctx context.Context, conn *grpc.ClientConn, binding taskmcp.Binding) *mcp.ClientSession {
	t.Helper()
	s, err := taskmcp.New(conn, binding)
	if err != nil {
		t.Fatal(err)
	}
	st, ct := mcp.NewInMemoryTransports()
	ss, err := s.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ss.Close() })
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "contract-test", Version: "1"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

type task struct {
	ID, Alias, Name, Status, Version, Assignee string
	Desc                                       *string
	Priority                                   int32
	AssigneeRun                                string `json:"assignee_run"`
	Labels                                     map[string]string
	Source                                     struct{ Session, Run string }
	Blocked                                    bool
	BlockedBy                                  []link `json:"blocked_by"`
	Blocks                                     []link
}

type link struct{ Ref, Name, Status string }

type page struct {
	Tasks []task
	Next  string
}

func call[T any](t *testing.T, ctx context.Context, c *mcp.ClientSession, name string, args map[string]any) T {
	t.Helper()
	v, err := c.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil || v.IsError {
		t.Fatalf("%s: %v %v; content: %+v", name, v, err, v.Content)
	}
	raw, err := json.Marshal(v.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var out T
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func Run(t *testing.T, conn *grpc.ClientConn, f Fixture) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	a, b := connect(t, ctx, conn, f.Actors[0]), connect(t, ctx, conn, f.Actors[1])
	reject := func(c *mcp.ClientSession, name string, args map[string]any) {
		t.Helper()
		v, err := c.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		if err == nil && !v.IsError {
			t.Fatalf("%s accepted forbidden arguments: %v", name, args)
		}
	}
	change := func(v task) map[string]any { return map[string]any{"ref": v.ID, "version": v.Version} }
	listed, err := a.ListTools(ctx, nil)
	if err != nil || len(listed.Tools) != 8 {
		t.Fatalf("tool discovery: %v %v", listed, err)
	}
	for _, tool := range listed.Tools {
		raw, _ := json.Marshal(tool.InputSchema)
		for _, forbidden := range []string{`"project"`, `"session"`, `"run_id"`, `"actor"`, `"assignee"`} {
			if strings.Contains(string(raw), forbidden) {
				t.Fatalf("%s lets the model supply %s", tool.Name, forbidden)
			}
		}
	}
	if p := call[page](t, ctx, a, "task_list", map[string]any{}); len(p.Tasks) != 0 || p.Next != "" {
		t.Fatalf("empty project: %+v", p)
	}
	args := map[string]any{"client_id": "implement", "name": "Implement task tools", "desc": "Detailed instructions", "priority": 2, "labels": map[string]string{"area": "tools"}}
	v := call[task](t, ctx, a, "task_add", args)
	if v.Status != "todo" || v.Assignee != "" || v.Version == "" || v.Source.Session != f.Actors[0].Session || v.Source.Run != f.Actors[0].Run {
		t.Fatalf("creation identity: %+v", v)
	}
	if again := call[task](t, ctx, a, "task_add", args); again.ID != v.ID || again.Version != v.Version {
		t.Fatal("creation retry duplicated the task")
	}
	if other := call[task](t, ctx, b, "task_add", args); other.ID == v.ID {
		t.Fatal("one session reused another session's creation key")
	}
	call[task](t, ctx, a, "task_add", map[string]any{"client_id": "third", "name": "Third task"})
	p := call[page](t, ctx, a, "task_list", map[string]any{"limit": 2})
	if len(p.Tasks) != 2 || p.Next == "" {
		t.Fatalf("first page: %+v", p)
	}
	for _, item := range p.Tasks {
		if item.Desc != nil || len(item.Labels) != 0 || item.Source.Session != "" {
			t.Fatal("task_list returned details")
		}
	}
	if next := call[page](t, ctx, a, "task_list", map[string]any{"limit": 2, "after": p.Next}); len(next.Tasks) != 1 || next.Next != "" {
		t.Fatalf("last page: %+v", next)
	}
	if detail := call[task](t, ctx, b, "task_get", map[string]any{"ref": v.Alias}); detail.ID != v.ID || detail.Desc == nil || *detail.Desc != "Detailed instructions" || detail.Labels["area"] != "tools" {
		t.Fatalf("details by alias: %+v", detail)
	}
	reject(a, "task_list", map[string]any{"mine": true, "unassigned": true})
	reject(a, "task_list", map[string]any{"limit": 101})
	reject(a, "task_claim", map[string]any{"ref": v.ID})
	spoof := change(v)
	spoof["actor"] = map[string]string{"session": f.Actors[1].Session, "run_id": f.Actors[1].Run}
	reject(a, "task_claim", spoof)
	spoof = change(v)
	spoof["project"] = "another-project"
	reject(a, "task_claim", spoof)
	spoof = change(v)
	spoof["status"] = "done"
	reject(a, "task_patch", spoof)
	v = call[task](t, ctx, a, "task_claim", change(v))
	if v.Assignee != f.Actors[0].Session || v.AssigneeRun != f.Actors[0].Run || v.Status != "in_progress" {
		t.Fatalf("bound claim: %+v", v)
	}
	reject(b, "task_claim", change(v))
	reject(b, "task_release", change(v))
	for _, update := range []map[string]any{{"status": "canceled"}, {"name": "Overwrite owned work"}} {
		update["ref"], update["version"] = v.ID, v.Version
		reject(b, "task_patch", update)
	}
	if mine := call[page](t, ctx, a, "task_list", map[string]any{"mine": true}); len(mine.Tasks) != 1 || mine.Tasks[0].ID != v.ID {
		t.Fatalf("mine: %+v", mine)
	}
	if free := call[page](t, ctx, a, "task_list", map[string]any{"unassigned": true}); len(free.Tasks) != 2 {
		t.Fatalf("unassigned: %+v", free)
	}
	stale := change(v)
	update := change(v)
	update["status"], update["desc"], update["priority"], update["clear_labels"] = "waiting", "", 0, true
	v = call[task](t, ctx, a, "task_patch", update)
	if v.Status != "waiting" || v.Desc == nil || *v.Desc != "" || v.Priority != 0 || len(v.Labels) != 0 {
		t.Fatalf("patch presence/clear: %+v", v)
	}
	stale["name"] = "Stale overwrite"
	reject(a, "task_patch", stale)
	v = call[task](t, ctx, a, "task_release", change(v))
	if v.Status != "todo" || v.Assignee != "" {
		t.Fatalf("release: %+v", v)
	}
	v = call[task](t, ctx, b, "task_claim", change(v))
	update = change(v)
	update["status"] = "done"
	v = call[task](t, ctx, b, "task_patch", update)
	if v.Status != "done" || v.Assignee != f.Actors[1].Session {
		t.Fatalf("completion: %+v", v)
	}
	foreign, err := resource.NewTaskServiceClient(conn).Add(ctx, resource.TaskAddRequest_builder{Project: f.OtherProject, ClientId: "foreign", Name: "Other project"}.Build())
	if err != nil {
		t.Fatal(err)
	}
	id, _ := uuid.FromBytes(foreign.GetId())
	reject(a, "task_get", map[string]any{"ref": id.String()})
	if p := call[page](t, ctx, a, "task_list", map[string]any{}); len(p.Tasks) != 3 {
		t.Fatal("another project's task leaked into the list")
	}
	// A connected MCP retains its launch run across a supervisor restart.
	newBinding := f.Restart()
	reject(b, "task_add", map[string]any{"client_id": "stale", "name": "Stale launch"})
	update = change(v)
	update["status"] = "canceled"
	reject(b, "task_patch", update)
	reject(b, "task_release", change(v))
	fresh := connect(t, ctx, conn, newBinding)
	call[task](t, ctx, fresh, "task_add", map[string]any{"client_id": "fresh", "name": "New run"})
	reject(fresh, "task_release", change(v))
	if _, err := taskmcp.New(conn, taskmcp.Binding{Project: f.Actors[0].Project, Session: f.Actors[0].Session}); err == nil {
		t.Fatal("a launch without a run was accepted")
	}

	// Blocking is recorded as a link, read from both ends, and enforced in one
	// place: a blocked task cannot be claimed. Nothing about it writes a status.
	first := call[task](t, ctx, fresh, "task_add", map[string]any{"client_id": "blocker", "name": "Must come first"})
	second := call[task](t, ctx, fresh, "task_add", map[string]any{"client_id": "blocked", "name": "Comes after"})
	linked := call[task](t, ctx, fresh, "task_link", map[string]any{"ref": second.ID, "blocked_by": first.ID})
	if !linked.Blocked || len(linked.BlockedBy) != 1 || linked.BlockedBy[0].Ref != first.Alias || linked.Status != "todo" {
		t.Fatalf("a link neither reported nor left the status alone: %+v", linked)
	}
	if other := call[task](t, ctx, fresh, "task_get", map[string]any{"ref": first.ID}); len(other.Blocks) != 1 || other.Blocks[0].Ref != second.Alias || other.Blocked {
		t.Fatalf("the blocker should read the same link from its end: %+v", other)
	}
	for _, item := range call[page](t, ctx, fresh, "task_list", map[string]any{}).Tasks {
		if item.ID == second.ID && !item.Blocked {
			t.Fatal("task_list did not mark blocked work")
		}
		if item.ID == first.ID && item.Blocked {
			t.Fatal("task_list marked a blocker as blocked")
		}
	}
	reject(fresh, "task_claim", change(linked))
	reject(fresh, "task_link", map[string]any{"ref": second.ID, "blocked_by": second.ID})
	reject(fresh, "task_link", map[string]any{"ref": first.ID, "blocked_by": second.ID})
	// Editing it is still allowed; only starting it is not.
	edit := change(linked)
	edit["desc"] = "Still editable while blocked"
	linked = call[task](t, ctx, fresh, "task_patch", edit)
	free := call[task](t, ctx, fresh, "task_unlink", map[string]any{"ref": second.ID, "blocked_by": first.ID})
	if free.Blocked || len(free.BlockedBy) != 0 {
		t.Fatalf("unlink: %+v", free)
	}
	if started := call[task](t, ctx, fresh, "task_claim", change(free)); started.Status != "in_progress" {
		t.Fatalf("claim after unlink: %+v", started)
	}
	t.Log(fmt.Sprintf("eight MCP tools passed for project %s", f.Actors[0].Project))
}
