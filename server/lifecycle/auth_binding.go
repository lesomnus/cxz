package lifecycle

import (
	"context"
	"github.com/lesomnus/cxz/internal/accounts"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type AuthBindingServer struct {
	Layer
	resource.AuthBindingServiceServer
}

func (s Layer) AuthBinding() resource.AuthBindingServiceServer {
	return AuthBindingServer{s, s.Next().AuthBinding()}
}
func bindingRef(id string) *resource.AuthBindingRef {
	return resource.AuthBindingRef_builder{BindingId: &id}.Build()
}
func (s AuthBindingServer) Add(ctx context.Context, r *resource.AuthBindingAddRequest) (*resource.AuthBinding, error) {
	if err := s.effect(); err != nil {
		return nil, err
	}
	if r.HasId() || r.GetBindingId() != "" || r.GetScope() != "" || r.GetCredentialRef() != "" || r.HasDateCreated() {
		return nil, closed()
	}
	a, err := s.Next().Account().Get(ctx, resource.AccountGetRequest_builder{Ref: r.GetAccount(), Select: resource.AccountSelect_builder{All: ptr(true)}.Build()}.Build())
	if err != nil {
		return nil, err
	}
	p, err := s.Next().Project().Get(ctx, resource.ProjectGetRequest_builder{Ref: r.GetProject(), Select: resource.ProjectSelect_builder{All: ptr(true)}.Build()}.Build())
	if err != nil {
		return nil, err
	}
	if r.GetAuthBackend() != "" && r.GetAuthBackend() != a.GetAuthBackend() {
		return nil, status.Error(codes.InvalidArgument, "binding backend must match account")
	}
	s.shared.mu.Lock()
	defer s.shared.mu.Unlock()
	return s.ensureBinding(ctx, p.GetRuntimeId(), a.GetAlias(), a.GetAgent(), a.GetAuthBackend())
}

// Caller holds shared.mu. The unique binding ID makes retries converge.
func (s Layer) ensureBinding(ctx context.Context, project, alias, agent, backend string) (*resource.AuthBinding, error) {
	b, err := accounts.Resolve(agent, backend)
	if err != nil {
		return nil, status.Error(codes.FailedPrecondition, err.Error())
	}
	if b.Info().Scope != "project" {
		return nil, status.Error(codes.Unimplemented, "binding scope not implemented")
	}
	spec, err := b.Binding(project, alias)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	id := spec.ID
	v, err := s.Next().AuthBinding().Get(ctx, resource.AuthBindingGetRequest_builder{Ref: bindingRef(id), Select: resource.AuthBindingSelect_builder{All: ptr(true)}.Build()}.Build())
	if err == nil {
		return v, nil
	}
	if status.Code(err) != codes.NotFound {
		return nil, err
	}
	return s.Next().AuthBinding().Add(ctx, resource.AuthBindingAddRequest_builder{Id: resourceID(10, id), BindingId: id, Account: accountRef(alias), Project: projectRef(project), AuthBackend: backend, Scope: spec.Scope, CredentialRef: spec.CredentialRef}.Build())
}
func (s AuthBindingServer) Patch(context.Context, *resource.AuthBindingPatchRequest) (*resource.AuthBinding, error) {
	return nil, closed()
}
func (s AuthBindingServer) Apply(context.Context, *resource.AuthBindingApplyRequest) (*resource.AuthBinding, error) {
	return nil, closed()
}
func (s AuthBindingServer) Erase(context.Context, *resource.AuthBindingRef) (*resource.AuthBindingEraseResponse, error) {
	return nil, closed()
}
