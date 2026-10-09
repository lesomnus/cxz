package multiclient

import (
	"context"

	"github.com/lesomnus/cxz/api"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
)

// An installation has one shared engine, so every one of these follows the
// connection being looked at. Nothing reads a payload to decide: the envelope's
// routing used to parse the MCP request out of a Docker spec to learn which
// connection the call belonged to, and serialise it again with the prefix
// stripped.

func (c *Client) SaveEngine(ctx context.Context, r *api.SaveEngineInput, opts ...grpc.CallOption) (*api.EngineReply, error) {
	return c.engineChange(ctx, func(client api.SessionsClient) (*api.EngineReply, error) {
		return client.SaveEngine(ctx, proto.Clone(r).(*api.SaveEngineInput), opts...)
	})
}

func (c *Client) StartEngine(ctx context.Context, r *api.StartEngineInput, opts ...grpc.CallOption) (*api.EngineReply, error) {
	return c.engineChange(ctx, func(client api.SessionsClient) (*api.EngineReply, error) {
		return client.StartEngine(ctx, proto.Clone(r).(*api.StartEngineInput), opts...)
	})
}

func (c *Client) StopEngine(ctx context.Context, r *api.Empty, opts ...grpc.CallOption) (*api.EngineReply, error) {
	return c.engineChange(ctx, func(client api.SessionsClient) (*api.EngineReply, error) {
		return client.StopEngine(ctx, r, opts...)
	})
}

func (c *Client) PruneEngine(ctx context.Context, r *api.Empty, opts ...grpc.CallOption) (*api.EngineReply, error) {
	return c.engineChange(ctx, func(client api.SessionsClient) (*api.EngineReply, error) {
		return client.PruneEngine(ctx, r, opts...)
	})
}

// A status is a read, so it does not refresh anything. Under the envelope only
// info was exempt, which meant a status read woke every watcher.
func (c *Client) EngineStatus(ctx context.Context, r *api.Empty, opts ...grpc.CallOption) (*api.EngineReply, error) {
	_, _, client, err := c.route(ctx, "")
	if err != nil {
		return nil, err
	}
	return client.EngineStatus(ctx, r, opts...)
}

func (c *Client) GetEngineInfo(ctx context.Context, r *api.Empty, opts ...grpc.CallOption) (*api.EngineInfo, error) {
	_, _, client, err := c.route(ctx, "")
	if err != nil {
		return nil, err
	}
	return client.GetEngineInfo(ctx, r, opts...)
}

func (c *Client) GetInstallationVersion(ctx context.Context, r *api.Empty, opts ...grpc.CallOption) (*api.InstallationVersion, error) {
	_, _, client, err := c.route(ctx, "")
	if err != nil {
		return nil, err
	}
	return client.GetInstallationVersion(ctx, r, opts...)
}

func (c *Client) engineChange(ctx context.Context, call func(api.SessionsClient) (*api.EngineReply, error)) (*api.EngineReply, error) {
	_, _, client, err := c.route(ctx, "")
	if err != nil {
		return nil, err
	}
	out, err := call(client)
	if err == nil {
		c.refreshSource(ctx, "")
	}
	return out, err
}
