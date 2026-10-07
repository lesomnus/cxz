package server

import (
	"context"
	"io"

	"github.com/lesomnus/cxz/internal/editor"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *Server) Editor(ctx context.Context, project string) (editor.Result, error) {
	if s.manager == nil {
		return editor.Result{}, status.Error(codes.FailedPrecondition, "browser editor requires the manager endpoint")
	}
	return s.manager.Editor(ctx, project)
}
func (s *Server) OpenEditorTunnel(ctx context.Context, project string) (io.ReadWriteCloser, error) {
	if s.manager == nil {
		return nil, status.Error(codes.FailedPrecondition, "browser editor requires the manager endpoint")
	}
	return s.manager.OpenEditorTunnel(ctx, project)
}
