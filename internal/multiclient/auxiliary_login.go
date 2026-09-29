package multiclient

import (
	"context"
	"io"

	"github.com/lesomnus/cxz/internal/accounts"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (c *Client) LoginAuxiliary(ctx context.Context, account string, input io.ReadCloser, output io.Writer) error {
	_, _, client, err := c.route(ctx, "")
	if err != nil {
		return err
	}
	login, ok := client.(accounts.AuxiliaryLoginClient)
	if !ok {
		return status.Error(codes.Unimplemented, "auxiliary login transport unavailable")
	}
	return login.LoginAuxiliary(ctx, account, input, output)
}
