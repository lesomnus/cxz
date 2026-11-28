package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/lesomnus/cxz/internal/projectid"
	"os"

	"github.com/lesomnus/cxz/internal/accounts"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/workspace"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *Server) LoginSession(ctx context.Context, r accounts.SessionLogin) (accounts.SessionLoginResult, error) {
	if s.manager != nil {
		return s.manager.LoginSession(ctx, r)
	}
	var out accounts.SessionLoginResult
	runtime, err := workspace.LoadRuntime(s.root)
	if err != nil {
		return out, err
	}
	if !projectid.Same(runtime.ProjectID, r.Project) {
		return out, status.Error(codes.PermissionDenied, "login project does not match runtime")
	}
	binary, err := runtime.AgentBinary("claude")
	if err != nil {
		return out, status.Error(codes.FailedPrecondition, err.Error())
	}
	if r.Key == "" || len(r.Key) > 1024 {
		return out, status.Error(codes.InvalidArgument, "session creation key required")
	}
	var raw []byte
	err = s.db.QueryRowContext(ctx, "SELECT manifest FROM sessions WHERE create_id=?", r.Key).Scan(&raw)
	if err == nil {
		var session core.Session
		if err := json.Unmarshal(raw, &session); err != nil {
			return out, err
		}
		if !projectid.Same(session.ProjectID, r.Project) || session.Account != r.Account || session.Kind != "claude" || session.AuthBackend != accounts.ProjectLocalOAuth {
			return out, status.Error(codes.PermissionDenied, "login account does not match session")
		}
		v, err := s.snapshot(ctx, session)
		if err != nil {
			return out, err
		}
		if v.State != "stopped" && v.State != "failed" && v.State != "interrupted" {
			return out, status.Error(codes.FailedPrecondition, "stop this session before logging in again")
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return out, err
	}
	backend, _ := accounts.Resolve("claude", accounts.ProjectLocalOAuth)
	// The credential is still in the login's staging directory here, so a
	// refusal leaves the account exactly as it was rather than needing the
	// wrong account to be undone afterwards.
	validate := func(credential []byte) error {
		observed, err := accounts.CredentialIdentity("claude", credential)
		if err != nil {
			return err
		}
		record, err := accounts.PinnedIdentity(r.Identity, observed)
		if err != nil {
			return status.Error(codes.FailedPrecondition, err.Error())
		}
		out.Identity = record
		return nil
	}
	err = backend.Login(ctx, accounts.LoginRequest{Root: accounts.SessionRoot(s.root, r.Key), Account: r.Account, Workspace: runtime.Workspace, Binary: binary, Env: os.Environ(), Input: r.Input, Output: r.Output, Error: r.Output, ValidateCredential: validate})
	if err != nil {
		return accounts.SessionLoginResult{}, err
	}
	return out, nil
}
