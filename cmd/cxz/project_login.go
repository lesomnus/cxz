package main

import (
	"context"
	"fmt"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/accounts"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func openWithProjectLogin(ctx context.Context, request *api.ProjectRequest, interactive bool,
	open func(context.Context, *api.ProjectRequest) (*api.Session, error),
	login func(context.Context, string) error,
) (*api.Session, error) {
	s, err := open(ctx, request)
	if err == nil {
		return s, nil
	}
	alias := ""
	st := status.Convert(err)
	if st.Code() == codes.FailedPrecondition {
		for _, detail := range st.Details() {
			if info, ok := detail.(*errdetails.ErrorInfo); ok && info.Domain == "cxz.auth" && info.Reason == "PROJECT_LOGIN_REQUIRED" {
				alias = info.Metadata["account"]
			}
		}
	}
	if accounts.Validate(alias, "claude") != nil || (request.Account != "" && request.Account != alias) {
		return nil, err
	}
	if !interactive {
		return nil, fmt.Errorf("workspace prepared; account %s needs project login; run cxz account login --project %q %s, then retry: %w", alias, request.Workspace, alias, err)
	}
	if err := login(ctx, alias); err != nil {
		return nil, err
	}
	// Provisioning/recreation already succeeded. Never repeat a destructive
	// recreate after login, and retain session identity/idempotency on retry.
	retry := proto.Clone(request).(*api.ProjectRequest)
	retry.Recreate = false
	retry.Account = alias
	return open(ctx, retry)
}
