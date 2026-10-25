package lifecycle

import (
	"context"
	"io"

	"github.com/lesomnus/cxz/internal/taskapi"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

type TaskLinkServer struct {
	Layer
	resource.UnimplementedTaskLinkServiceServer
}

func (s Layer) TaskLink() resource.TaskLinkServiceServer { return TaskLinkServer{Layer: s} }

// target routes a link request the way a task request is routed, and for the
// same reason: the rows are in the project runtime's resources.db, so the rules
// have to run beside them. A manager hands the whole call over; the runtime
// answers from its own store with the policy wrapped around it.
func (s TaskLinkServer) target(ctx context.Context, ref *resource.ProjectRef) (resource.TaskLinkServiceServer, func(), error) {
	if err := s.effect(); err != nil {
		return nil, nil, err
	}
	if ref == nil {
		return nil, nil, status.Error(codes.InvalidArgument, "project is required")
	}
	p, err := s.Project().Get(ctx, resource.ProjectGetRequest_builder{Ref: ref, Select: resource.ProjectSelect_builder{All: ptr(true)}.Build()}.Build())
	if err != nil {
		return nil, nil, err
	}
	if connector, ok := s.shared.runtime.(taskConnector); ok {
		conn, err := connector.TaskConnection(ctx, p.GetRuntimeId())
		if err != nil {
			return nil, nil, err
		}
		if conn != nil {
			return taskLinkClientServer{
				TaskLinkServiceClient: resource.NewTaskLinkServiceClient(conn),
				project:               resource.ProjectRef_builder{Id: p.GetId()}.Build(),
			}, func() { _ = conn.Close() }, nil
		}
	}
	return s.links(), func() {}, nil
}

// links is the policy over this store. The guard is the layer's, not the
// request's: it has to outlive one call to serialize two.
func (s TaskLinkServer) links() taskapi.LinkService {
	return taskapi.LinkService{
		TaskLinkServiceServer: s.Next().TaskLink(),
		Guard:                 &s.shared.linkMu,
		Project: func(ctx context.Context, ref *resource.ProjectRef) (*resource.Project, error) {
			return s.Project().Get(ctx, resource.ProjectGetRequest_builder{Ref: ref, Select: resource.ProjectSelect_builder{All: ptr(true)}.Build()}.Build())
		},
		Task: func(ctx context.Context, ref *resource.TaskRef) (*resource.Task, error) {
			return s.Next().Task().Get(ctx, resource.TaskGetRequest_builder{Ref: ref, Select: resource.TaskSelect_builder{
				All: ptr(true), Project: resource.ProjectSelect_builder{All: ptr(true)}.Build(),
			}.Build()}.Build())
		},
	}
}

func (s TaskLinkServer) Add(ctx context.Context, r *resource.TaskLinkAddRequest) (*resource.TaskLink, error) {
	backend, close, err := s.target(ctx, r.GetProject())
	if err != nil {
		return nil, err
	}
	defer close()
	return backend.Add(ctx, r)
}
func (s TaskLinkServer) Get(ctx context.Context, r *resource.TaskLinkGetRequest) (*resource.TaskLink, error) {
	backend, close, err := s.target(ctx, r.GetRef().GetProject())
	if err != nil {
		return nil, err
	}
	defer close()
	return backend.Get(ctx, r)
}
func (s TaskLinkServer) Erase(ctx context.Context, r *resource.TaskLinkRef) (*resource.TaskLinkEraseResponse, error) {
	backend, close, err := s.target(ctx, r.GetProject())
	if err != nil {
		return nil, err
	}
	defer close()
	return backend.Erase(ctx, r)
}
func (s TaskLinkServer) List(ctx context.Context, r *resource.TaskLinkListRequest) (*resource.TaskLinkListResponse, error) {
	backend, close, err := s.target(ctx, r.GetProject())
	if err != nil {
		return nil, err
	}
	defer close()
	return backend.List(ctx, r)
}
func (s TaskLinkServer) Watch(r *resource.TaskLinkWatchRequest, out grpc.ServerStreamingServer[resource.TaskLinkWatchResponse]) error {
	backend, close, err := s.target(out.Context(), r.GetProject())
	if err != nil {
		return err
	}
	defer close()
	return backend.Watch(r, out)
}
func (s TaskLinkServer) Patch(ctx context.Context, r *resource.TaskLinkPatchRequest) (*resource.TaskLink, error) {
	return taskapi.LinkService{}.Patch(ctx, r)
}
func (s TaskLinkServer) Apply(ctx context.Context, r *resource.TaskLinkApplyRequest) (*resource.TaskLink, error) {
	return taskapi.LinkService{}.Apply(ctx, r)
}

// Adapt the public client directly; no second legacy runtime protocol.
type taskLinkClientServer struct {
	resource.UnimplementedTaskLinkServiceServer
	resource.TaskLinkServiceClient
	project *resource.ProjectRef
}

func (s taskLinkClientServer) task(ref *resource.TaskRef) *resource.TaskRef {
	if ref == nil {
		return nil
	}
	ref = proto.Clone(ref).(*resource.TaskRef)
	ref.SetProject(s.project)
	return ref
}

func (s taskLinkClientServer) Add(ctx context.Context, r *resource.TaskLinkAddRequest) (*resource.TaskLink, error) {
	r = proto.Clone(r).(*resource.TaskLinkAddRequest)
	r.SetProject(s.project)
	if source := s.task(r.GetSource()); source != nil {
		r.SetSource(source)
	}
	if target := s.task(r.GetTarget()); target != nil {
		r.SetTarget(target)
	}
	return s.TaskLinkServiceClient.Add(ctx, r)
}
func (s taskLinkClientServer) Get(ctx context.Context, r *resource.TaskLinkGetRequest) (*resource.TaskLink, error) {
	r = proto.Clone(r).(*resource.TaskLinkGetRequest)
	if r.GetRef() == nil {
		r.SetRef(&resource.TaskLinkRef{})
	}
	r.GetRef().SetProject(s.project)
	return s.TaskLinkServiceClient.Get(ctx, r)
}
func (s taskLinkClientServer) Erase(ctx context.Context, r *resource.TaskLinkRef) (*resource.TaskLinkEraseResponse, error) {
	r = proto.Clone(r).(*resource.TaskLinkRef)
	r.SetProject(s.project)
	return s.TaskLinkServiceClient.Erase(ctx, r)
}
func (s taskLinkClientServer) filters(filters []*resource.TaskLinkFilter) []*resource.TaskLinkFilter {
	result := make([]*resource.TaskLinkFilter, 0, len(filters))
	for _, f := range filters {
		if f == nil {
			continue
		}
		f = proto.Clone(f).(*resource.TaskLinkFilter)
		f.SetProject(s.project)
		if f.HasRef() {
			f.GetRef().SetProject(s.project)
		}
		if source := s.task(f.GetSource()); source != nil {
			f.SetSource(source)
		}
		if target := s.task(f.GetTarget()); target != nil {
			f.SetTarget(target)
		}
		result = append(result, f)
	}
	return result
}
func (s taskLinkClientServer) List(ctx context.Context, r *resource.TaskLinkListRequest) (*resource.TaskLinkListResponse, error) {
	r = proto.Clone(r).(*resource.TaskLinkListRequest)
	r.SetProject(s.project)
	r.SetFilters(s.filters(r.GetFilters()))
	return s.TaskLinkServiceClient.List(ctx, r)
}
func (s taskLinkClientServer) Watch(r *resource.TaskLinkWatchRequest, out grpc.ServerStreamingServer[resource.TaskLinkWatchResponse]) error {
	r = proto.Clone(r).(*resource.TaskLinkWatchRequest)
	r.SetProject(s.project)
	r.SetFilters(s.filters(r.GetFilters()))
	stream, err := s.TaskLinkServiceClient.Watch(out.Context(), r)
	if err != nil {
		return err
	}
	for {
		value, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if err := out.Send(value); err != nil {
			return err
		}
	}
}
func (s taskLinkClientServer) Patch(ctx context.Context, r *resource.TaskLinkPatchRequest) (*resource.TaskLink, error) {
	return taskapi.LinkService{}.Patch(ctx, r)
}
func (s taskLinkClientServer) Apply(ctx context.Context, r *resource.TaskLinkApplyRequest) (*resource.TaskLink, error) {
	return taskapi.LinkService{}.Apply(ctx, r)
}
