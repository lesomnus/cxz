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

func (x *Sessions) Add(_ context.Context, r *resource.SessionAddRequest) (*resource.Session, error) {
	s := x.S
	s.mu.Lock()
	defer s.mu.Unlock()
	if r.GetClientId() == "" {
		return nil, status.Error(codes.InvalidArgument, "client_id required")
	}
	var project *resource.Project
	for _, p := range s.projects {
		if matchProject(p, r.GetProject()) {
			project = p
			break
		}
	}
	if project == nil {
		return nil, status.Error(codes.NotFound, "sandbox project not found")
	}
	for _, st := range s.sessions {
		v := st.value
		if v.GetClientId() == r.GetClientId() {
			if !proto.Equal(v.GetProject(), project) || v.GetName() != r.GetName() || v.GetModel() != r.GetModel() || (r.GetAgent() != "" && v.GetAgent() != r.GetAgent()) {
				return nil, status.Error(codes.AlreadyExists, "client_id reused")
			}
			return proto.CloneOf(v), nil
		}
	}
	id := fmt.Sprintf("session-added-%d", s.rev+1)
	agent := r.GetAgent()
	if agent == "" {
		agent = "codex"
	}
	v := resource.Session_builder{Id: resourceid.Bytes(8, id), RuntimeId: id, Alias: id, Name: r.GetName(), Desc: r.GetDesc(), Agent: agent, Model: r.GetModel(), Project: project, ClientId: r.GetClientId(), DateCreated: timestamppb.Now(), DateUpdated: timestamppb.Now(), Status: resource.SessionStatus_builder{State: "created"}.Build()}.Build()
	s.sessions = append(s.sessions, &session{value: v, effort: "high", scenario: "conversation", seen: map[string]bool{}})
	s.rev++
	return proto.CloneOf(v), nil
}

func (x *Sessions) Start(ctx context.Context, r *resource.SessionControl) (*resource.Session, error) {
	return x.Resume(ctx, r)
}
