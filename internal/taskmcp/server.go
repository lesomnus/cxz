// Package taskmcp exposes session-bound tools over the project Task API.
package taskmcp

import (
	"context"
	_ "embed"
	"fmt"
	"github.com/lesomnus/cxz/internal/projectid"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lesomnus/cxz/resource"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

//go:embed instructions.txt
var Instructions string

type Binding struct {
	Project, Session, Run string
}

type tools struct {
	binding  Binding
	tasks    resource.TaskServiceClient
	sessions resource.SessionServiceClient
}

type taskView struct {
	ID          string            `json:"id"`
	Alias       string            `json:"alias"`
	Name        string            `json:"name"`
	Status      string            `json:"status"`
	Priority    int32             `json:"priority"`
	Assignee    string            `json:"assignee,omitempty"`
	AssigneeRun string            `json:"assignee_run,omitempty"`
	Version     string            `json:"version"`
	Desc        *string           `json:"desc,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	Source      *sourceView       `json:"source,omitempty"`
}

type sourceView struct {
	Session string `json:"session"`
	Run     string `json:"run"`
}

type listInput struct {
	Limit      int32  `json:"limit,omitempty" jsonschema:"Page size, default 20, maximum 100"`
	After      string `json:"after,omitempty" jsonschema:"next cursor from the previous page"`
	Mine       bool   `json:"mine,omitempty" jsonschema:"Only tasks assigned to this session"`
	Unassigned bool   `json:"unassigned,omitempty" jsonschema:"Only unassigned tasks; cannot combine with mine"`
}

type listOutput struct {
	Tasks []taskView `json:"tasks"`
	Next  string     `json:"next,omitempty"`
}

type getInput struct {
	Ref string `json:"ref" jsonschema:"Task ID or alias in this project"`
}

type addInput struct {
	ClientID string            `json:"client_id" jsonschema:"Creation retry key, 1–64 bytes; reuse only for the same creation"`
	Name     string            `json:"name"`
	Desc     string            `json:"desc,omitempty"`
	Alias    string            `json:"alias,omitempty"`
	Priority int32             `json:"priority,omitempty" jsonschema:"0 highest through 4 lowest"`
	Labels   map[string]string `json:"labels,omitempty"`
}

type changeInput struct {
	Ref     string `json:"ref" jsonschema:"Task ID or alias in this project"`
	Version string `json:"version" jsonschema:"Exact version from the last read"`
}

type patchInput struct {
	Ref         string            `json:"ref" jsonschema:"Task ID or alias in this project"`
	Version     string            `json:"version" jsonschema:"Exact version from the last read"`
	Name        *string           `json:"name,omitempty"`
	Desc        *string           `json:"desc,omitempty"`
	Alias       *string           `json:"alias,omitempty"`
	Priority    *int32            `json:"priority,omitempty"`
	Status      string            `json:"status,omitempty" jsonschema:"in_progress, waiting, done or canceled; use Claim to start and Release to return to todo"`
	Labels      map[string]string `json:"labels,omitempty" jsonschema:"Replace labels with a nonempty map"`
	ClearLabels bool              `json:"clear_labels,omitempty"`
}

func New(conn grpc.ClientConnInterface, binding Binding) (*mcp.Server, error) {
	if binding.Project == "" || binding.Session == "" || binding.Run == "" {
		return nil, fmt.Errorf("Task MCP requires a project, session and launch run; restart the agent")
	}
	t := tools{binding: binding, tasks: resource.NewTaskServiceClient(conn), sessions: resource.NewSessionServiceClient(conn)}
	s := mcp.NewServer(&mcp.Implementation{Name: "cxz-tasks", Version: "1"}, &mcp.ServerOptions{Instructions: Instructions})
	s.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
			defer cancel()
			return next(ctx, method, req)
		}
	})
	mcp.AddTool(s, &mcp.Tool{Name: "task_list", Description: "List this project's paginated task summaries, without descriptions or labels.", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true}}, t.list)
	mcp.AddTool(s, &mcp.Tool{Name: "task_get", Description: "Read a task's details and current version.", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true}}, t.get)
	mcp.AddTool(s, &mcp.Tool{Name: "task_add", Description: "Create unassigned Todo work. Same client_id retries return the original task."}, t.add)
	mcp.AddTool(s, &mcp.Tool{Name: "task_claim", Description: "Atomically claim unassigned Todo work for this session/run."}, t.claim)
	mcp.AddTool(s, &mcp.Tool{Name: "task_patch", Description: "Edit unassigned work or this run's task. Execution status changes require ownership."}, t.patch)
	mcp.AddTool(s, &mcp.Tool{Name: "task_release", Description: "Release this run's task back to Todo for another session."}, t.release)
	return s, nil
}

func (t tools) project() *resource.ProjectRef {
	return projectid.Ref(t.binding.Project)
}

func (t tools) actor() *resource.TaskActor {
	return resource.TaskActor_builder{Session: resource.SessionRef_builder{RuntimeId: proto.String(t.binding.Session)}.Build(), RunId: proto.String(t.binding.Run)}.Build()
}

func (t tools) ref(value string) (*resource.TaskRef, error) {
	if strings.TrimSpace(value) == "" {
		return nil, status.Error(codes.InvalidArgument, "task ref is required")
	}
	if id, err := uuid.Parse(value); err == nil {
		return resource.TaskRef_builder{Id: id[:], Project: t.project()}.Build(), nil
	}
	return resource.TaskRef_builder{Alias: proto.String(value), Project: t.project()}.Build(), nil
}

func (t tools) active(ctx context.Context) error {
	s, err := t.sessions.Get(ctx, resource.SessionGetRequest_builder{
		Ref:    t.actor().GetSession(),
		Select: resource.SessionSelect_builder{All: proto.Bool(true), Project: resource.ProjectSelect_builder{All: proto.Bool(true)}.Build()}.Build(),
	}.Build())
	if err != nil {
		return err
	}
	state := s.GetStatus().GetState()
	if !s.GetListed() || !projectid.Same(projectid.String(s.GetProject().GetId()), t.binding.Project) || s.GetStatus().GetRunId() != t.binding.Run || (state != "idle" && state != "working" && state != "waiting_input") {
		return status.Error(codes.FailedPrecondition, "MCP launch run is stale or inactive; restart the agent")
	}
	return nil
}

func view(v *resource.Task, details bool) taskView {
	id, _ := uuid.FromBytes(v.GetId())
	out := taskView{ID: id.String(), Alias: v.GetAlias(), Name: v.GetName(), Status: strings.ToLower(strings.TrimPrefix(v.GetStatus().String(), "TASK_STATUS_")),
		Priority: v.GetPriority(), Assignee: v.GetAssignee(), AssigneeRun: v.GetAssigneeRun(), Version: v.GetDateUpdated().AsTime().Format(time.RFC3339Nano)}
	if details {
		out.Desc, out.Labels = proto.String(v.GetDesc()), v.GetLabels()
		if v.GetSourceSession() != "" {
			out.Source = &sourceView{Session: v.GetSourceSession(), Run: v.GetSourceRun()}
		}
	}
	return out
}

func (t tools) list(ctx context.Context, _ *mcp.CallToolRequest, in listInput) (*mcp.CallToolResult, listOutput, error) {
	if in.Mine && in.Unassigned {
		return nil, listOutput{}, status.Error(codes.InvalidArgument, "mine and unassigned cannot be combined")
	}
	if in.Limit == 0 {
		in.Limit = 20
	}
	if in.Limit < 1 || in.Limit > 100 {
		return nil, listOutput{}, status.Error(codes.InvalidArgument, "limit must be 1 through 100")
	}
	filter := resource.TaskFilter_builder{}.Build()
	if in.Mine {
		filter.SetAssignee(t.binding.Session)
	} else if in.Unassigned {
		filter.SetAssignee("")
	}
	page, err := t.tasks.List(ctx, resource.TaskListRequest_builder{Project: t.project(), Filters: []*resource.TaskFilter{filter}, Size: in.Limit, After: in.After}.Build())
	if err != nil {
		return nil, listOutput{}, err
	}
	out := listOutput{Tasks: make([]taskView, 0, len(page.GetItems())), Next: page.GetNext()}
	for _, v := range page.GetItems() {
		out.Tasks = append(out.Tasks, view(v, false))
	}
	return nil, out, nil
}

func (t tools) read(ctx context.Context, ref *resource.TaskRef) (*resource.Task, error) {
	return t.tasks.Get(ctx, resource.TaskGetRequest_builder{Ref: ref, Select: resource.TaskSelect_builder{All: proto.Bool(true)}.Build()}.Build())
}

func (t tools) get(ctx context.Context, _ *mcp.CallToolRequest, in getInput) (*mcp.CallToolResult, taskView, error) {
	ref, err := t.ref(in.Ref)
	if err != nil {
		return nil, taskView{}, err
	}
	v, err := t.read(ctx, ref)
	if err != nil {
		return nil, taskView{}, err
	}
	return nil, view(v, true), nil
}

func (t tools) add(ctx context.Context, _ *mcp.CallToolRequest, in addInput) (*mcp.CallToolResult, taskView, error) {
	if strings.TrimSpace(in.ClientID) == "" || len(in.ClientID) > 64 {
		return nil, taskView{}, status.Error(codes.InvalidArgument, "client_id must contain 1–64 bytes")
	}
	if err := t.active(ctx); err != nil {
		return nil, taskView{}, err
	}
	v, err := t.tasks.Add(ctx, resource.TaskAddRequest_builder{Project: t.project(), ClientId: "mcp:" + t.binding.Session + ":" + in.ClientID,
		Name: in.Name, Desc: in.Desc, Alias: in.Alias, Priority: in.Priority, Labels: in.Labels, SourceSession: t.binding.Session, SourceRun: t.binding.Run}.Build())
	if err != nil {
		return nil, taskView{}, err
	}
	return nil, view(v, true), nil
}

func (t tools) change(ctx context.Context, in changeInput) (*resource.TaskRef, *timestamppb.Timestamp, error) {
	ref, err := t.ref(in.Ref)
	if err != nil {
		return nil, nil, err
	}
	at, err := time.Parse(time.RFC3339Nano, in.Version)
	if err != nil {
		return nil, nil, status.Error(codes.InvalidArgument, "version must be the exact timestamp from the last read")
	}
	if err := t.active(ctx); err != nil {
		return nil, nil, err
	}
	return ref, timestamppb.New(at), nil
}

func (t tools) claim(ctx context.Context, _ *mcp.CallToolRequest, in changeInput) (*mcp.CallToolResult, taskView, error) {
	ref, at, err := t.change(ctx, in)
	if err != nil {
		return nil, taskView{}, err
	}
	v, err := t.tasks.Claim(ctx, resource.TaskClaimRequest_builder{Ref: ref, DateUpdated: at, Actor: t.actor()}.Build())
	if err != nil {
		return nil, taskView{}, err
	}
	return nil, view(v, true), nil
}

func (t tools) owns(v *resource.Task) bool {
	return v.GetAssignee() == t.binding.Session && v.GetAssigneeRun() == t.binding.Run
}

func (t tools) patch(ctx context.Context, _ *mcp.CallToolRequest, in patchInput) (*mcp.CallToolResult, taskView, error) {
	ref, at, err := t.change(ctx, changeInput{Ref: in.Ref, Version: in.Version})
	if err != nil {
		return nil, taskView{}, err
	}
	v, err := t.read(ctx, ref)
	if err != nil {
		return nil, taskView{}, err
	}
	if v.GetAssignee() != "" && !t.owns(v) {
		return nil, taskView{}, status.Error(codes.FailedPrecondition, "task belongs to another session/run")
	}
	r := resource.TaskPatchRequest_builder{Ref: ref, DateUpdated: at, Actor: t.actor(), Name: in.Name, Desc: in.Desc, Alias: in.Alias,
		Priority: in.Priority, Labels: in.Labels, ClearLabels: proto.Bool(in.ClearLabels)}.Build()
	if in.Status != "" {
		value, ok := resource.TaskStatus_value["TASK_STATUS_"+strings.ToUpper(in.Status)]
		if !ok || value < int32(resource.TaskStatus_TASK_STATUS_IN_PROGRESS) || value > int32(resource.TaskStatus_TASK_STATUS_CANCELED) {
			return nil, taskView{}, status.Error(codes.InvalidArgument, "use in_progress, waiting, done or canceled; Claim starts work and Release returns it to todo")
		}
		st := resource.TaskStatus(value)
		if st != resource.TaskStatus_TASK_STATUS_CANCELED && !t.owns(v) {
			return nil, taskView{}, status.Error(codes.FailedPrecondition, "Claim the task before changing its execution status")
		}
		r.SetStatus(st)
	}
	v, err = t.tasks.Patch(ctx, r)
	if err != nil {
		return nil, taskView{}, err
	}
	return nil, view(v, true), nil
}

func (t tools) release(ctx context.Context, _ *mcp.CallToolRequest, in changeInput) (*mcp.CallToolResult, taskView, error) {
	ref, at, err := t.change(ctx, in)
	if err != nil {
		return nil, taskView{}, err
	}
	v, err := t.tasks.Release(ctx, resource.TaskClaimRequest_builder{Ref: ref, DateUpdated: at, Actor: t.actor()}.Build())
	if err != nil {
		return nil, taskView{}, err
	}
	return nil, view(v, true), nil
}
