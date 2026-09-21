package main

import (
	"context"

	"github.com/lesomnus/xli"
)

func selfUpdateHandler() xli.Handler {
	return xli.OnRun(func(ctx context.Context, c *xli.Command, _ xli.Next) error {
		return runSelfUpdate(ctx, c)
	})
}
