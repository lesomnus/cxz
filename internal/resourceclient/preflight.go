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

var ErrAccountRequired = errors.New("no matching session to resume; select an account with --account ACCOUNT (list profiles with cxz account ls)")

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
		// A project with no sessions at all is recreated as a project, the way
		// `recreate @all` already does it: the container is replaced and
		// nothing is resumed, because there is nothing to resume. Requiring an
		// account here meant the only way to recreate such a project was to let
		// it create a session nobody asked for -- and the one project in that
		// state could not have its state migrated at all. A project that has
		// sessions but none that match is a different question, and is still
		// asked. See cxz#263.
		if r.Recreate && !r.NewSession && !hasSessions(list.Sessions, p.Id) {
			r.Agent = kind
			r.PrepareOnly = true
			return nil
		}
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

// hasSessions reports whether the project has any session at all, live,
// stopped or failed. Recreating is about the container; resuming is about
// sessions, and a project with none has nothing to resume.
func hasSessions(list []*api.Session, project string) bool {
	for _, s := range list {
		if s.ProjectId == project {
			return true
		}
	}
	return false
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
		if s.ProjectId != project || s.Agent != kind || (r.Account != "" && s.Account != r.Account) {
			continue
		}
		if r.NewSession {
			if r.ClientId != "" && s.CreateId == r.ClientId {
				chosen = s
				break
			}
			continue
		}
		if isLive(s) && !beforeRecreate {
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
