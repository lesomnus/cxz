package multiclient

import (
	"context"

	"github.com/lesomnus/cxz/internal/containerterm"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (c *Client) OpenTerminal(ctx context.Context, project string, columns, rows int) (containerterm.Terminal, error) {
	_, id, client, err := c.route(ctx, project)
	if err != nil {
		return nil, err
	}
	terminal, ok := client.(containerterm.TerminalClient)
	if !ok {
		return nil, status.Error(codes.Unimplemented, "container terminal transport unavailable; update cxz on the daemon host")
	}
	return terminal.OpenTerminal(ctx, id, columns, rows)
}
