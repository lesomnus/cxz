package lifecycle

import (
	"bytes"
	"context"

	"github.com/lesomnus/cxz/resource"
	"github.com/lesomnus/cxz/server/pd"
	"github.com/lesomnus/payday/pdid"
	"github.com/lesomnus/payday/watch"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

type inventoryRow[T interface{ GetId() []byte }] struct {
	id     []byte
	value  T
	action string
}

// Register with the same committed-write broker as the generated resource
// watchers. Stream subscribes before reading the snapshot, closing the initial
// list/change race, and coalesces writes into the latest state of each row.
func streamInventory[T interface{ GetId() []byte }](
	ctx context.Context, broker *watch.Watch, service string, skip bool,
	list func(string) ([]T, string, error),
	read func([]byte) (T, bool, error),
	send func([]inventoryRow[T]) error,
) error {
	var snapshot func(watch.Seen) error
	if !skip {
		snapshot = func(seen watch.Seen) error {
			after := ""
			for {
				values, next, err := list(after)
				if err != nil {
					return err
				}
				rows := make([]inventoryRow[T], 0, len(values))
				for _, value := range values {
					id, err := pdid.From(value.GetId())
					if err != nil {
						return err
					}
					seen[id] = true
					rows = append(rows, inventoryRow[T]{id: value.GetId(), value: value})
				}
				// An empty initial frame settles a catalog with no matching rows too.
				if err := send(rows); err != nil {
					return err
				}
				if next == "" {
					return nil
				}
				after = next
			}
		}
	}
	return watch.Stream(ctx, broker, service, snapshot, func(changes map[pdid.Id]string, seen watch.Seen) error {
		rows := make([]inventoryRow[T], 0, len(changes))
		for id, action := range changes {
			value, matches, err := read(id.Bytes())
			if err != nil {
				return err
			}
			if !matches && !seen[id] {
				continue
			}
			seen[id] = matches
			rows = append(rows, inventoryRow[T]{id: id.Bytes(), value: value, action: action})
		}
		if len(rows) == 0 {
			return nil
		}
		return send(rows)
	})
}

func (s ProjectServer) watchInventory(r *resource.ProjectWatchRequest, out grpc.ServerStreamingServer[resource.ProjectWatchResponse]) error {
	ctx := out.Context()
	if len(r.GetFilters()) > pd.ProjectFilterLimit {
		return status.Error(codes.InvalidArgument, "too many project filters")
	}
	filters := make([]*resource.ProjectFilter, 0, len(r.GetFilters()))
	for _, original := range r.GetFilters() {
		if original == nil {
			return status.Error(codes.InvalidArgument, "empty project filter")
		}
		f := proto.Clone(original).(*resource.ProjectFilter)
		if f.HasRef() {
			value, err := s.ProjectServiceServer.Get(ctx, resource.ProjectGetRequest_builder{Ref: f.GetRef()}.Build())
			if err != nil {
				return err
			}
			f.SetRef(resource.ProjectRef_builder{Id: value.GetId()}.Build())
		}
		filters = append(filters, f)
	}
	return streamInventory(ctx, s.shared.watch, pd.ProjectService, r.GetSkipSnapshot(),
		func(after string) ([]*resource.Project, string, error) {
			page, err := s.ProjectServiceServer.List(ctx, resource.ProjectListRequest_builder{Filters: filters, After: after}.Build())
			if err != nil {
				return nil, "", err
			}
			return page.GetItems(), page.GetNext(), nil
		},
		func(id []byte) (*resource.Project, bool, error) {
			current := make([]*resource.ProjectFilter, 0, len(filters))
			for _, original := range filters {
				if original.HasRef() && !bytes.Equal(original.GetRef().GetId(), id) {
					continue
				}
				f := proto.Clone(original).(*resource.ProjectFilter)
				f.SetRef(resource.ProjectRef_builder{Id: id}.Build())
				current = append(current, f)
			}
			if len(current) == 0 {
				return nil, false, nil
			}
			page, err := s.ProjectServiceServer.List(ctx, resource.ProjectListRequest_builder{Filters: current, Size: 1}.Build())
			if err != nil {
				return nil, false, err
			}
			if len(page.GetItems()) == 0 {
				return nil, false, nil
			}
			return page.GetItems()[0], true, nil
		},
		func(rows []inventoryRow[*resource.Project]) error {
			items := make([]*resource.ProjectWatchItem, 0, len(rows))
			for _, row := range rows {
				items = append(items, resource.ProjectWatchItem_builder{Id: row.id, Value: row.value, Action: row.action}.Build())
			}
			return out.Send(resource.ProjectWatchResponse_builder{Items: items}.Build())
		})
}

func (s SessionServer) watchInventory(r *resource.SessionWatchRequest, out grpc.ServerStreamingServer[resource.SessionWatchResponse]) error {
	ctx := out.Context()
	if len(r.GetFilters()) > pd.SessionFilterLimit {
		return status.Error(codes.InvalidArgument, "too many session filters")
	}
	filters := make([]*resource.SessionFilter, 0, len(r.GetFilters()))
	for _, original := range r.GetFilters() {
		if original == nil {
			return status.Error(codes.InvalidArgument, "empty session filter")
		}
		f := proto.Clone(original).(*resource.SessionFilter)
		if f.HasRef() {
			value, err := s.SessionServiceServer.Get(ctx, resource.SessionGetRequest_builder{Ref: f.GetRef()}.Build())
			if err != nil {
				return err
			}
			f.SetRef(resource.SessionRef_builder{Id: value.GetId()}.Build())
		}
		if f.HasProject() {
			value, err := s.Next().Project().Get(ctx, resource.ProjectGetRequest_builder{Ref: f.GetProject()}.Build())
			if err != nil {
				return err
			}
			f.SetProject(resource.ProjectRef_builder{Id: value.GetId()}.Build())
		}
		filters = append(filters, f)
	}
	return streamInventory(ctx, s.shared.watch, pd.SessionService, r.GetSkipSnapshot(),
		func(after string) ([]*resource.Session, string, error) {
			page, err := s.SessionServiceServer.List(ctx, resource.SessionListRequest_builder{Filters: filters, After: after}.Build())
			if err != nil {
				return nil, "", err
			}
			return page.GetItems(), page.GetNext(), nil
		},
		func(id []byte) (*resource.Session, bool, error) {
			current := make([]*resource.SessionFilter, 0, len(filters))
			for _, original := range filters {
				if original.HasRef() && !bytes.Equal(original.GetRef().GetId(), id) {
					continue
				}
				f := proto.Clone(original).(*resource.SessionFilter)
				f.SetRef(resource.SessionRef_builder{Id: id}.Build())
				current = append(current, f)
			}
			if len(current) == 0 {
				return nil, false, nil
			}
			page, err := s.SessionServiceServer.List(ctx, resource.SessionListRequest_builder{Filters: current, Size: 1}.Build())
			if err != nil {
				return nil, false, err
			}
			if len(page.GetItems()) == 0 {
				return nil, false, nil
			}
			return page.GetItems()[0], true, nil
		},
		func(rows []inventoryRow[*resource.Session]) error {
			items := make([]*resource.SessionWatchItem, 0, len(rows))
			for _, row := range rows {
				items = append(items, resource.SessionWatchItem_builder{Id: row.id, Value: row.value, Action: row.action}.Build())
			}
			return out.Send(resource.SessionWatchResponse_builder{Items: items}.Build())
		})
}
