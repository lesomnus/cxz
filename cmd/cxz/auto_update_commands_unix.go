//go:build !windows

package main

import (
	"context"
	"github.com/lesomnus/cxz/internal/dockerx"
	"github.com/lesomnus/cxz/internal/transport"
)

func serverUpdateCommand(ctx context.Context, root, action string) ([]byte, error) {
	v, e := transport.Load(root)
	if e != nil {
		return nil, e
	}
	if _, e = dockerx.Owned(ctx, v.Container, v.Owner, ""); e != nil {
		return nil, e
	}
	// Applying needs the container and owner, and this is the side that knows
	// them: the manager's own root says nothing about what runs it.
	if action == "apply" || action == "apply-force" {
		forced := "sweep"
		if action == "apply-force" {
			forced = "force"
		}
		return dockerx.Run(ctx, "exec", v.Container, "/usr/local/bin/cxz", "_rollout-apply", "/var/lib/cxz", v.Container, v.Owner, forced)
	}
	return dockerx.Run(ctx, "exec", v.Container, "/usr/local/bin/cxz", "_update-policy", "/var/lib/cxz", action)
}

func serverRetryRuntime(ctx context.Context, root, project string) ([]byte, error) {
	v, err := transport.Load(root)
	if err != nil {
		return nil, err
	}
	if _, err := dockerx.Owned(ctx, v.Container, v.Owner, ""); err != nil {
		return nil, err
	}
	return dockerx.Run(ctx, "exec", v.Container, "/usr/local/bin/cxz", "_runtime-recovery-retry", "/var/lib/cxz", project)
}
