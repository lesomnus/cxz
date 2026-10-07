package server

import (
	"context"
	"encoding/json"
	"os"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/historypolicy"
	"github.com/lesomnus/cxz/internal/sessionpurge"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// purgeSession destroys one session's data inside the runtime that holds it. By
// the time this runs the caller has already stopped the agent and stopped
// listing the session, so nothing here has to negotiate with a live turn.
//
// The database goes before the files. Its tables are derived -- a restart
// rebuilds them from the manifests still on disk -- so dropping them first
// stops this process from serving a session whose journal is about to vanish,
// and a purge interrupted after that point is simply run again.
func (s *Server) purgeSession(ctx context.Context, spec []byte) (*api.Receipt, error) {
	var r sessionpurge.Request
	if len(spec) > sessionpurge.MaxSpec || json.Unmarshal(spec, &r) != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid session purge request")
	}
	if err := s.requireManagerAuthority(ctx, "purge a session from the host client, not from inside a project"); err != nil {
		return nil, err
	}
	m, err := s.manifest(ctx, r.Session)
	if err != nil {
		return nil, err
	}
	subject := sessionpurge.Subject{Session: m.ID, CreateID: m.CreateID, Project: m.ProjectID}
	// A project runtime holds the conversation while the manager holds the
	// uploads, and each purges its own root. A standalone server has no manager
	// above it, so there it is one root and both scopes belong to this pass.
	scopes := []sessionpurge.Scope{sessionpurge.Runtime}
	if !s.insideProject() {
		scopes = append(scopes, sessionpurge.Manager)
	}
	if !r.DryRun {
		if err := s.forgetSession(ctx, m.ID); err != nil {
			return nil, err
		}
	}
	out := sessionpurge.Reply{Session: m.ID, DryRun: r.DryRun}
	for _, scope := range scopes {
		step := sessionpurge.Execute
		if r.DryRun {
			step = sessionpurge.Plan
		}
		reply, err := step(ctx, scope, s.root, subject)
		if err != nil {
			return nil, err
		}
		out.Targets = append(out.Targets, reply.Targets...)
	}
	return receipt(out)
}

type managerAuthorityKey struct{}

func (s *Server) insideProject() bool { return os.Getenv("CXZ_PROJECT_ID") != "" }

// requireManagerAuthority keeps a project container from driving an operation
// that is the host's. A project runtime answers on two surfaces: the manager's
// token-authenticated channel, and a local socket the agent inside the container
// can reach. An agent that may read a journal must not also be able to unlink
// it, and a purge driven from in there would silently skip the uploads that only
// the manager can see.
func (s *Server) requireManagerAuthority(ctx context.Context, refusal string) error {
	if !s.insideProject() {
		return nil
	}
	if authorized, _ := ctx.Value(managerAuthorityKey{}).(bool); authorized {
		return nil
	}
	return status.Error(codes.PermissionDenied, refusal)
}

// forgetSession drops the derived projection, in memory and in the database. The
// per-session lock is taken so a reader mid-projection finishes first rather
// than committing rows back behind the delete.
func (s *Server) forgetSession(ctx context.Context, id string) error {
	s.projectionMu.Lock()
	p := s.projections[id]
	delete(s.projections, id)
	s.projectionMu.Unlock()
	if p != nil {
		p.mu.Lock()
		defer p.mu.Unlock()
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = historypolicy.ForgetFloor(ctx, tx, id); err != nil {
		return err
	}
	for _, q := range []string{"DELETE FROM events WHERE session_id=?", "DELETE FROM sessions WHERE id=?"} {
		if _, err = tx.ExecContext(ctx, q, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func receipt(reply sessionpurge.Reply) (*api.Receipt, error) {
	b, err := json.Marshal(reply)
	return &api.Receipt{Status: string(b)}, err
}
