package lifecycle

import (
	"bytes"
	"context"
	"strings"

	"github.com/lesomnus/cxz/internal/taskapi"
	"github.com/lesomnus/cxz/resource"
	"github.com/lesomnus/cxz/server/bare"
	"github.com/lesomnus/cxz/server/pd"
	"github.com/lesomnus/payday/pdid"
	"github.com/lesomnus/payday/watch"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Project identity survives erasure and filter exits without publishing bodies
// or unrelated projects' identifiers to a changes-only subscriber.
type taskWatchRecorder struct {
	bare.Recorder
	watch *watch.Watch
}

func (r taskWatchRecorder) Snapshots(method string) bare.SnapshotOptions {
	if strings.HasPrefix(method, pd.TaskService) {
		return bare.SnapshotOptions{Value: true, Previous: method == resource.TaskService_Erase_FullMethodName}
	}
	if next, ok := r.Recorder.(bare.SnapshotRecorder); ok {
		return next.Snapshots(method)
	}
	return bare.SnapshotOptions{}
}

func (r taskWatchRecorder) Record(ctx context.Context, store bare.Server, change bare.Change) error {
	if !strings.HasPrefix(change.By, pd.TaskService) {
		return r.Recorder.Record(ctx, store, change)
	}
	value, _ := change.Value.(*resource.Task)
	if value == nil {
		value, _ = change.Previous.(*resource.Task)
	}
	if value == nil || len(value.GetProject().GetId()) != 16 {
		return status.Error(codes.Internal, "task change has no project identity")
	}
	id, err := pdid.From(value.GetId())
	if err != nil {
		return err
	}
	r.watch.Note(ctx, watch.Change{Method: change.Method, By: change.By, Key: id, Signal: bytes.Clone(value.GetProject().GetId())})
	return nil
}

type taskWatchChange struct {
	project []byte
	action  string
}

func (s taskInventory) watchChanges(r *resource.TaskWatchRequest, out grpc.ServerStreamingServer[resource.TaskWatchResponse], read func([]byte) (*resource.Task, bool, error)) error {
	if s.shared.watch == nil || s.shared.watch.Broker() == nil {
		return status.Error(codes.Unavailable, "task watch broker is disabled")
	}
	events, stop := s.shared.watch.Subscribe()
	defer stop()
	if err := taskapi.WatchReady(out); err != nil {
		return err
	}
	for {
		changes := map[pdid.Id]taskWatchChange{}
		take := func(event watch.Event) {
			for _, change := range event.Changes {
				if strings.HasPrefix(change.By, pd.TaskService) {
					changes[change.Key] = taskWatchChange{project: change.Signal, action: event.Method}
				}
			}
		}
		select {
		case <-out.Context().Done():
			return status.FromContextError(out.Context().Err()).Err()
		case event, ok := <-events:
			if !ok {
				return watch.ErrBehind
			}
			take(event)
		}
	drain:
		for {
			select {
			case event, ok := <-events:
				if !ok {
					return watch.ErrBehind
				}
				take(event)
			default:
				break drain
			}
		}
		items := make([]*resource.TaskWatchItem, 0, len(changes))
		for id, change := range changes {
			authorized := false
			for _, filter := range r.GetFilters() {
				if bytes.Equal(change.project, filter.GetProject().GetId()) && len(change.project) == 16 {
					authorized = true
					break
				}
			}
			if !authorized {
				continue
			}
			value, matches, err := read(id.Bytes())
			if err != nil {
				return err
			}
			if !matches {
				value = nil
			}
			items = append(items, resource.TaskWatchItem_builder{Id: id.Bytes(), Value: value, Action: change.action}.Build())
		}
		if len(items) > 0 {
			if err := out.Send(resource.TaskWatchResponse_builder{Items: items}.Build()); err != nil {
				return err
			}
		}
	}
}
