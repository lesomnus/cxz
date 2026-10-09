package multiclient

import (
	"context"

	"github.com/lesomnus/cxz/api"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
)

// The project says which connection writes the file. Nothing parses a payload
// to find it, which also means the secret is never copied into a second buffer
// on the way.

func (c *Client) PutSecretFile(ctx context.Context, r *api.PutSecretFileInput, opts ...grpc.CallOption) (*api.SecretFileReply, error) {
	in := proto.Clone(r).(*api.PutSecretFileInput)
	_, id, client, err := c.route(ctx, r.Project)
	if err != nil {
		return nil, err
	}
	in.Project = id
	defer clear(in.Secret)
	return client.PutSecretFile(ctx, in, opts...)
}

func (c *Client) DeleteSecretFile(ctx context.Context, r *api.DeleteSecretFileInput, opts ...grpc.CallOption) (*api.SecretFileReply, error) {
	in := proto.Clone(r).(*api.DeleteSecretFileInput)
	_, id, client, err := c.route(ctx, r.Project)
	if err != nil {
		return nil, err
	}
	in.Project = id
	return client.DeleteSecretFile(ctx, in, opts...)
}
