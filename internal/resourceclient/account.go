package resourceclient

import (
	"context"
	"fmt"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func ar(alias string) *resource.AccountRef {
	if alias == "" {
		return nil
	}
	return resource.AccountRef_builder{Alias: &alias}.Build()
}
func (c *Client) Account(ctx context.Context, alias string) (*resource.Account, error) {
	return c.Accounts.Get(ctx, resource.AccountGetRequest_builder{Ref: ar(alias), Select: resource.AccountSelect_builder{All: ptr(true)}.Build()}.Build())
}

// EnsureAccount mirrors only selected metadata into the project resource DB.
func (c *Client) EnsureAccount(ctx context.Context, alias, agent string) error {
	a, err := c.Account(ctx, alias)
	if status.Code(err) == codes.NotFound {
		a, err = c.Accounts.Add(ctx, resource.AccountAddRequest_builder{Alias: alias, Name: alias, Agent: agent}.Build())
		if status.Code(err) == codes.AlreadyExists {
			a, err = c.Account(ctx, alias)
		}
	}
	if err != nil {
		return err
	}
	if a.GetAgent() != agent {
		return fmt.Errorf("account agent mismatch")
	}
	return nil
}
