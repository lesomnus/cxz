package lifecycle

import (
	"bytes"
	"context"
	"io"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/taskapi"
	"github.com/lesomnus/cxz/resource"
	"github.com/lesomnus/cxz/server/pd"
	"github.com/lesomnus/payday/watch"
	"github.com/lesomnus/protobuf-patch/patch"
	"github.com/protobuf-orm/protobuf-orm/ormpatch"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

type TaskServer struct {
	Layer
	resource.UnimplementedTaskServiceServer
}

func (s Layer) Task() resource.TaskServiceServer { return TaskServer{Layer: s} }

func taskWatchUnary(w *watch.Watch) grpc.UnaryServerInterceptor {
	next := w.Unary()
	return func(ctx context.Context, request any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if action := taskapi.MutationAction(ctx); action != "" {
			copy := *info
			copy.FullMethod = action
			info = &copy
		}
		return next(ctx, request, info, handler)
	}
}

// Manager implementations return an authenticated project connection. A project
// runtime returns nil and uses its own volume's resources.db. Never fall back to
// a manager-side copy when the authoritative project is stopped/unreachable.
type taskConnector interface {
	TaskConnection(context.Context, string) (*grpc.ClientConn, error)
}

func (s TaskServer) target(ctx context.Context, ref *resource.ProjectRef) (resource.TaskServiceServer, func(), error) {
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
			return taskClientServer{TaskServiceClient: resource.NewTaskServiceClient(conn), project: resource.ProjectRef_builder{Id: p.GetId()}.Build(), layer: s.Layer}, func() { _ = conn.Close() }, nil
		}
	}
	return taskapi.Service{
		TaskServiceServer: taskInventory{TaskServiceServer: s.Next().Task(), Layer: s.Layer},
		Project: func(ctx context.Context, ref *resource.ProjectRef) (*resource.Project, error) {
			return s.Project().Get(ctx, resource.ProjectGetRequest_builder{Ref: ref, Select: resource.ProjectSelect_builder{All: ptr(true)}.Build()}.Build())
		},
		Session: func(ctx context.Context, ref *resource.SessionRef) (*resource.Session, error) {
			v, err := s.Session().Get(ctx, resource.SessionGetRequest_builder{Ref: ref, Select: resource.SessionSelect_builder{All: ptr(true), Project: resource.ProjectSelect_builder{All: ptr(true)}.Build()}.Build()}.Build())
			if err != nil {
				return nil, err
			}
			// Claim checks the live run, not the manager's periodic inventory.
			live, err := s.shared.runtime.Get(ctx, &api.SessionRef{Id: v.GetRuntimeId()})
			if err != nil {
				return nil, err
			}
			v.SetStatus(resource.SessionStatus_builder{State: live.State, RunId: live.RunId}.Build())
			return v, nil
		},
	}, func() {}, nil
}

func (s TaskServer) Add(ctx context.Context, r *resource.TaskAddRequest) (*resource.Task, error) {
	backend, close, err := s.target(ctx, r.GetProject())
	if err != nil {
		return nil, err
	}
	defer close()
	return backend.Add(ctx, r)
}
func (s TaskServer) Get(ctx context.Context, r *resource.TaskGetRequest) (*resource.Task, error) {
	backend, close, err := s.target(ctx, r.GetRef().GetProject())
	if err != nil {
		return nil, err
	}
	defer close()
	return backend.Get(ctx, r)
}
func (s TaskServer) Patch(ctx context.Context, r *resource.TaskPatchRequest) (*resource.Task, error) {
	backend, close, err := s.target(ctx, r.GetRef().GetProject())
	if err != nil {
		return nil, err
	}
	defer close()
	return backend.Patch(ctx, r)
}
func (s TaskServer) Claim(ctx context.Context, r *resource.TaskClaimRequest) (*resource.Task, error) {
	backend, close, err := s.target(ctx, r.GetRef().GetProject())
	if err != nil {
		return nil, err
	}
	defer close()
	return backend.Claim(ctx, r)
}
func (s TaskServer) Release(ctx context.Context, r *resource.TaskClaimRequest) (*resource.Task, error) {
	backend, close, err := s.target(ctx, r.GetRef().GetProject())
	if err != nil {
		return nil, err
	}
	defer close()
	return backend.Release(ctx, r)
}
func (s TaskServer) List(ctx context.Context, r *resource.TaskListRequest) (*resource.TaskListResponse, error) {
	backend, close, err := s.target(ctx, r.GetProject())
	if err != nil {
		return nil, err
	}
	defer close()
	return backend.List(ctx, r)
}
func (s TaskServer) Watch(r *resource.TaskWatchRequest, out grpc.ServerStreamingServer[resource.TaskWatchResponse]) error {
	backend, close, err := s.target(out.Context(), r.GetProject())
	if err != nil {
		return err
	}
	defer close()
	return backend.Watch(r, out)
}
func (s TaskServer) Apply(ctx context.Context, r *resource.TaskApplyRequest) (*resource.Task, error) {
	return taskapi.Service{}.Apply(ctx, r)
}
func (s TaskServer) Erase(ctx context.Context, r *resource.TaskRef) (*resource.TaskEraseResponse, error) {
	return taskapi.Service{}.Erase(ctx, r)
}

// Adapt the public client directly; no second legacy runtime protocol.
type taskClientServer struct {
	resource.UnimplementedTaskServiceServer
	resource.TaskServiceClient
	project *resource.ProjectRef
	layer   Layer
}

func (s taskClientServer) ref(ref *resource.TaskRef) *resource.TaskRef {
	if ref == nil {
		ref = &resource.TaskRef{}
	} else {
		ref = proto.Clone(ref).(*resource.TaskRef)
	}
	ref.SetProject(s.project)
	return ref
}
func (s taskClientServer) actor(ctx context.Context, actor *resource.TaskActor) (*resource.TaskActor, error) {
	if actor == nil {
		return nil, nil
	}
	result := proto.Clone(actor).(*resource.TaskActor)
	if actor.GetSession() == nil {
		return result, nil
	}
	// Session aliases belong to an inventory; runtime IDs survive that boundary.
	v, err := s.layer.Session().Get(ctx, resource.SessionGetRequest_builder{Ref: actor.GetSession(), Select: resource.SessionSelect_builder{All: ptr(true)}.Build()}.Build())
	if err != nil {
		// Release remains possible after session purge using its snapshot ID.
		if actor.GetSession().HasRuntimeId() {
			return result, nil
		}
		return nil, err
	}
	result.SetSession(sessionRef(v.GetRuntimeId()))
	return result, nil
}
func (s taskClientServer) filters(ctx context.Context, filters []*resource.TaskFilter) ([]*resource.TaskFilter, error) {
	return (taskapi.Service{Project: func(ctx context.Context, ref *resource.ProjectRef) (*resource.Project, error) {
		return s.layer.Project().Get(ctx, resource.ProjectGetRequest_builder{Ref: ref, Select: resource.ProjectSelect_builder{All: ptr(true)}.Build()}.Build())
	}}).NormalizeFilters(ctx, s.project, filters)
}

func (s taskClientServer) Add(ctx context.Context, r *resource.TaskAddRequest) (*resource.Task, error) {
	r = proto.Clone(r).(*resource.TaskAddRequest)
	r.SetProject(s.project)
	return s.TaskServiceClient.Add(ctx, r)
}
func (s taskClientServer) Get(ctx context.Context, r *resource.TaskGetRequest) (*resource.Task, error) {
	r = proto.Clone(r).(*resource.TaskGetRequest)
	r.SetRef(s.ref(r.GetRef()))
	return s.TaskServiceClient.Get(ctx, r)
}
func (s taskClientServer) Patch(ctx context.Context, r *resource.TaskPatchRequest) (*resource.Task, error) {
	r = proto.Clone(r).(*resource.TaskPatchRequest)
	r.SetRef(s.ref(r.GetRef()))
	actor, err := s.actor(ctx, r.GetActor())
	if err != nil {
		return nil, err
	}
	r.SetActor(actor)
	return s.TaskServiceClient.Patch(ctx, r)
}
func (s taskClientServer) Claim(ctx context.Context, r *resource.TaskClaimRequest) (*resource.Task, error) {
	r = proto.Clone(r).(*resource.TaskClaimRequest)
	r.SetRef(s.ref(r.GetRef()))
	actor, err := s.actor(ctx, r.GetActor())
	if err != nil {
		return nil, err
	}
	r.SetActor(actor)
	return s.TaskServiceClient.Claim(ctx, r)
}
func (s taskClientServer) Release(ctx context.Context, r *resource.TaskClaimRequest) (*resource.Task, error) {
	r = proto.Clone(r).(*resource.TaskClaimRequest)
	r.SetRef(s.ref(r.GetRef()))
	actor, err := s.actor(ctx, r.GetActor())
	if err != nil {
		return nil, err
	}
	r.SetActor(actor)
	return s.TaskServiceClient.Release(ctx, r)
}
func (s taskClientServer) List(ctx context.Context, r *resource.TaskListRequest) (*resource.TaskListResponse, error) {
	r = proto.Clone(r).(*resource.TaskListRequest)
	r.SetProject(s.project)
	filters, err := s.filters(ctx, r.GetFilters())
	if err != nil {
		return nil, err
	}
	r.SetFilters(filters)
	return s.TaskServiceClient.List(ctx, r)
}
func (s taskClientServer) Apply(ctx context.Context, r *resource.TaskApplyRequest) (*resource.Task, error) {
	return nil, closed()
}
func (s taskClientServer) Erase(ctx context.Context, r *resource.TaskRef) (*resource.TaskEraseResponse, error) {
	return nil, closed()
}
func (s taskClientServer) Watch(r *resource.TaskWatchRequest, out grpc.ServerStreamingServer[resource.TaskWatchResponse]) error {
	r = proto.Clone(r).(*resource.TaskWatchRequest)
	r.SetProject(s.project)
	filters, err := s.filters(out.Context(), r.GetFilters())
	if err != nil {
		return err
	}
	r.SetFilters(filters)
	stream, err := s.TaskServiceClient.Watch(out.Context(), r)
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

type taskInventory struct {
	resource.TaskServiceServer
	Layer
}

func (s taskInventory) Patch(ctx context.Context, r *resource.TaskPatchRequest) (*resource.Task, error) {
	if !r.GetClearLabels() {
		return s.TaskServiceServer.Patch(ctx, r)
	}
	// Empty maps have no wire presence. Preserve the generated version test and
	// other edits, then append an explicit empty-map assignment to the document.
	doc, err := ormpatch.FromPatchRequest(ormpatch.MustEntityOf(resource.File_cxz_task_proto, "Task"), r.ProtoReflect(), nil)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	clear := patch.MustNew("cxz.Task", patch.Target(patch.Name("labels")).Assign(patch.Map()))
	doc.GetDelta().SetEntries(append(doc.GetDelta().GetEntries(), clear.GetDelta().GetEntries()...))
	return s.TaskServiceServer.Apply(ctx, resource.TaskApplyRequest_builder{Ref: r.GetRef(), Patch: doc}.Build())
}

// Empty snapshots settle immediately too. Changes are coalesced full state from
// the same broker that publishes generated Sink writes after commit.
func (s taskInventory) Watch(r *resource.TaskWatchRequest, out grpc.ServerStreamingServer[resource.TaskWatchResponse]) error {
	ctx := out.Context()
	return streamInventory(ctx, s.shared.watch, pd.TaskService, r.GetSkipSnapshot(),
		func(after string) ([]*resource.Task, string, error) {
			page, err := s.TaskServiceServer.List(ctx, resource.TaskListRequest_builder{Filters: r.GetFilters(), After: after}.Build())
			if err != nil {
				return nil, "", err
			}
			return page.GetItems(), page.GetNext(), nil
		},
		func(id []byte) (*resource.Task, bool, error) {
			filters := make([]*resource.TaskFilter, 0, len(r.GetFilters()))
			for _, original := range r.GetFilters() {
				if original.HasRef() {
					v, err := s.TaskServiceServer.Get(ctx, resource.TaskGetRequest_builder{Ref: original.GetRef(), Select: resource.TaskSelect_builder{All: ptr(true)}.Build()}.Build())
					if status.Code(err) == codes.NotFound {
						continue
					}
					if err != nil {
						return nil, false, err
					}
					if !bytes.Equal(v.GetId(), id) {
						continue
					}
				}
				f := proto.Clone(original).(*resource.TaskFilter)
				f.SetRef(resource.TaskRef_builder{Id: id}.Build())
				filters = append(filters, f)
			}
			if len(filters) == 0 {
				return nil, false, nil
			}
			page, err := s.TaskServiceServer.List(ctx, resource.TaskListRequest_builder{Filters: filters, Size: 1}.Build())
			if err != nil {
				return nil, false, err
			}
			if len(page.GetItems()) == 0 {
				return nil, false, nil
			}
			return page.GetItems()[0], true, nil
		},
		func(rows []inventoryRow[*resource.Task]) error {
			items := make([]*resource.TaskWatchItem, 0, len(rows))
			for _, row := range rows {
				items = append(items, resource.TaskWatchItem_builder{Id: row.id, Value: row.value, Action: row.action}.Build())
			}
			return out.Send(resource.TaskWatchResponse_builder{Items: items}.Build())
		})
}
