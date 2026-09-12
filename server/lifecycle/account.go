package lifecycle

import (
	"context"
	"github.com/lesomnus/cxz/internal/accounts"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type AccountServer struct {
	Layer
	resource.AccountServiceServer
}

func (s Layer) Account() resource.AccountServiceServer { return AccountServer{s, s.Next().Account()} }
func accountRef(alias string) *resource.AccountRef {
	return resource.AccountRef_builder{Alias: &alias}.Build()
}
func (s AccountServer) Add(ctx context.Context, r *resource.AccountAddRequest) (*resource.Account, error) {
	if err := s.effect(); err != nil {
		return nil, err
	}
	if r.HasId() || r.HasDateCreated() {
		return nil, closed()
	}
	if err := accounts.Validate(r.GetAlias(), r.GetAgent()); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	name := r.GetName()
	if name == "" {
		name = r.GetAlias()
	}
	if err := validName(name); err != nil {
		return nil, err
	}
	s.shared.mu.Lock()
	defer s.shared.mu.Unlock()
	return s.Next().Account().Add(ctx, resource.AccountAddRequest_builder{Id: resourceID(9, r.GetAlias()), Alias: r.GetAlias(), Name: name, Desc: r.GetDesc(), Agent: r.GetAgent()}.Build())
}
func (s AccountServer) Patch(context.Context, *resource.AccountPatchRequest) (*resource.Account, error) {
	return nil, closed()
}
func (s AccountServer) Apply(context.Context, *resource.AccountApplyRequest) (*resource.Account, error) {
	return nil, closed()
}
func (s AccountServer) Erase(context.Context, *resource.AccountRef) (*resource.AccountEraseResponse, error) {
	return nil, closed()
}

func (s Layer) ensureAccount(ctx context.Context, alias, agent string) error {
	if alias == "" {
		return nil
	}
	v, err := s.Next().Account().Get(ctx, resource.AccountGetRequest_builder{Ref: accountRef(alias), Select: resource.AccountSelect_builder{All: ptr(true)}.Build()}.Build())
	if status.Code(err) == codes.NotFound {
		_, err = s.Next().Account().Add(ctx, resource.AccountAddRequest_builder{Id: resourceID(9, alias), Alias: alias, Name: alias, Agent: agent}.Build())
		return err
	}
	if err == nil && v.GetAgent() != agent {
		return status.Error(codes.FailedPrecondition, "account agent mismatch")
	}
	return err
}
