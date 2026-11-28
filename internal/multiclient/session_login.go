package multiclient

import (
	"context"

	"github.com/lesomnus/cxz/internal/accounts"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (c *Client) LoginSession(ctx context.Context, r accounts.SessionLogin) (accounts.SessionLoginResult, error) {
	_, id, client, err := c.route(ctx, r.Project)
	if err != nil {
		return accounts.SessionLoginResult{}, err
	}
	login, ok := client.(accounts.SessionLoginClient)
	if !ok {
		return accounts.SessionLoginResult{}, status.Error(codes.Unimplemented, "session login transport unavailable")
	}
	r.Project = id
	return login.LoginSession(ctx, r)
}
