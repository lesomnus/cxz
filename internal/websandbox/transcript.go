package websandbox

import (
	"context"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/eventwire"
	"github.com/lesomnus/cxz/internal/transcripthistory"
	"github.com/lesomnus/cxz/resource"
)

func (x *Sessions) Transcript(_ context.Context, r *resource.SessionTranscriptRequest) (*resource.SessionTranscriptReply, error) {
	s := x.S
	s.mu.Lock()
	defer s.mu.Unlock()
	st, err := s.find(r.GetRef())
	if err != nil {
		return nil, err
	}
	native := make([]*api.Event, 0, len(st.events))
	for _, e := range st.events {
		native = append(native, eventwire.FromResource(st.value.GetRuntimeId(), e))
	}
	page, err := transcripthistory.FromEvents(native, &api.TranscriptRequest{AfterSeq: r.GetAfterSeq(), BeforeSeq: r.GetBeforeSeq(), Limit: r.GetLimit(), SnapshotSeq: r.GetSnapshotSeq()})
	if err != nil {
		return nil, err
	}
	out := resource.SessionTranscriptReply_builder{SnapshotSeq: &page.SnapshotSeq, HasOlder: &page.HasOlder, HasNewer: &page.HasNewer}.Build()
	for _, e := range page.Events {
		out.SetEvents(append(out.GetEvents(), eventwire.ToResource(e)))
	}
	for _, e := range page.Metadata {
		out.SetMetadata(append(out.GetMetadata(), eventwire.ToResource(e)))
	}
	if page.PrecedingInput != nil {
		out.SetPrecedingInput(eventwire.ToResource(page.PrecedingInput))
	}
	return out, nil
}
func (x *Sessions) EventDetails(_ context.Context, r *resource.SessionEventDetailsRequest) (*resource.SessionEventBatch, error) {
	s := x.S
	s.mu.Lock()
	defer s.mu.Unlock()
	st, err := s.find(r.GetRef())
	if err != nil {
		return nil, err
	}
	native := make([]*api.Event, 0, len(st.events))
	for _, e := range st.events {
		native = append(native, eventwire.FromResource(st.value.GetRuntimeId(), e))
	}
	page, err := transcripthistory.DetailsFromEvents(native, r.GetSeq())
	if err != nil {
		return nil, err
	}
	out := resource.SessionEventBatch_builder{}.Build()
	for _, e := range page.Events {
		out.SetEvents(append(out.GetEvents(), eventwire.ToResource(e)))
	}
	return out, nil
}
