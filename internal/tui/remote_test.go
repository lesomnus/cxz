package tui

import (
	"context"
	"testing"

	"github.com/lesomnus/cxz/internal/containerterm"
	"github.com/lesomnus/cxz/internal/transport"
)

func TestRemoteWorkspaceAndLocalDockerGuard(t *testing.T) {
	ctx := transport.WithRemote(context.Background())
	if got, err := workspacePath(ctx, "/srv/work/../project"); err != nil || got != "/srv/project" {
		t.Fatal(got, err)
	}
	for _, path := range []string{".", "relative", `C:\work`, "~/work"} {
		if _, err := workspacePath(ctx, path); err == nil {
			t.Fatal("local path accepted", path)
		}
	}
	if _, err := containerterm.Open(ctx, nil, 80, 24, func() {}); err == nil {
		t.Fatal("remote terminal reached local Docker")
	}
	if _, err := containerterm.ListPaths(ctx, nil, "/"); err == nil {
		t.Fatal("remote lookup reached local Docker")
	}
}
