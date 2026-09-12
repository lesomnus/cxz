package resourceclient

import (
	"context"
	"errors"
	"fmt"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/accounts"
	"github.com/lesomnus/cxz/internal/settings"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var ErrAccountRequired = errors.New("no matching session to resume; select an account with --account ACCOUNT (list profiles with cxz account list)")

// CheckOpen performs read-only prerequisite checks before project registration,
// provisioning or destructive recreation. Open checks again for non-CLI callers.
// Vendor login validity may still require the prepared project runtime.
func (c *Client) CheckOpen(ctx context.Context, r *api.ProjectRequest, opts ...grpc.CallOption) error {
	if err := settings.ValidateModel(r.Model); err != nil {
		return err
	}
	if r.Account != "" {
		a, err := c.Account(ctx, r.Account)
		if err != nil {
			return err
		}
		if r.Agent != "" && r.Agent != a.GetAgent() {
			return fmt.Errorf("agent does not match account %s", r.Account)
		}
		if _, err = accounts.Resolve(a.GetAgent(), a.GetAuthBackend()); err != nil {
			return err
		}
		r.Agent = a.GetAgent()
	}
	if r.Agent != "" && r.Agent != "claude" && r.Agent != "codex" {
		return fmt.Errorf("unknown agent %q", r.Agent)
	}
	if r.PrepareOnly {
		return nil
	}
	p, err := c.ResolveProject(ctx, r.Workspace, opts...)
	if status.Code(err) == codes.NotFound {
		if r.Account == "" {
			return ErrAccountRequired
		}
		return nil
	}
	if err != nil {
		return err
	}
	list, err := c.List(ctx, &api.Empty{}, opts...)
	if err != nil {
		return err
	}
	kind, chosen, err := chooseSession(list.Sessions, p.Id, r, r.Recreate)
	if err != nil {
		return err
	}
	if chosen == nil {
		if r.Account == "" {
			return ErrAccountRequired
		}
	} else {
		if chosen.Account == "" {
			return fmt.Errorf("existing session has no account; create a new session with --account")
		}
		a, err := c.Account(ctx, chosen.Account)
		if err != nil {
			return err
		}
		if a.GetAgent() != chosen.Agent {
			return fmt.Errorf("existing session account/agent mismatch")
		}
		if _, err = accounts.Resolve(a.GetAgent(), a.GetAuthBackend()); err != nil {
			return err
		}
		r.Account = chosen.Account
	}
	r.Agent = kind
	return nil
}

func isLive(s *api.Session) bool {
	return s.State == "idle" || s.State == "working" || s.State == "waiting_input" || s.State == "starting"
}

// beforeRecreate models the explicit stop performed by Project.Recreate.
func chooseSession(list []*api.Session, project string, r *api.ProjectRequest, beforeRecreate bool) (string, *api.Session, error) {
	kind := r.Agent
	if kind == "" {
		for _, s := range list {
			if s.ProjectId == project && isLive(s) {
				kind = s.Agent
				break
			}
		}
		if kind == "" {
			for _, s := range list {
				if s.ProjectId == project {
					kind = s.Agent
					break
				}
			}
		}
	}
	if kind == "" {
		kind = "claude"
	}
	var chosen *api.Session
	for _, s := range list {
		if s.ProjectId != project {
			continue
		}
		if isLive(s) && !beforeRecreate {
			if r.NewSession || s.Agent != kind || (r.Account != "" && s.Account != r.Account) {
				if r.NewSession && s.Agent == kind && s.Account == r.Account && s.CreateId == r.ClientId {
					chosen = s
					break
				}
				return kind, nil, fmt.Errorf("workspace has an active %s session %s; stop it explicitly before starting another", s.Agent, s.Id)
			}
			chosen = s
			break
		}
		if !r.NewSession && chosen == nil && s.Agent == kind && (r.Account == "" || s.Account == r.Account) {
			chosen = s
		}
	}
	if chosen != nil && r.Model != "" && r.Model != chosen.Model {
		return kind, nil, fmt.Errorf("existing session model is immutable; stop it and create a new session")
	}
	return kind, chosen, nil
}
