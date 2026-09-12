package resourceclient

import (
	"context"
	"fmt"
	"github.com/lesomnus/cxz/internal/accounts"
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
func (c *Client) EnsureAccount(ctx context.Context, alias, agent string, selected ...string) error {
	backend := ""
	if len(selected) > 0 {
		backend = selected[0]
	}
	b, e := accounts.Select(agent, backend)
	if e != nil {
		return e
	}
	backend = b.Info().ID
	a, err := c.Account(ctx, alias)
	if status.Code(err) == codes.NotFound {
		a, err = c.Accounts.Add(ctx, resource.AccountAddRequest_builder{Alias: alias, Name: alias, Agent: agent, AuthBackend: backend}.Build())
		if status.Code(err) == codes.AlreadyExists {
			a, err = c.Account(ctx, alias)
		}
	}
	if err != nil {
		return err
	}
	if a.GetAgent() != agent || a.GetAuthBackend() != backend {
		return fmt.Errorf("account agent mismatch")
	}
	return nil
}
func br(id string) *resource.AuthBindingRef {
	return resource.AuthBindingRef_builder{BindingId: &id}.Build()
}
func (c *Client) Bind(ctx context.Context, project, account string) (*resource.AuthBinding, error) {
	return c.Bindings.Add(ctx, resource.AuthBindingAddRequest_builder{Project: pr(project), Account: ar(account)}.Build())
}
