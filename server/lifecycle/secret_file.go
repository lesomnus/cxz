package lifecycle

import (
	"context"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/resource"
)

// Two methods rather than one with a verb, so that the one carrying a secret is
// the one named for it. Whether the link may carry it is settled before the
// call is made -- see internal/transport.Confidential -- because the exposed
// TCP surface cannot be told apart from a local socket on this side.

func (s ProjectServer) PutSecretFile(ctx context.Context, r *resource.PutSecretFileRequest) (*resource.SecretFileReply, error) {
	if err := s.effect(); err != nil {
		return nil, err
	}
	v, err := s.ProjectServiceServer.Get(ctx, resource.ProjectGetRequest_builder{
		Ref: r.GetRef(), Select: resource.ProjectSelect_builder{All: ptr(true)}.Build(),
	}.Build())
	if err != nil {
		return nil, err
	}
	secret := r.GetSecret()
	defer clear(secret)
	out, err := s.shared.runtime.PutSecretFile(ctx, &api.PutSecretFileInput{
		Project: v.GetRuntimeId(), Session: r.GetSession(), Secret: secret,
	})
	if err != nil {
		return nil, err
	}
	return resource.SecretFileReply_builder{Path: ptr(out.Path)}.Build(), nil
}

func (s ProjectServer) DeleteSecretFile(ctx context.Context, r *resource.DeleteSecretFileRequest) (*resource.SecretFileReply, error) {
	if err := s.effect(); err != nil {
		return nil, err
	}
	v, err := s.ProjectServiceServer.Get(ctx, resource.ProjectGetRequest_builder{
		Ref: r.GetRef(), Select: resource.ProjectSelect_builder{All: ptr(true)}.Build(),
	}.Build())
	if err != nil {
		return nil, err
	}
	out, err := s.shared.runtime.DeleteSecretFile(ctx, &api.DeleteSecretFileInput{
		Project: v.GetRuntimeId(), Path: r.GetPath(),
	})
	if err != nil {
		return nil, err
	}
	return resource.SecretFileReply_builder{Path: ptr(out.Path)}.Build(), nil
}
