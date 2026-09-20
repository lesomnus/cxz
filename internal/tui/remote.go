package tui

import (
	"context"
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"github.com/lesomnus/cxz/internal/transport"
)

func workspacePath(ctx context.Context, input string) (string, error) {
	if transport.IsRemote(ctx) {
		if !strings.HasPrefix(input, "/") || strings.ContainsAny(input, "\x00\r\n") {
			return "", fmt.Errorf("enter an absolute Linux workspace path on the remote daemon host")
		}
		return path.Clean(input), nil
	}
	return filepath.Abs(input)
}
