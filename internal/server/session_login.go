package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"os"

	"github.com/lesomnus/cxz/internal/accounts"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/workspace"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *Server) LoginSession(ctx context.Context, project, account, key string, input io.ReadCloser, output io.Writer) error {
	if s.manager != nil {
		return s.manager.LoginSession(ctx, project, account, key, input, output)
	}
	runtime, err := workspace.LoadRuntime(s.root)
	if err != nil {
		return err
	}
	if runtime.ProjectID != project {
		return status.Error(codes.PermissionDenied, "login project does not match runtime")
	}
	if runtime.Claude == "" {
		return status.Error(codes.FailedPrecondition, "Claude is not provisioned in this project")
	}
	if key == "" || len(key) > 1024 {
		return status.Error(codes.InvalidArgument, "session creation key required")
	}
	var raw []byte
	err = s.db.QueryRowContext(ctx, "SELECT manifest FROM sessions WHERE create_id=?", key).Scan(&raw)
	if err == nil {
		var session core.Session
		if err := json.Unmarshal(raw, &session); err != nil {
			return err
		}
		if session.ProjectID != project || session.Account != account || session.Kind != "claude" || session.AuthBackend != accounts.ProjectLocalOAuth {
			return status.Error(codes.PermissionDenied, "login account does not match session")
		}
		v, err := s.snapshot(ctx, session)
		if err != nil {
			return err
		}
		if v.State != "stopped" && v.State != "failed" && v.State != "interrupted" {
			return status.Error(codes.FailedPrecondition, "stop this session before logging in again")
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	backend, _ := accounts.Resolve("claude", accounts.ProjectLocalOAuth)
	return backend.Login(ctx, accounts.LoginRequest{Root: accounts.SessionRoot(s.root, key), Account: account, Workspace: runtime.Workspace, Binary: runtime.Claude, Env: os.Environ(), Input: input, Output: output, Error: output})
}
