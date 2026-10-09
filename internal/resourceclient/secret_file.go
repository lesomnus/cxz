package resourceclient

import (
	"context"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc"
)

func (c *Client) PutSecretFile(ctx context.Context, r *api.PutSecretFileInput, opts ...grpc.CallOption) (*api.SecretFileReply, error) {
	v, err := c.projects.PutSecretFile(ctx, resource.PutSecretFileRequest_builder{
		Ref: pr(r.Project), Session: &r.Session, Secret: r.Secret,
	}.Build(), opts...)
	if err != nil {
		return nil, err
	}
	return &api.SecretFileReply{Path: v.GetPath()}, nil
}

func (c *Client) DeleteSecretFile(ctx context.Context, r *api.DeleteSecretFileInput, opts ...grpc.CallOption) (*api.SecretFileReply, error) {
	v, err := c.projects.DeleteSecretFile(ctx, resource.DeleteSecretFileRequest_builder{
		Ref: pr(r.Project), Path: &r.Path,
	}.Build(), opts...)
	if err != nil {
		return nil, err
	}
	return &api.SecretFileReply{Path: v.GetPath()}, nil
}
