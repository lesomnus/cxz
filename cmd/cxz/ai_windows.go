//go:build windows

package main

import (
	"context"
	"fmt"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/xli"
)

func aiLogin(_ context.Context, _ *xli.Command, r *api.AuxLoginInfoReply) error {
	return fmt.Errorf("run cxz ai login %s on the connected Linux Manager host", r.Account)
}
