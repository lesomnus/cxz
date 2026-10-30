package websandbox

import (
	"context"
	"fmt"

	"github.com/lesomnus/cxz/internal/resourceid"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (x *Sessions) Fork(_ context.Context, r *resource.SessionControl) (*resource.Session, error) {
	s := x.S
	s.mu.Lock()
	defer s.mu.Unlock()
	if r.GetClientId() == "" || r.GetRunId() == "" {
		return nil, status.Error(codes.InvalidArgument, "run_id and client_id required")
	}
	source, err := s.find(r.GetRef())
	if err != nil {
		return nil, err
	}
	for _, existing := range s.sessions {
		if existing.value.GetClientId() == r.GetClientId() {
			if existing.forkSource != source.value.GetRuntimeId() || existing.forkRun != r.GetRunId() {
				return nil, status.Error(codes.AlreadyExists, "client_id reused")
			}
			return proto.Clone(existing.value).(*resource.Session), nil
		}
	}
	state := source.value.GetStatus()
	if state.GetRunId() != r.GetRunId() {
		return nil, status.Error(codes.FailedPrecondition, "stale run_id")
	}
	switch state.GetState() {
	case "idle", "stopped", "interrupted", "failed":
	default:
		return nil, status.Error(codes.FailedPrecondition, "fork requires a settled conversation")
	}
	if len(state.GetPending()) > 0 || state.GetQueued() != "" {
		return nil, status.Error(codes.FailedPrecondition, "resolve pending questions and queued input before forking")
	}
	child := &session{value: proto.Clone(source.value).(*resource.Session), effort: source.effort, scenario: source.scenario, seen: map[string]bool{}, generation: 1, forkSource: source.value.GetRuntimeId(), forkRun: r.GetRunId()}
	id := fmt.Sprintf("session-fork-%d", s.rev+1)
	child.value.SetId(resourceid.Bytes(8, id))
	child.value.SetRuntimeId(id)
	child.value.SetAlias(id)
	child.value.SetClientId(r.GetClientId())
	child.value.SetDateCreated(timestamppb.Now())
	child.value.SetDateUpdated(timestamppb.Now())
	child.value.SetStatus(resource.SessionStatus_builder{State: "idle", RunId: id + "-run-1", VendorId: id + "-vendor", LastSeq: state.GetLastSeq(), PermissionMode: state.GetPermissionMode()}.Build())
	for _, event := range source.events {
		child.events = append(child.events, proto.Clone(event).(*resource.SessionEvent))
	}
	s.sessions = append(s.sessions, child)
	s.state(child, "idle")
	return proto.Clone(child.value).(*resource.Session), nil
}
