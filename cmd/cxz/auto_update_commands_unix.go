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
	return dockerx.Run(ctx, "exec", v.Container, "/usr/local/bin/cxz", "_update-policy", "/var/lib/cxz", action)
}
