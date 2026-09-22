package multiclient

import (
	"context"
	"io"

	"github.com/lesomnus/cxz/internal/accounts"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (c *Client) LoginSession(ctx context.Context, project, account, key string, input io.ReadCloser, output io.Writer) error {
	_, id, client, err := c.route(ctx, project)
	if err != nil {
		return err
	}
	login, ok := client.(accounts.SessionLoginClient)
	if !ok {
		return status.Error(codes.Unimplemented, "session login transport unavailable")
	}
	return login.LoginSession(ctx, id, account, key, input, output)
}
