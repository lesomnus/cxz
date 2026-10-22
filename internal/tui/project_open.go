package tui

import (
	"context"

	"github.com/lesomnus/cxz/api"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Opening a project is a composition rather than a call: create the container,
// then decide whether a session is wanted. The manager's half is
// CreateContainer, so the method lives on the client that composes them and is
// asked for by shape -- the same way internal/multiclient asks.
// See docs/plans/project-lifecycle-api.md.
type projectOpener interface {
	Open(context.Context, *api.ProjectRequest, ...grpc.CallOption) (*api.Session, error)
}

func openProject(ctx context.Context, c api.SessionsClient, r *api.ProjectRequest) (*api.Session, error) {
	opener, ok := c.(projectOpener)
	if !ok {
		return nil, status.Error(codes.Unimplemented, "this endpoint cannot open projects; update cxz")
	}
	return opener.Open(ctx, r)
}
