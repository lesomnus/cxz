//go:build !windows

package main

import (
	"context"
	"fmt"
	"io"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/accounts"
	"google.golang.org/protobuf/proto"
)

func openWithProjectLogin(ctx context.Context, request *api.ProjectRequest, interactive bool,
	open func(context.Context, *api.ProjectRequest) (*api.Session, error),
	login func(context.Context, string, string) error,
	progress ...io.Writer,
) (*api.Session, error) {
	if interactive && len(progress) > 0 && progress[0] != nil {
		original := open
		open = func(ctx context.Context, r *api.ProjectRequest) (*api.Session, error) {
			var session *api.Session
			err := sessionProgress(ctx, progress[0], func() error { var err error; session, err = original(ctx, r); return err })
			return session, err
		}
	}
	s, err := open(ctx, request)
	if err == nil {
		return s, nil
	}
	alias, creationKey, required := accounts.RequiredProjectLogin(err, request.Account, request.ClientId)
	if !required {
		return nil, err
	}
	if !interactive {
		return nil, fmt.Errorf("workspace prepared; account %s needs independent session login; create the session interactively with cxz up (existing sessions: cxz account login --session <session> %s): %w", alias, alias, err)
	}
	if err := login(ctx, alias, creationKey); err != nil {
		return nil, err
	}
	// Provisioning/recreation already succeeded. Never repeat a destructive
	// recreate after login, and retain session identity/idempotency on retry.
	retry := proto.Clone(request).(*api.ProjectRequest)
	retry.Recreate = false
	retry.Account = alias
	return open(ctx, retry)
}
