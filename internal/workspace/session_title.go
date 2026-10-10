package workspace

import (
	"context"

	"github.com/lesomnus/cxz/internal/sessiontitle"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// SetSessionTitle stores metadata for an already resolved session. It cancels
// in-flight title generation and marks the title as manual, without running AI.
func (m *Manager) SetSessionTitle(ctx context.Context, id, text string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if id == "" || len(text) > sessiontitle.MaxInputBytes {
		return "", status.Error(codes.InvalidArgument, "invalid session title")
	}
	text = sessiontitle.Normalize(text)
	if text == "" {
		return "", status.Error(codes.InvalidArgument, "title must not be empty")
	}
	c, err := m.auxiliaryController()
	if err != nil {
		return "", err
	}
	if err := c.SetTitle(id, text); err != nil {
		return "", err
	}
	return text, nil
}
