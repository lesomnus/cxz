// Package taskapi defines the project task policy shared by native and WASM
// servers. Storage owns atomic version comparisons and committed notifications.
package taskapi

import (
	"bytes"
	"context"
	"strings"

	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type Service struct {
	resource.TaskServiceServer
	Project func(context.Context, *resource.ProjectRef) (*resource.Project, error)
	Session func(context.Context, *resource.SessionRef) (*resource.Session, error)
}

type actionKey struct{}

func actionContext(ctx context.Context, action string) context.Context {
	return context.WithValue(ctx, actionKey{}, action)
}

// MutationAction retains the public operation when Claim/Release use the
// generated versioned Patch internally. Storage publication uses it for Watch.
func MutationAction(ctx context.Context) string {
	action, _ := ctx.Value(actionKey{}).(string)
	return action
}

func all() *resource.TaskSelect {
	return resource.TaskSelect_builder{All: proto.Bool(true), Project: resource.ProjectSelect_builder{All: proto.Bool(true)}.Build()}.Build()
}

func (s Service) scope(ctx context.Context, ref *resource.ProjectRef) (*resource.Project, error) {
	if ref == nil {
		return nil, invalid("project is required")
	}
	p, err := s.Project(ctx, ref)
	if err != nil {
		return nil, err
	}
	if !p.GetListed() {
		return nil, status.Error(codes.NotFound, "project deleted")
	}
	return p, nil
}

func (s Service) read(ctx context.Context, ref *resource.TaskRef) (*resource.Task, error) {
	p, err := s.scope(ctx, ref.GetProject())
	if err != nil {
		return nil, err
	}
	key := proto.Clone(ref).(*resource.TaskRef)
	key.SetProject(resource.ProjectRef_builder{Id: p.GetId()}.Build())
	v, err := s.TaskServiceServer.Get(ctx, resource.TaskGetRequest_builder{Ref: key, Select: all()}.Build())
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(v.GetProject().GetId(), p.GetId()) {
		return nil, status.Error(codes.NotFound, "task not found in project")
	}
	return v, nil
}

func (s Service) Get(ctx context.Context, r *resource.TaskGetRequest) (*resource.Task, error) {
	v, err := s.read(ctx, r.GetRef())
	if err != nil {
		return nil, err
	}
	request := proto.Clone(r).(*resource.TaskGetRequest)
	request.GetRef().SetProject(resource.ProjectRef_builder{Id: v.GetProject().GetId()}.Build())
	return s.TaskServiceServer.Get(ctx, request)
}

func (s Service) Add(ctx context.Context, r *resource.TaskAddRequest) (*resource.Task, error) {
	p, err := s.scope(ctx, r.GetProject())
	if err != nil {
		return nil, err
	}
	if r.HasId() || r.HasDateCreated() || r.GetAssignee() != "" || r.GetAssigneeRun() != "" {
		return nil, invalid("identity, timestamps and ownership are server controlled")
	}
	if r.GetStatus() != resource.TaskStatus_TASK_STATUS_UNSPECIFIED && r.GetStatus() != resource.TaskStatus_TASK_STATUS_TODO {
		return nil, invalid("new tasks must be Todo")
	}
	if strings.TrimSpace(r.GetClientId()) == "" || len(r.GetClientId()) > 128 {
		return nil, invalid("client_id is required and must fit in 128 bytes")
	}
	if err := fields(r.GetName(), r.GetDesc(), r.GetPriority(), r.GetLabels()); err != nil {
		return nil, err
	}
	request := proto.Clone(r).(*resource.TaskAddRequest)
	request.SetProject(resource.ProjectRef_builder{Id: p.GetId()}.Build())
	request.SetStatus(resource.TaskStatus_TASK_STATUS_TODO)
	ref := resource.TaskRef_builder{ClientId: proto.String(r.GetClientId()), Project: request.GetProject()}.Build()
	// A client_id is a project-local idempotency key. The first Add wins, even
	// when its mutable fields have subsequently been edited.
	if v, err := s.read(ctx, ref); err == nil {
		return v, nil
	} else if status.Code(err) != codes.NotFound {
		return nil, err
	}
	if r.GetSourceSession() == "" {
		if r.GetSourceRun() != "" || r.GetSourceSeq() != 0 {
			return nil, invalid("a conversation reference requires source_session")
		}
	} else {
		session, err := s.Session(ctx, resource.SessionRef_builder{RuntimeId: proto.String(r.GetSourceSession())}.Build())
		if err != nil {
			return nil, err
		}
		if !bytes.Equal(session.GetProject().GetId(), p.GetId()) {
			return nil, invalid("source session belongs to another project")
		}
		if r.GetSourceSeq() != 0 && r.GetSourceRun() == "" {
			return nil, invalid("an event reference requires source_run")
		}
	}
	v, err := s.TaskServiceServer.Add(actionContext(ctx, resource.TaskService_Add_FullMethodName), request)
	if status.Code(err) == codes.AlreadyExists {
		return s.read(ctx, ref)
	}
	return v, err
}

func version(at *timestamppb.Timestamp) error {
	if at == nil || at.CheckValid() != nil {
		return invalid("date_updated from the last read is required")
	}
	return nil
}

func (s Service) actor(ctx context.Context, a *resource.TaskActor, p *resource.Project, active bool) (*resource.Session, error) {
	if a.GetSession() == nil || a.GetRunId() == "" {
		return nil, invalid("session and run_id are required")
	}
	v, err := s.Session(ctx, a.GetSession())
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(v.GetProject().GetId(), p.GetId()) {
		return nil, invalid("assignee belongs to another project")
	}
	if active {
		state := v.GetStatus().GetState()
		if !v.GetListed() || v.GetStatus().GetRunId() != a.GetRunId() || (state != "idle" && state != "working") {
			return nil, status.Error(codes.FailedPrecondition, "assignee run is stale or inactive")
		}
	}
	return v, nil
}

func (s Service) Patch(ctx context.Context, r *resource.TaskPatchRequest) (*resource.Task, error) {
	if r.GetDateUpdatedForce() || r.HasAssignee() || r.HasAssigneeRun() {
		return nil, invalid("use Claim/Release to change ownership; force updates are forbidden")
	}
	if err := version(r.GetDateUpdated()); err != nil {
		return nil, err
	}
	v, err := s.read(ctx, r.GetRef())
	if err != nil {
		return nil, err
	}
	next := proto.Clone(v).(*resource.Task)
	if r.HasName() {
		next.SetName(r.GetName())
	}
	if r.HasDesc() {
		next.SetDesc(r.GetDesc())
	}
	if r.HasPriority() {
		next.SetPriority(r.GetPriority())
	}
	if r.HasLabels() {
		next.SetLabels(r.GetLabels())
	}
	if r.GetLabelsNull() {
		next.ClearLabels()
	}
	if err := fields(next.GetName(), next.GetDesc(), next.GetPriority(), next.GetLabels()); err != nil {
		return nil, err
	}
	if r.HasStatus() {
		st := r.GetStatus()
		if st < resource.TaskStatus_TASK_STATUS_TODO || st > resource.TaskStatus_TASK_STATUS_CANCELED {
			return nil, invalid("unknown task status")
		}
		if v.GetAssignee() == "" {
			if st == resource.TaskStatus_TASK_STATUS_IN_PROGRESS || st == resource.TaskStatus_TASK_STATUS_WAITING {
				return nil, invalid("Claim a task before starting it")
			}
		} else {
			if st == resource.TaskStatus_TASK_STATUS_TODO {
				return nil, invalid("use Release to return owned work to Todo")
			}
			// Operators may cancel an abandoned task without impersonating its
			// owner. Other execution transitions require the current owner run.
			if st != resource.TaskStatus_TASK_STATUS_CANCELED {
				owner, err := s.actor(ctx, r.GetActor(), v.GetProject(), true)
				if err != nil {
					return nil, err
				}
				if owner.GetRuntimeId() != v.GetAssignee() || r.GetActor().GetRunId() != v.GetAssigneeRun() {
					return nil, status.Error(codes.FailedPrecondition, "task belongs to another session/run")
				}
			}
		}
	}
	request := proto.Clone(r).(*resource.TaskPatchRequest)
	request.SetRef(resource.TaskRef_builder{Id: v.GetId(), Project: resource.ProjectRef_builder{Id: v.GetProject().GetId()}.Build()}.Build())
	request.ClearActor() // Routing/policy fields are not ORM patch fields.
	return s.TaskServiceServer.Patch(actionContext(ctx, resource.TaskService_Patch_FullMethodName), request)
}

func (s Service) Claim(ctx context.Context, r *resource.TaskClaimRequest) (*resource.Task, error) {
	if err := version(r.GetDateUpdated()); err != nil {
		return nil, err
	}
	v, err := s.read(ctx, r.GetRef())
	if err != nil {
		return nil, err
	}
	a, err := s.actor(ctx, r.GetActor(), v.GetProject(), true)
	if err != nil {
		return nil, err
	}
	if v.GetAssignee() == a.GetRuntimeId() && v.GetAssigneeRun() == r.GetActor().GetRunId() &&
		(v.GetStatus() == resource.TaskStatus_TASK_STATUS_IN_PROGRESS || v.GetStatus() == resource.TaskStatus_TASK_STATUS_WAITING) {
		return v, nil
	}
	if v.GetAssignee() != "" || v.GetStatus() != resource.TaskStatus_TASK_STATUS_TODO {
		return nil, status.Error(codes.FailedPrecondition, "task is already owned or not Todo")
	}
	return s.TaskServiceServer.Patch(actionContext(ctx, resource.TaskService_Claim_FullMethodName), resource.TaskPatchRequest_builder{
		Ref: resource.TaskRef_builder{Id: v.GetId()}.Build(), DateUpdated: r.GetDateUpdated(), Assignee: proto.String(a.GetRuntimeId()),
		AssigneeRun: proto.String(r.GetActor().GetRunId()), Status: resource.TaskStatus_TASK_STATUS_IN_PROGRESS.Enum(),
	}.Build())
}

func (s Service) Release(ctx context.Context, r *resource.TaskClaimRequest) (*resource.Task, error) {
	if err := version(r.GetDateUpdated()); err != nil {
		return nil, err
	}
	v, err := s.read(ctx, r.GetRef())
	if err != nil {
		return nil, err
	}
	// Release is an explicit operator action, not an automatic lease expiry.
	// Resolve the expected session ID when it exists; a purged session can still
	// be named by its retained runtime ID. The version fences a later claim.
	id := r.GetActor().GetSession().GetRuntimeId()
	if id == "" {
		a, err := s.actor(ctx, r.GetActor(), v.GetProject(), false)
		if err != nil {
			return nil, err
		}
		id = a.GetRuntimeId()
	}
	if id == "" || r.GetActor().GetRunId() == "" || id != v.GetAssignee() || r.GetActor().GetRunId() != v.GetAssigneeRun() {
		return nil, status.Error(codes.FailedPrecondition, "expected owner does not match")
	}
	return s.TaskServiceServer.Patch(actionContext(ctx, resource.TaskService_Release_FullMethodName), resource.TaskPatchRequest_builder{
		Ref: resource.TaskRef_builder{Id: v.GetId()}.Build(), DateUpdated: r.GetDateUpdated(), Assignee: proto.String(""), AssigneeRun: proto.String(""),
		Status: resource.TaskStatus_TASK_STATUS_TODO.Enum(),
	}.Build())
}

// NormalizeFilters resolves every project constraint before routing a request.
func (s Service) NormalizeFilters(ctx context.Context, project *resource.ProjectRef, filters []*resource.TaskFilter) ([]*resource.TaskFilter, error) {
	p, err := s.scope(ctx, project)
	if err != nil {
		return nil, err
	}
	if len(filters) > 32 {
		return nil, invalid("too many task filters")
	}
	if len(filters) == 0 {
		filters = []*resource.TaskFilter{resource.TaskFilter_builder{}.Build()}
	}
	result := make([]*resource.TaskFilter, 0, len(filters))
	for _, f := range filters {
		if f == nil {
			return nil, invalid("empty task filter")
		}
		if f.HasProject() {
			other, err := s.scope(ctx, f.GetProject())
			if err != nil {
				return nil, err
			}
			if !bytes.Equal(other.GetId(), p.GetId()) {
				return nil, invalid("filter is outside project scope")
			}
		}
		if f.HasRef() {
			// A filter's ref identifies a row; the required outer scope wins.
			f = proto.Clone(f).(*resource.TaskFilter)
			f.GetRef().SetProject(resource.ProjectRef_builder{Id: p.GetId()}.Build())
		}
		f = proto.Clone(f).(*resource.TaskFilter)
		f.SetProject(resource.ProjectRef_builder{Id: p.GetId()}.Build())
		result = append(result, f)
	}
	return result, nil
}

func (s Service) List(ctx context.Context, r *resource.TaskListRequest) (*resource.TaskListResponse, error) {
	filters, err := s.NormalizeFilters(ctx, r.GetProject(), r.GetFilters())
	if err != nil {
		return nil, err
	}
	request := proto.Clone(r).(*resource.TaskListRequest)
	request.SetFilters(filters)
	return s.TaskServiceServer.List(ctx, request)
}

func (s Service) Watch(r *resource.TaskWatchRequest, out grpc.ServerStreamingServer[resource.TaskWatchResponse]) error {
	filters, err := s.NormalizeFilters(out.Context(), r.GetProject(), r.GetFilters())
	if err != nil {
		return err
	}
	request := proto.Clone(r).(*resource.TaskWatchRequest)
	request.SetFilters(filters)
	return s.TaskServiceServer.Watch(request, out)
}

func (s Service) Apply(context.Context, *resource.TaskApplyRequest) (*resource.Task, error) {
	return nil, status.Error(codes.PermissionDenied, "use versioned Patch, Claim or Release")
}
func (s Service) Erase(context.Context, *resource.TaskRef) (*resource.TaskEraseResponse, error) {
	return nil, status.Error(codes.PermissionDenied, "cancel tasks instead of deleting shared work history")
}

func invalid(s string) error { return status.Error(codes.InvalidArgument, s) }
func fields(name, desc string, priority int32, labels *resource.TaskLabels) error {
	if strings.TrimSpace(name) == "" || len(name) > 256 {
		return invalid("task name must contain 1–256 bytes")
	}
	if len(desc) > 65536 {
		return invalid("task description exceeds 64 KiB")
	}
	if priority < 0 || priority > 4 {
		return invalid("priority must be 0 (highest) through 4")
	}
	if len(labels.GetValues()) > 32 {
		return invalid("too many labels")
	}
	for _, label := range labels.GetValues() {
		if strings.TrimSpace(label) == "" || len(label) > 64 {
			return invalid("labels must contain 1–64 bytes")
		}
	}
	return nil
}
