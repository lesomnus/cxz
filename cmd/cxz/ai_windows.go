//go:build windows

package main

import (
	"context"
	"fmt"
	"github.com/lesomnus/cxz/internal/auxiliary"
	"github.com/lesomnus/xli"
)

func aiLogin(_ context.Context, _ *xli.Command, r auxiliary.Reply) error {
	return fmt.Errorf("run cxz ai login %s on the connected Linux Manager host", r.Profile.Account)
}
