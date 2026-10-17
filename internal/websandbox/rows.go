package websandbox

import (
	"time"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/eventwire"
	"github.com/lesomnus/cxz/internal/transcripthistory"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc"
)

// Rows streams the same projection the sandbox pages. There is no index here --
// the conversation is a slice in memory -- so the projection is rebuilt per
// read, which is affordable at a fake's sizes and keeps the one implementation.
func (x *Sessions) Rows(r *resource.SessionRowsRequest, stream grpc.ServerStreamingServer[resource.SessionRow]) error {
	cursor := r.GetAfterUpdated()
	tick := time.NewTicker(60 * time.Millisecond)
	defer tick.Stop()
	// Heartbeats too, and due immediately like the runtime's: the reader arms
	// its stall deadline on the first one, so a fake that never sent them would
	// be the one place that path is never exercised.
	last := time.Now().Add(-transcripthistory.Heartbeat)
	for {
		versions, err := x.versions(r.GetRef(), cursor)
		if err != nil {
			return err
		}
		for _, v := range versions {
			out := resource.SessionRow_builder{Updated: &v.Updated}.Build()
			out.SetEvent(eventwire.ToResource(v.Event))
			if err = stream.Send(out); err != nil {
				return err
			}
			cursor = v.Updated
			last = time.Now()
		}
		if time.Since(last) >= transcripthistory.Heartbeat {
			if err = stream.Send(resource.SessionRow_builder{Updated: &cursor}.Build()); err != nil {
				return err
			}
			last = time.Now()
		}
		select {
		case <-stream.Context().Done():
			return stream.Context().Err()
		case <-x.S.ctx.Done():
			return x.S.ctx.Err()
		case <-tick.C:
		}
	}
}

func (x *Sessions) versions(ref *resource.SessionRef, cursor uint64) ([]transcripthistory.Version, error) {
	s := x.S
	s.mu.Lock()
	defer s.mu.Unlock()
	st, err := s.find(ref)
	if err != nil {
		return nil, err
	}
	native := make([]*api.Event, 0, len(st.events))
	for _, e := range st.events {
		native = append(native, eventwire.FromResource(st.value.GetRuntimeId(), e))
	}
	return transcripthistory.VersionsFromEvents(native, cursor), nil
}
