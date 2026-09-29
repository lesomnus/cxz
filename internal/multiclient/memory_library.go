package multiclient

import (
	"context"
	"github.com/lesomnus/cxz/internal/memorylib"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (c *Client) Library(ctx context.Context, session string, q memorylib.Request) (memorylib.Reply, error) {
	_, id, client, e := c.route(ctx, session)
	if e != nil {
		return memorylib.Reply{}, e
	}
	if c, ok := client.(memorylib.Client); ok {
		return c.Library(ctx, id, q)
	}
	return memorylib.Reply{}, status.Error(codes.Unimplemented, "update manager for shared memory")
}
